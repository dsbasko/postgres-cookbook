// Команда demo юнита 03-04: upsert через INSERT ... ON CONFLICT.
//
// Два режима:
//
//	demo          — вставка → обновление того же ключа → DO NOTHING → итог;
//	demo -reset   — накатить канон Brew + таблицу stock_levels и выйти (db-reset).
//
// stock_levels обнуляется в начале демо → вывод детерминирован и идемпотентен.
// Логи — в stderr, stdout — только результат (для дословной вставки в README).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/dsbasko/postgres-cookbook/lectures/03-crud-fluency/03-04-upsert-on-conflict/internal/db"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

func main() {
	logger := log.New()

	reset := flag.Bool("reset", false, "накатить канон Brew + stock_levels и выйти")
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
		fmt.Println("Канон Brew + таблица stock_levels накатаны.")
		return nil
	}

	queries := db.New(pool)
	if err := queries.TruncateStock(ctx); err != nil {
		return fmt.Errorf("TruncateStock: %w", err)
	}

	// 1) Первый upsert нового ключа → вставка.
	r1, err := queries.UpsertStock(ctx, db.UpsertStockParams{ShopCode: "CENTRAL", DrinkSku: "ESP-01", OnHand: 50})
	if err != nil {
		return fmt.Errorf("UpsertStock #1: %w", err)
	}
	fmt.Println("1) Первый upsert (CENTRAL/ESP-01, 50): новый ключ → вставка")
	fmt.Printf("   on_hand=%d, was_update=%v\n", r1.OnHand, r1.WasUpdate)

	// 2) Повторный upsert того же ключа → обновление; EXCLUDED.on_hand победил.
	r2, err := queries.UpsertStock(ctx, db.UpsertStockParams{ShopCode: "CENTRAL", DrinkSku: "ESP-01", OnHand: 80})
	if err != nil {
		return fmt.Errorf("UpsertStock #2: %w", err)
	}
	fmt.Println("\n2) Повторный upsert того же ключа (CENTRAL/ESP-01, 80): конфликт → обновление")
	fmt.Printf("   on_hand=%d, was_update=%v  (DO UPDATE SET on_hand = EXCLUDED.on_hand)\n", r2.OnHand, r2.WasUpdate)

	// 3) Upsert другого ключа → снова вставка.
	r3, err := queries.UpsertStock(ctx, db.UpsertStockParams{ShopCode: "NORTH", DrinkSku: "LAT-01", OnHand: 30})
	if err != nil {
		return fmt.Errorf("UpsertStock #3: %w", err)
	}
	fmt.Println("\n3) Upsert нового ключа (NORTH/LAT-01, 30): вставка")
	fmt.Printf("   on_hand=%d, was_update=%v\n", r3.OnHand, r3.WasUpdate)

	// 4) ON CONFLICT DO NOTHING на существующем ключе → 0 строк, значение цело.
	n, err := queries.UpsertIgnore(ctx, db.UpsertIgnoreParams{ShopCode: "CENTRAL", DrinkSku: "ESP-01", OnHand: 999})
	if err != nil {
		return fmt.Errorf("UpsertIgnore: %w", err)
	}
	fmt.Println("\n4) ON CONFLICT DO NOTHING для существующего ключа (CENTRAL/ESP-01, 999):")
	fmt.Printf("   строк затронуто: %d (конфликт проигнорирован, on_hand остался 80)\n", n)

	// 5) Итоговое состояние.
	rows, err := queries.ListStock(ctx)
	if err != nil {
		return fmt.Errorf("ListStock: %w", err)
	}
	fmt.Println("\n5) Итоговое состояние stock_levels:")
	for _, r := range rows {
		fmt.Printf("   %s/%s  on_hand=%d\n", r.ShopCode, r.DrinkSku, r.OnHand)
	}

	return nil
}

// schemaDDL читает schema.sql юнита (DDL поверх канона: таблица stock_levels).
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
