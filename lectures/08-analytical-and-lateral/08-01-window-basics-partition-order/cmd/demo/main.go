// Команда demo юнита 08-01: основы оконных функций.
//
// Два режима:
//
//	demo          — агрегат (GROUP BY схлопывает) vs окно (строки на месте),
//	                затем running total на клиента;
//	demo -reset   — накатить канон Brew + purchases_lab и выйти (db-reset).
//
// Данные лабораторной таблицы фиксированы (seed в schema.sql) → вывод
// детерминирован. Логи — в stderr, stdout — только результат (для дословной
// вставки в README).
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

	"github.com/dsbasko/postgres-cookbook/lectures/08-analytical-and-lateral/08-01-window-basics-partition-order/internal/db"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

func main() {
	logger := log.New()

	reset := flag.Bool("reset", false, "накатить канон Brew + purchases_lab и выйти")
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
		fmt.Println("Канон Brew + purchases_lab накатаны.")
		return nil
	}

	queries := db.New(pool)

	// 1) Агрегат GROUP BY: 7 покупок схлопнулись в 3 строки (по клиенту).
	totals, err := queries.CustomerTotals(ctx)
	if err != nil {
		return fmt.Errorf("CustomerTotals: %w", err)
	}
	fmt.Println("1) Агрегат GROUP BY — покупки схлопнуты в одну строку на клиента:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "КЛИЕНТ\tпокупок\tсумма")
	for _, r := range totals {
		fmt.Fprintf(w, "%s\t%d\t%d\n", r.Customer, r.Purchases, r.Total)
	}
	if err := w.Flush(); err != nil {
		return err
	}

	// 2) Оконная sum: те же 7 покупок остались на месте, рядом — итог по клиенту
	//    (PARTITION BY) и общий итог (OVER ()).
	win, err := queries.WindowTotals(ctx)
	if err != nil {
		return fmt.Errorf("WindowTotals: %w", err)
	}
	fmt.Println("\n2) Оконная sum OVER (...) — строки на месте, итоги доклеены колонкой:")
	w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "КЛИЕНТ\tдень\tсумма\tитог клиента\tобщий итог")
	for _, r := range win {
		fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\n", r.Customer, r.Day, r.Cents, r.CustomerTotal, r.GrandTotal)
	}
	if err := w.Flush(); err != nil {
		return err
	}

	// 3) ORDER BY внутри окна → running total (накопленный итог по клиенту).
	rt, err := queries.RunningTotal(ctx)
	if err != nil {
		return fmt.Errorf("RunningTotal: %w", err)
	}
	fmt.Println("\n3) sum OVER (PARTITION BY customer ORDER BY day) — running total на клиента:")
	w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "КЛИЕНТ\tдень\tсумма\tнакоплено")
	for _, r := range rt {
		fmt.Fprintf(w, "%s\t%s\t%d\t%d\n", r.Customer, r.Day, r.Cents, r.Running)
	}
	return w.Flush()
}

// schemaDDL читает schema.sql юнита (DDL+seed поверх канона: purchases_lab).
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
