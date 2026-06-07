// Команда demo юнита 10-02: движок цен и промо.
//
// Капстон про два способа сказать «эти интервалы не должны пересекаться» — и про
// аудит изменения цены без отдельного SELECT:
//
//   - PG18 temporal: PRIMARY KEY (drink_id, valid WITHOUT OVERLAPS) — у одного
//     напитка не может быть двух цен на один момент времени, БД сама это держит;
//   - классический EXCLUDE USING gist (...&&) — тот же запрет до PG18, на промо;
//   - RETURNING old.* / new.* (PG18) — UPDATE цены сразу отдаёт «было → стало»,
//     этим и наполняем аудит.
//
// Два режима:
//
//	demo          — собрать таблицы, показать оба запрета пересечений и аудит;
//	demo -reset   — накатить канон Brew (схема + seed) и выйти (db-reset).
//
// raw-pgx escape-hatch (go.mod, без sqlc): sqlc v1.30.0 не парсит DDL с
// WITHOUT OVERLAPS и не понимает RETURNING old/new (см. 03-05) — выбираем фичу,
// а не инструмент. Лабораторные столы пересоздаются в начале, канон не трогаем.
// Логи — в stderr, stdout — только результат (для дословной вставки в README).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

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

	if err := buildSchema(ctx, pool); err != nil {
		return fmt.Errorf("buildSchema: %w", err)
	}

	fmt.Println("1) Temporal PK (PG18): у одного напитка не пересекаются периоды цены.")
	if err := showTemporal(ctx, pool); err != nil {
		return fmt.Errorf("showTemporal: %w", err)
	}

	fmt.Println("\n2) Классический EXCLUDE (до PG18): то же для окон промо-кода.")
	if err := showExclude(ctx, pool); err != nil {
		return fmt.Errorf("showExclude: %w", err)
	}

	fmt.Println("\n3) RETURNING old/new (PG18): меняем цену и пишем аудит без отдельного SELECT.")
	if err := showAudit(ctx, pool); err != nil {
		return fmt.Errorf("showAudit: %w", err)
	}

	return nil
}

// buildSchema собирает таблицы движка цен и промо.
//
//   - price_periods: temporal-таблица. PRIMARY KEY (drink_id, valid WITHOUT
//     OVERLAPS) — PG18 разрешает один скалярный столбец-ключ плюс range-столбец,
//     для которого требует «без пересечений». Скалярная часть ключа опирается на
//     btree_gist (целочисленный gist-opclass).
//   - promo_windows: то же ограничение «без пересечений по коду», но классикой —
//     EXCLUDE USING gist (..&&). Так делали до PG18; полезно знать обе формы.
//   - price_audit: журнал «было → стало», наполняем через RETURNING old/new.
func buildSchema(ctx context.Context, pool *pgxpool.Pool) error {
	const ddl = `
		CREATE EXTENSION IF NOT EXISTS btree_gist;

		DROP TABLE IF EXISTS price_periods;
		DROP TABLE IF EXISTS promo_windows;
		DROP TABLE IF EXISTS price_audit;

		CREATE TABLE price_periods (
			drink_id    bigint    NOT NULL,
			price_cents bigint    NOT NULL CHECK (price_cents > 0),
			valid       tstzrange NOT NULL,
			PRIMARY KEY (drink_id, valid WITHOUT OVERLAPS)
		);

		CREATE TABLE promo_windows (
			code text      NOT NULL,
			span tstzrange NOT NULL,
			EXCLUDE USING gist (code WITH =, span WITH &&)
		);

		CREATE TABLE price_audit (
			id        bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
			drink_id  bigint      NOT NULL,
			old_cents bigint      NOT NULL,
			new_cents bigint      NOT NULL,
			changed_at timestamptz NOT NULL DEFAULT now()
		);`
	_, err := pool.Exec(ctx, ddl)
	return err
}

