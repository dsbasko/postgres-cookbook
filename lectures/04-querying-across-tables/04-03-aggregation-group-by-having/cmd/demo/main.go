// Команда demo юнита 04-03: агрегация, GROUP BY/HAVING, count(*) vs count(col).
//
// Два режима:
//
//	demo          — сводка меню по категориям + статистика по клиентам + HAVING;
//	demo -reset   — накатить канон Brew (schema + seed) и выйти (db-reset).
//
// Все запросы read-only по каноническим drinks/customers/orders — вывод
// детерминирован. Логи в stderr, stdout — только результат (для README).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/dsbasko/postgres-cookbook/lectures/04-querying-across-tables/04-03-aggregation-group-by-having/internal/db"
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

	// 1) GROUP BY category: сводка меню по категориям.
	stats, err := queries.MenuStatsByCategory(ctx)
	if err != nil {
		return fmt.Errorf("MenuStatsByCategory: %w", err)
	}
	fmt.Println("1) Сводка меню по категориям (GROUP BY category):")
	fmt.Printf("   %-8s %6s %8s %8s %8s\n", "катег.", "напитк", "min", "max", "avg")
	for _, s := range stats {
		fmt.Printf("   %-8s %6d %8s %8s %8s\n",
			s.Category, s.Drinks, money(s.PriceMin), money(s.PriceMax), money(s.PriceAvg))
	}

	// 2) GROUP BY клиент: count(*) vs count(o.id) на LEFT JOIN.
	cust, err := queries.CustomerOrderStats(ctx)
	if err != nil {
		return fmt.Errorf("CustomerOrderStats: %w", err)
	}
	fmt.Println("\n2) Статистика по клиентам (customers LEFT JOIN orders, GROUP BY клиент):")
	fmt.Printf("   %-16s %9s %9s %9s\n", "клиент", "count(*)", "count(id)", "выручка")
	for _, c := range cust {
		fmt.Printf("   %-16s %9d %9d %9s\n", c.Customer, c.RowsInGroup, c.Orders, c.Revenue)
	}
	fmt.Println("   → у Карины count(*)=1 (строка есть), но count(o.id)=0 (заказов нет):")
	fmt.Println("     count(*) считает строки, count(колонка) — только не-NULL значения.")

	// 3) HAVING: оставить только клиентов с двумя и более заказами.
	regulars, err := queries.RegularCustomers(ctx)
	if err != nil {
		return fmt.Errorf("RegularCustomers: %w", err)
	}
	fmt.Println("\n3) Постоянные клиенты — HAVING count(o.id) >= 2:")
	for _, r := range regulars {
		fmt.Printf("   %-16s заказов: %d, выручка: %s\n", r.Customer, r.Orders, r.Revenue)
	}
	fmt.Println("   → HAVING фильтрует уже посчитанные группы; WHERE так не умеет.")

	return nil
}

// money форматирует центы как «Ц.КК».
func money(cents int64) string {
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}
