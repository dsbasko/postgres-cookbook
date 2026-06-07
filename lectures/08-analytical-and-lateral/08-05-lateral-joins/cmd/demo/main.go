// Команда demo юнита 08-05: LATERAL-join (top-N на группу, убийца N+1).
//
// Два режима:
//
//	demo          — top-3 заказа на клиента одним запросом (LEFT JOIN LATERAL),
//	                затем top-1 (тот же приём с LIMIT 1, обобщение DISTINCT ON);
//	demo -reset   — накатить канон Brew + lat_*_lab и выйти (db-reset).
//
// Данные лабораторных таблиц фиксированы (seed в schema.sql) → вывод
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

	"github.com/dsbasko/postgres-cookbook/lectures/08-analytical-and-lateral/08-05-lateral-joins/internal/db"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

func main() {
	logger := log.New()

	reset := flag.Bool("reset", false, "накатить канон Brew + lat_*_lab и выйти")
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
		fmt.Println("Канон Brew + lat_*_lab накатаны.")
		return nil
	}

	queries := db.New(pool)

	// 1) Top-3 заказа на клиента — один запрос вместо N+1 из приложения.
	top3, err := queries.TopOrdersPerCustomer(ctx)
	if err != nil {
		return fmt.Errorf("TopOrdersPerCustomer: %w", err)
	}
	fmt.Println("1) Top-3 заказа на клиента (LEFT JOIN LATERAL, один запрос):")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "КЛИЕНТ\t#\tсумма\tдень")
	for _, r := range top3 {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Name, r.Rn, r.Cents, r.Day)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Println("   → Карина без заказов сохранена ('—'); у Алисы 4 заказа, top-3 отсёк самый дешёвый (280).")

	// 2) Top-1 — тот же приём с LIMIT 1 (обобщение DISTINCT ON из 04-04).
	top1, err := queries.BiggestOrderPerCustomer(ctx)
	if err != nil {
		return fmt.Errorf("BiggestOrderPerCustomer: %w", err)
	}
	fmt.Println("\n2) Самый крупный заказ на клиента (LATERAL c LIMIT 1):")
	w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "КЛИЕНТ\tсумма\tдень")
	for _, r := range top1 {
		fmt.Fprintf(w, "%s\t%s\t%s\n", r.Name, r.Cents, r.Day)
	}
	return w.Flush()
}

// schemaDDL читает schema.sql юнита (DDL+seed поверх канона: lat_*_lab).
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
