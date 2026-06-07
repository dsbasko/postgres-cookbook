// Команда demo юнита 08-03: lag/lead и оконные фреймы.
//
// Два режима:
//
//	demo          — день-к-дню через lag/lead, затем скользящее среднее двумя
//	                фреймами (ROWS vs RANGE) — на пропуске дня они расходятся;
//	demo -reset   — накатить канон Brew + daily_revenue_lab и выйти (db-reset).
//
// Данные лабораторной таблицы фиксированы (seed в schema.sql) → вывод
// детерминирован. Логи — в stderr, stdout — только результат.
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

	"github.com/dsbasko/postgres-cookbook/lectures/08-analytical-and-lateral/08-03-lag-lead-and-frames/internal/db"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

func main() {
	logger := log.New()

	reset := flag.Bool("reset", false, "накатить канон Brew + daily_revenue_lab и выйти")
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
		fmt.Println("Канон Brew + daily_revenue_lab накатаны.")
		return nil
	}

	queries := db.New(pool)

	// 1) lag/lead: предыдущий день, дельта день-к-дню, следующий день.
	dod, err := queries.DayOverDay(ctx)
	if err != nil {
		return fmt.Errorf("DayOverDay: %w", err)
	}
	fmt.Println("1) lag/lead — день-к-дню (prev/next = '—', где соседа нет):")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ДЕНЬ\tвыручка\tвчера\tдельта\tзавтра")
	for _, r := range dod {
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\n", r.Day, r.Cents, r.Prev, r.Delta, r.Next)
	}
	if err := w.Flush(); err != nil {
		return err
	}

	// 2) Скользящее среднее: ROWS (3 физ. строки) vs RANGE (окно в 2 дня).
	ma, err := queries.MovingAverage(ctx)
	if err != nil {
		return fmt.Errorf("MovingAverage: %w", err)
	}
	fmt.Println("\n2) Скользящее среднее за 3 дня — ROWS vs RANGE (расходятся после пропуска 05.02):")
	w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ДЕНЬ\tвыручка\tma_rows\tma_range")
	for _, r := range ma {
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\n", r.Day, r.Cents, r.MaRows, r.MaRange)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Println("   → 06 и 07 февраля: ROWS берёт 3 строки подряд, RANGE — только даты в окне 2 дней (05 нет).")

	return nil
}

// schemaDDL читает schema.sql юнита (DDL+seed поверх канона: daily_revenue_lab).
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
