// Команда demo юнита 01-01: числа и деньги в Postgres глазами Go-разработчика.
//
// Два режима:
//
//	demo          — float vs numeric (0.1 + 0.2), меню в центах, итог заказа;
//	demo -reset   — накатить канон Brew (schema + seed) и выйти (цель db-reset).
//
// Главный вывод урока: float для денег не годится (увидишь «хвост» 0.300...4),
// а в приложении деньги держим целыми в минорных единицах (центах) как BIGINT —
// точно и ложится в Go int64. Логи — в stderr (internal/log), stdout — только
// результат (для дословной вставки в README).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/dsbasko/postgres-cookbook/lectures/01-data-types/01-01-numbers-and-money/internal/db"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

func main() {
	logger := log.New()

	reset := flag.Bool("reset", false, "накатить канон Brew (schema + seed) и выйти")
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

	queries := db.New(pool)

	// 1) float vs numeric: одна и та же арифметика, разные типы.
	fn, err := queries.FloatVsNumeric(ctx)
	if err != nil {
		return fmt.Errorf("FloatVsNumeric: %w", err)
	}
	fmt.Println("1) 0.1 + 0.2 — float8 (Go float64) против numeric:")
	fmt.Printf("   float:    %v   (= 0.3? %v)\n", fn.FloatSum, fn.FloatEq03)
	fmt.Printf("   numeric:  %s         (= 0.3? %v)\n", fn.NumericSum, fn.NumericEq03)

	// 2) меню в центах: BIGINT → int64, в рубли.копейки разворачиваем сами.
	menu, err := queries.MenuPriced(ctx)
	if err != nil {
		return fmt.Errorf("MenuPriced: %w", err)
	}
	fmt.Println("\n2) Меню Brew — base_price BIGINT в центах, печатаем как ₽.коп:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tНАЗВАНИЕ\tЦЕНТЫ\tЦЕНА")
	for _, d := range menu {
		fmt.Fprintf(w, "%d\t%s\t%d\t%d.%02d\n",
			d.ID, d.Name, d.BasePrice, d.BasePrice/100, d.BasePrice%100)
	}
	if err := w.Flush(); err != nil {
		return err
	}

	// 3) итог заказа целиком в центах: складываем целые, без float-дрейфа.
	total, err := queries.OrderTotalCents(ctx, 1)
	if err != nil {
		return fmt.Errorf("OrderTotalCents: %w", err)
	}
	fmt.Printf("\n3) Итог заказа #1 — sum в центах:  %d  (= %d.%02d)\n",
		total, total/100, total%100)

	return nil
}
