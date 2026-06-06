// Команда demo юнита 04-04: DISTINCT ON — одна строка на группу.
//
// Два режима:
//
//	demo          — последний заказ на клиента + самый дорогой заказ на клиента;
//	demo -reset   — накатить канон Brew (schema + seed) и выйти (db-reset).
//
// Запросы read-only по каноническим orders/customers — вывод детерминирован
// (created_at фиксированы в seed). Логи в stderr, stdout — только результат.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/dsbasko/postgres-cookbook/lectures/04-querying-across-tables/04-04-distinct-on/internal/db"
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

	// 1) Последний заказ на клиента: DISTINCT ON + ORDER BY ... created_at DESC.
	latest, err := queries.LatestOrderPerCustomer(ctx)
	if err != nil {
		return fmt.Errorf("LatestOrderPerCustomer: %w", err)
	}
	fmt.Println("1) Последний заказ на клиента — DISTINCT ON (customer_id), свежесть по created_at:")
	fmt.Printf("   %-16s %6s %8s %-9s %s\n", "клиент", "заказ", "сумма", "статус", "дата")
	for _, r := range latest {
		fmt.Printf("   %-16s #%-5d %8s %-9s %s\n", r.Customer, r.OrderID, r.Amount, r.Status, r.Day)
	}
	fmt.Println("   → у Алисы два заказа (#1 и #3), DISTINCT ON оставил один свежий — #3.")
	fmt.Println("     Карины нет: у неё заказов нет, а выбираем мы из orders.")

	// 2) Самый дорогой заказ на клиента: тот же DISTINCT ON, хвост ORDER BY = amount DESC.
	priciest, err := queries.PriciestOrderPerCustomer(ctx)
	if err != nil {
		return fmt.Errorf("PriciestOrderPerCustomer: %w", err)
	}
	fmt.Println("\n2) Самый дорогой заказ на клиента — тот же DISTINCT ON, но хвост ORDER BY = amount DESC:")
	fmt.Printf("   %-16s %6s %8s\n", "клиент", "заказ", "сумма")
	for _, r := range priciest {
		fmt.Printf("   %-16s #%-5d %8s\n", r.Customer, r.OrderID, r.Amount)
	}
	fmt.Println("   → у Алисы теперь #1 (10.50 > 9.60): сменили критерий — сменился победитель группы.")

	return nil
}
