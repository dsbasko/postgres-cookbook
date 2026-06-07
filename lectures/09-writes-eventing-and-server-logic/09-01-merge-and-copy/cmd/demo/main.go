// Команда demo юнита 09-01: MERGE + COPY FROM STDIN.
//
// Два режима:
//
//	demo          — bulk-загрузка поставки через COPY FROM STDIN, затем одна
//	                команда MERGE сверяет её с нашим складом (INSERT/UPDATE/DELETE
//	                в одном проходе) и через merge_action() отчитывается, что с
//	                каждой строкой случилось;
//	demo -reset   — накатить канон Brew (схема + seed) и выйти (db-reset).
//
// Это raw-pgx юнит (escape-hatch до sqlc) по двум причинам сразу:
//   - COPY FROM STDIN — это протокол COPY, а не обычный запрос; в pgx он живёт
//     как pool.CopyFrom, для sqlc такого метода нет;
//   - MERGE ... RETURNING merge_action() парсер sqlc v1.30.0 не понимает.
//
// Лабораторные столы (supplier_feed_lab — стейджинг поставки, stock_lab —
// наш склад) создаются и засеваются в начале run → вывод детерминирован.
// Логи — в stderr, stdout — только результат (для дословной вставки в README).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

// feed — строки ночной поставки от поставщика (drink_sku → остаток на складе).
// Заметь нули: CAP-01 пришёл с нулём — это сигнал «снять с продажи» (MERGE его
// удалит). CLD-01/TEA-01 на нашем складе ещё не заведены (MERGE их вставит).
var feed = [][]any{
	{"ESP-01", int32(60)}, // есть у нас → UPDATE 50→60
	{"CAP-01", int32(0)},  // есть у нас, но 0 → DELETE
	{"LAT-01", int32(35)}, // есть у нас → UPDATE 30→35
	{"CLD-01", int32(25)}, // у нас нет → INSERT
	{"TEA-01", int32(15)}, // у нас нет → INSERT
}

func main() {
	logger := log.New()

	reset := flag.Bool("reset", false, "накатить канон Brew (схема + seed) и выйти")
	flag.Parse()

	ctx, cancel := runctx.New()
	defer cancel()

	if err := run(ctx, *reset); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("demo failed", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, reset bool) error {
	pool, err := pg.NewPool(ctx)
	if err != nil {
		return fmt.Errorf("pg.NewPool: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping (песочница поднята? `docker compose up -d`): %w", err)
	}

	if reset {
		if err := brew.Reset(ctx, pool); err != nil {
			return fmt.Errorf("brew.Reset: %w", err)
		}
		fmt.Println("Канон Brew накатан: схема + seed-данные на месте.")
		return nil
	}

	if err := setupLab(ctx, pool); err != nil {
		return fmt.Errorf("setupLab: %w", err)
	}

	// 1) COPY FROM STDIN — массовая загрузка поставки в стейджинг-таблицу.
	// pool.CopyFrom гоняет бинарный протокол COPY: ни INSERT на строку, ни
	// round-trip на строку — весь батч уезжает одним потоком.
	copied, err := pool.CopyFrom(ctx,
		pgx.Identifier{"supplier_feed_lab"},
		[]string{"drink_sku", "on_hand"},
		pgx.CopyFromRows(feed),
	)
	if err != nil {
		return fmt.Errorf("CopyFrom: %w", err)
	}
	fmt.Printf("1) COPY FROM STDIN: загружено строк поставки = %d\n", copied)
	fmt.Println("   Наш склад ДО сверки (stock_lab):")
	if err := dumpStock(ctx, pool); err != nil {
		return err
	}

	// 2) MERGE: одна команда сверяет поставку (s) с нашим складом (t) и в одном
	// проходе делает INSERT/UPDATE/DELETE. merge_action() в RETURNING говорит,
	// какая ветка сработала для каждой строки.
	fmt.Println("\n2) MERGE поставки в склад — один проход, три исхода:")
	rows, err := mergeFeed(ctx, pool)
	if err != nil {
		return fmt.Errorf("mergeFeed: %w", err)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SKU\tmerge_action()\tостаток")
	for _, r := range rows {
		fmt.Fprintf(w, "%s\t%s\t%d\n", r.sku, r.action, r.onHand)
	}
	if err := w.Flush(); err != nil {
		return err
	}

	// 3) Итог: склад после сверки. CAP-01 снят (DELETE), CLD-01/TEA-01 заведены
	// (INSERT), ESP-01/LAT-01 обновлены (UPDATE) — всё одной командой.
	fmt.Println("\n3) Наш склад ПОСЛЕ сверки (stock_lab):")
	return dumpStock(ctx, pool)
}

// mergeRow — одна строка из RETURNING команды MERGE.
type mergeRow struct {
	action string
	sku    string
	onHand int32
}

// mergeFeed выполняет MERGE и собирает строки RETURNING. Порядок RETURNING у
// MERGE не определён, поэтому сортируем по SKU в Go — иначе вывод «плавал» бы.
func mergeFeed(ctx context.Context, pool *pgxpool.Pool) ([]mergeRow, error) {
	const q = `
		MERGE INTO stock_lab t
		USING supplier_feed_lab s ON t.drink_sku = s.drink_sku
		WHEN MATCHED AND s.on_hand = 0 THEN DELETE
		WHEN MATCHED THEN UPDATE SET on_hand = s.on_hand
		WHEN NOT MATCHED THEN INSERT (drink_sku, on_hand) VALUES (s.drink_sku, s.on_hand)
		RETURNING merge_action() AS action, t.drink_sku, t.on_hand;`
	rows, err := pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []mergeRow
	for rows.Next() {
		var r mergeRow
		// При ветке DELETE t.on_hand — это значение УДАЛЁННОЙ строки (до удаления).
		if err := rows.Scan(&r.action, &r.sku, &r.onHand); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].sku < out[j].sku })
	return out, nil
}

// dumpStock печатает текущее содержимое склада, отсортированное по SKU.
func dumpStock(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx, `SELECT drink_sku, on_hand FROM stock_lab ORDER BY drink_sku`)
	if err != nil {
		return err
	}
	defer rows.Close()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "   SKU\tостаток")
	for rows.Next() {
		var sku string
		var onHand int32
		if err := rows.Scan(&sku, &onHand); err != nil {
			return err
		}
		fmt.Fprintf(w, "   %s\t%d\n", sku, onHand)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return w.Flush()
}

// setupLab создаёт лабораторные столы и засевает их фиксированными данными.
// DROP+CREATE+seed делают run идемпотентным: повторный прогон даёт тот же вывод.
// Стейджинг supplier_feed_lab очищается — COPY ниже наполнит его заново.
func setupLab(ctx context.Context, pool *pgxpool.Pool) error {
	const ddl = `
		DROP TABLE IF EXISTS supplier_feed_lab;
		DROP TABLE IF EXISTS stock_lab;

		CREATE TABLE stock_lab (
			drink_sku text   PRIMARY KEY,
			on_hand   int    NOT NULL
		);
		-- Наш склад ДО поставки: три позиции уже заведены.
		INSERT INTO stock_lab (drink_sku, on_hand) VALUES
			('ESP-01', 50),
			('CAP-01', 40),
			('LAT-01', 30);

		CREATE TABLE supplier_feed_lab (
			drink_sku text NOT NULL,
			on_hand   int  NOT NULL
		);`
	_, err := pool.Exec(ctx, ddl)
	return err
}
