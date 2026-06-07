// Команда demo юнита 08-02: ранжирование, top-N на группу, ntile.
//
// Два режима:
//
//	demo          — три ранга бок о бок (видно расхождение на ничьих), затем
//	                top-1 на категорию и раскладка по квартилям;
//	demo -reset   — накатить канон Brew + drink_sales_lab и выйти (db-reset).
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

	"github.com/dsbasko/postgres-cookbook/lectures/08-analytical-and-lateral/08-02-ranking-and-top-n-per-group/internal/db"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

func main() {
	logger := log.New()

	reset := flag.Bool("reset", false, "накатить канон Brew + drink_sales_lab и выйти")
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
		fmt.Println("Канон Brew + drink_sales_lab накатаны.")
		return nil
	}

	queries := db.New(pool)

	// 1) Три ранга бок о бок внутри coffee — видно расхождение на ничьих.
	ranks, err := queries.RankFunctions(ctx)
	if err != nil {
		return fmt.Errorf("RankFunctions: %w", err)
	}
	fmt.Println("1) Три ранга в категории coffee (ORDER BY units DESC, drink):")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "НАПИТОК\tпродано\trow_number\trank\tdense_rank")
	for _, r := range ranks {
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d\n", r.Drink, r.Units, r.Rn, r.Rnk, r.Dns)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Println("   → row_number уникален (2,3); rank ставит ничьим 2,2 и прыгает на 4; dense_rank идёт 2,2,3.")

	// 2) Top-1 на категорию: row_number() = 1 внутри PARTITION BY category.
	top, err := queries.TopPerCategory(ctx)
	if err != nil {
		return fmt.Errorf("TopPerCategory: %w", err)
	}
	fmt.Println("\n2) Лидер продаж в каждой категории (row_number() = 1 в CTE):")
	w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "КАТЕГОРИЯ\tнапиток\tпродано")
	for _, r := range top {
		fmt.Fprintf(w, "%s\t%s\t%d\n", r.Category, r.Drink, r.Units)
	}
	if err := w.Flush(); err != nil {
		return err
	}

	// 3) ntile(4) — раскладка всех напитков по 4 корзинам (по 2 в каждой).
	q, err := queries.Quartiles(ctx)
	if err != nil {
		return fmt.Errorf("Quartiles: %w", err)
	}
	fmt.Println("\n3) ntile(4) — квартили продаж (корзина 1 — лидеры, 4 — аутсайдеры):")
	w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "НАПИТОК\tпродано\tквартиль")
	for _, r := range q {
		fmt.Fprintf(w, "%s\t%d\t%d\n", r.Drink, r.Units, r.Quartile)
	}
	return w.Flush()
}

// schemaDDL читает schema.sql юнита (DDL+seed поверх канона: drink_sales_lab).
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