// showTemporal кладёт два смежных периода цены (приняты), затем пробует период,
// который перекрывает уже существующий, — temporal PK отбивает его как
// нарушение exclusion-ограничения (SQLSTATE 23P01).
func showTemporal(ctx context.Context, pool *pgxpool.Pool) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "   период цены напитка #1\tцена\tрезультат")

	periods := []struct {
		from, to string
		price    int64
	}{
		{"2025-01-01", "2025-02-01", 300},
		{"2025-02-01", "2025-03-01", 320},
		{"2025-01-15", "2025-02-15", 999}, // перекрывает первые два → отбой
	}
	for _, p := range periods {
		_, err := pool.Exec(ctx,
			`INSERT INTO price_periods (drink_id, price_cents, valid)
			 VALUES (1, $1, tstzrange($2::timestamptz, $3::timestamptz))`,
			p.price, p.from, p.to)
		fmt.Fprintf(w, "   [%s, %s)\t%d.%02d\t%s\n", p.from, p.to, p.price/100, p.price%100, outcome(err))
	}
	return w.Flush()
}

// showExclude — та же защита от пересечений до PG18: EXCLUDE по (code =, span &&).
// Одинаковый код с пересекающимся окном отбивается; другой код с тем же окном
// проходит (ограничение смотрит на пару «код + окно»).
func showExclude(ctx context.Context, pool *pgxpool.Pool) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "   промо-код\tокно\tрезультат")

	windows := []struct {
		code, from, to string
	}{
		{"SUMMER", "2025-06-01", "2025-09-01"},
		{"SUMMER", "2025-08-01", "2025-10-01"}, // тот же код, окно пересекается → отбой
		{"AUTUMN", "2025-08-01", "2025-10-01"}, // другой код — пересечение разрешено
	}
	for _, win := range windows {
		_, err := pool.Exec(ctx,
			`INSERT INTO promo_windows (code, span)
			 VALUES ($1, tstzrange($2::timestamptz, $3::timestamptz))`,
			win.code, win.from, win.to)
		fmt.Fprintf(w, "   %s\t[%s, %s)\t%s\n", win.code, win.from, win.to, outcome(err))
	}
	return w.Flush()
}

// showAudit поднимает цену второго периода напитка #1 и просит UPDATE сразу
// вернуть «было» и «стало» через RETURNING old/new — этим и наполняем аудит,
// без отдельного SELECT до и после.
func showAudit(ctx context.Context, pool *pgxpool.Pool) error {
	const newPrice = int64(340)

	var oldCents, gotNew int64
	err := pool.QueryRow(ctx,
		`UPDATE price_periods
		    SET price_cents = $1
		  WHERE drink_id = 1 AND valid = tstzrange('2025-02-01'::timestamptz, '2025-03-01'::timestamptz)
		RETURNING old.price_cents, new.price_cents`,
		newPrice).Scan(&oldCents, &gotNew)
	if err != nil {
		return err
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO price_audit (drink_id, old_cents, new_cents) VALUES (1, $1, $2)`,
		oldCents, gotNew); err != nil {
		return err
	}

	fmt.Printf("   период [2025-02-01, 2025-03-01): цена %d.%02d → %d.%02d (одним UPDATE ... RETURNING old/new)\n",
		oldCents/100, oldCents%100, gotNew/100, gotNew%100)

	var n int64
	var auditOld, auditNew int64
	if err := pool.QueryRow(ctx,
		`SELECT count(*), max(old_cents), max(new_cents) FROM price_audit`).Scan(&n, &auditOld, &auditNew); err != nil {
		return err
	}
	fmt.Printf("   аудит: %d запись, было %d.%02d → стало %d.%02d\n",
		n, auditOld/100, auditOld%100, auditNew/100, auditNew%100)
	return nil
}

// outcome переводит ошибку INSERT в короткую метку: «OK» либо «SQLSTATE NNNNN».
func outcome(err error) string {
	if err == nil {
		return "OK (принято)"
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return "отбито, SQLSTATE " + pgErr.Code
	}
	return "ошибка: " + err.Error()
}
