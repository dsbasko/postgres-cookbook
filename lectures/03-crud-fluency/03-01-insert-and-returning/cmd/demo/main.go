// Команда demo юнита 03-01: INSERT ... RETURNING — сгенерированные сервером
// значения обратно в одном round-trip.
//
// Два режима:
//
//	demo          — карта лояльности: RETURNING id/points/created_at + bulk-insert;
//	demo -reset   — накатить канон Brew + таблицу loyalty_cards и выйти (db-reset).
//
// created_at заполняется DEFAULT now() — его значение недетерминированно, поэтому
// печатаем не само время, а факт «заполнено ли» (created_set). Логи — в stderr,
// stdout — только результат (для дословной вставки в README).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"text/tabwriter"

	"github.com/dsbasko/postgres-cookbook/lectures/03-crud-fluency/03-01-insert-and-returning/internal/db"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

func main() {
	logger := log.New()

	reset := flag.Bool("reset", false, "накатить канон Brew + loyalty_cards и выйти")
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
		ddl, err := schemaDDL()
		if err != nil {
			return err
		}
		if err := brew.Apply(ctx, pool, ddl); err != nil {
			return fmt.Errorf("brew.Apply: %w", err)
		}
		fmt.Println("Канон Brew + таблица loyalty_cards накатаны.")
		return nil
	}

	queries := db.New(pool)

	// Обнуляем таблицу — id стартуют с 1, вывод воспроизводим.
	if err := queries.TruncateCards(ctx); err != nil {
		return fmt.Errorf("TruncateCards: %w", err)
	}

	// 1) Одна вставка: RETURNING отдаёт сгенерированный id и значения по DEFAULT.
	card, err := queries.IssueCard(ctx, db.IssueCardParams{CustomerID: 1, CardNo: "BREW-0001"})
	if err != nil {
		return fmt.Errorf("IssueCard: %w", err)
	}
	fmt.Println("1) INSERT ... RETURNING — серверные значения обратно одним запросом:")
	fmt.Printf("   выдали карту: id=%d, points=%d (по DEFAULT), created_at заполнен=%v\n",
		card.ID, card.Points, card.CreatedSet)
	fmt.Println("   → id и points не передавали — их вернул RETURNING, без второго SELECT.")

	// 2) Многострочная вставка: одна команда, RETURNING по строке на карту.
	bulk, err := queries.IssueCardsBulk(ctx, db.IssueCardsBulkParams{
		CustA: 2, CardA: "BREW-0002",
		CustB: 3, CardB: "BREW-0003",
	})
	if err != nil {
		return fmt.Errorf("IssueCardsBulk: %w", err)
	}
	fmt.Println("\n2) Многострочный INSERT ... RETURNING — то же и для многих строк:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tCARD_NO")
	for _, c := range bulk {
		fmt.Fprintf(w, "%d\t%s\n", c.ID, c.CardNo)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Println("   → одна команда вставила обе карты; RETURNING вернул id каждой.")

	return nil
}

// schemaDDL читает schema.sql юнита (DDL поверх канона: таблица loyalty_cards).
// Путь резолвится через runtime.Caller относительно этого исходника (go:embed не
// дотянется: файл лежит на два уровня выше cmd/demo/).
func schemaDDL() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("runtime.Caller: не удалось определить путь к исходнику")
	}
	path := filepath.Join(filepath.Dir(thisFile), "..", "..", "schema.sql")
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read schema.sql: %w", err)
	}
	return string(b), nil
}
