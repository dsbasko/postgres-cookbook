// Команда demo юнита 08-06: GROUPING SETS / ROLLUP / CUBE.
//
// Два режима:
//
//	demo          — ROLLUP (иерархические подытоги), CUBE (все комбинации),
//	                GROUPING SETS (ровно перечисленные срезы);
//	demo -reset   — накатить канон Brew + sales_fact_lab и выйти (db-reset).
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

	"github.com/dsbasko/postgres-cookbook/lectures/08-analytical-and-lateral/08-06-grouping-sets-rollup-cube/internal/db"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

func main() {
	logger := log.New()

	reset := flag.Bool("reset", false, "накатить канон Brew + sales_fact_lab и выйти")
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
		fmt.Println("Канон Brew + sales_fact_lab накатаны.")
		return nil
	}

	queries := db.New(pool)

	print := func(header string, rows func() ([]row, error)) error {
		rr, err := rows()
		if err != nil {
			return err
		}
		fmt.Println(header)
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "МАГАЗИН\tкатегория\tвыручка\tуровень")
		for _, r := range rr {
			fmt.Fprintf(w, "%s\t%s\t%d\t%d\n", r.shop, r.category, r.cents, r.level)
		}
		return w.Flush()
	}

	// 1) ROLLUP — листья + подытог по магазину + общий итог.
	if err := print("1) ROLLUP (shop, category) — листья, подытог по магазину, общий итог:", func() ([]row, error) {
		rr, err := queries.RollupByShop(ctx)
		if err != nil {
			return nil, fmt.Errorf("RollupByShop: %w", err)
		}
		out := make([]row, len(rr))
		for i, r := range rr {
			out[i] = row{r.Shop, r.Category, r.Cents, r.Level}
		}
		return out, nil
	}); err != nil {
		return err
	}

	// 2) CUBE — добавляет подытоги по категории поперёк магазинов.
	fmt.Println()
	if err := print("2) CUBE (shop, category) — плюс подытоги по категории по всей сети:", func() ([]row, error) {
		rr, err := queries.CubeAllAngles(ctx)
		if err != nil {
			return nil, fmt.Errorf("CubeAllAngles: %w", err)
		}
		out := make([]row, len(rr))
		for i, r := range rr {
			out[i] = row{r.Shop, r.Category, r.Cents, r.Level}
		}
		return out, nil
	}); err != nil {
		return err
	}

	// 3) GROUPING SETS — ровно три перечисленных среза, без листьев.
	fmt.Println()
	if err := print("3) GROUPING SETS ((shop),(category),()) — только нужные срезы:", func() ([]row, error) {
		rr, err := queries.GroupingSetsExplicit(ctx)
		if err != nil {
			return nil, fmt.Errorf("GroupingSetsExplicit: %w", err)
		}
		out := make([]row, len(rr))
		for i, r := range rr {
			out[i] = row{r.Shop, r.Category, r.Cents, r.Level}
		}
		return out, nil
	}); err != nil {
		return err
	}

	return nil
}

// row — общая форма строки итогов (одинакова у всех трёх запросов).
type row struct {
	shop     string
	category string
	cents    int64
	level    int32
}

// schemaDDL читает schema.sql юнита (DDL+seed поверх канона: sales_fact_lab).
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
