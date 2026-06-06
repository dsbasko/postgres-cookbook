// Команда demo юнита 04-06: CTE (WITH) и материализация.
//
// Два режима:
//
//	demo          — CTE-конвейер трат клиента + доля заказа от общего (CTE дважды);
//	demo -reset   — накатить канон Brew (schema + seed) и выйти (db-reset).
//
// Запросы read-only по каноническим orders/order_items/customers — вывод
// детерминирован. Логи в stderr, stdout — только результат (для README).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/dsbasko/postgres-cookbook/lectures/04-querying-across-tables/04-06-ctes-and-materialization/internal/db"
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

	// 1) CTE-конвейер: позиции → сумма заказа → траты клиента.
	spend, err := queries.CustomerSpend(ctx)
	if err != nil {
		return fmt.Errorf("CustomerSpend: %w", err)
	}
	fmt.Println("1) Траты клиента — CTE-конвейер (order_totals → per_customer → имя):")
	fmt.Printf("   %-16s %7s %9s\n", "клиент", "заказов", "потрачено")
	for _, s := range spend {
		fmt.Printf("   %-16s %7d %9s\n", s.Customer, s.Orders, money(s.Spent))
	}
	fmt.Println("   → суммы посчитаны из позиций order_items; Карины нет — у неё заказов нет.")

	// 2) CTE, использованный дважды: доля заказа от общего итога.
	shares, err := queries.OrderShareOfTotal(ctx)
	if err != nil {
		return fmt.Errorf("OrderShareOfTotal: %w", err)
	}
	fmt.Println("\n2) Доля заказа от общего — CTE order_totals использован дважды (FROM + scalar-подзапрос):")
	fmt.Printf("   %-6s %9s %7s\n", "заказ", "сумма", "доля,%")
	for _, r := range shares {
		fmt.Printf("   #%-5d %9s %7s\n", r.OrderID, money(r.Cents), r.Pct)
	}
	fmt.Println("   → ссылок на CTE две → Postgres материализует его (считает один раз, переиспользует).")

	return nil
}

// money форматирует центы как «Ц.КК».
func money(cents int64) string {
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}
