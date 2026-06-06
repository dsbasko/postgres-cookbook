// Команда demo юнита 04-05: подзапросы (scalar/IN/EXISTS) и ловушка NOT IN+NULL.
//
// Два режима:
//
//	demo          — scalar/IN/EXISTS-подзапросы + NOT IN-ловушка против NOT EXISTS;
//	demo -reset   — накатить канон Brew + таблицу promo и выйти (db-reset).
//
// scalar/IN/EXISTS идут по каноническим drinks/order_items/customers; ловушка —
// по лабораторной promo (есть допустимый NULL). Логи в stderr, stdout — результат.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/dsbasko/postgres-cookbook/lectures/04-querying-across-tables/04-05-subqueries-exists-vs-in/internal/db"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

func main() {
	logger := log.New()

	reset := flag.Bool("reset", false, "накатить канон Brew + таблицу promo и выйти")
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
		fmt.Println("Канон Brew + таблица promo накатаны.")
		return nil
	}

	queries := db.New(pool)

	// 1) Scalar-подзапрос: напитки дороже средней цены.
	above, err := queries.AbovePriceAvg(ctx)
	if err != nil {
		return fmt.Errorf("AbovePriceAvg: %w", err)
	}
	fmt.Println("1) Scalar-подзапрос — напитки дороже средней цены (avg=4.00):")
	for _, d := range above {
		fmt.Printf("   #%d %-12s %d.%02d\n", d.ID, d.Name, d.BasePrice/100, d.BasePrice%100)
	}

	// 2) IN-подзапрос: напитки, которые хоть раз заказывали.
	ordered, err := queries.DrinksOrdered(ctx)
	if err != nil {
		return fmt.Errorf("DrinksOrdered: %w", err)
	}
	names := make([]string, 0, len(ordered))
	for _, d := range ordered {
		names = append(names, d.Name)
	}
	fmt.Printf("\n2) IN-подзапрос — напитки, которые хоть раз заказывали (%d): %s\n",
		len(ordered), strings.Join(names, ", "))
	fmt.Println("   → зелёного чая нет: его не заказывали ни разу.")

	// 3) EXISTS: клиенты, у которых есть хотя бы один заказ.
	withOrders, err := queries.CountCustomersWithOrders(ctx)
	if err != nil {
		return fmt.Errorf("CountCustomersWithOrders: %w", err)
	}
	fmt.Printf("\n3) EXISTS-подзапрос — клиентов хотя бы с одним заказом: %d (Карина без заказов не в счёт).\n", withOrders)

	// 4) Ловушка NOT IN + NULL против NOT EXISTS.
	if err := queries.TruncatePromo(ctx); err != nil {
		return fmt.Errorf("TruncatePromo: %w", err)
	}
	if err := queries.SeedPromo(ctx); err != nil {
		return fmt.Errorf("SeedPromo: %w", err)
	}
	trap, err := queries.CountNotFeaturedNotIn(ctx)
	if err != nil {
		return fmt.Errorf("CountNotFeaturedNotIn: %w", err)
	}
	fix, err := queries.CountNotFeaturedNotExists(ctx)
	if err != nil {
		return fmt.Errorf("CountNotFeaturedNotExists: %w", err)
	}
	fmt.Println("\n4) «Сколько напитков НЕ на акции?» — акции = {эспрессо #1, всё меню (NULL)}:")
	fmt.Printf("   NOT IN (...)      → %d   ← ловушка: NULL в списке обнулил ответ\n", trap)
	fmt.Printf("   NOT EXISTS (...)  → %d   ← правильно (5 напитков минус эспрессо #1)\n", fix)

	return nil
}

// schemaDDL читает schema.sql юнита (DDL поверх канона: таблица promo). Путь
// резолвится через runtime.Caller относительно этого исходника (go:embed не
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
