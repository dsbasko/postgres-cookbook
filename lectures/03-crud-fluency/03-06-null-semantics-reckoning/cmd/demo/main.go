// Команда demo юнита 03-06: трезвая семантика NULL и ловушка NOT IN + NULL.
//
// Два режима:
//
//	demo          — факты трёхзначной логики + NOT IN (ловушка) против NOT EXISTS;
//	demo -reset   — накатить канон Brew + таблицу unavailable и выйти (db-reset).
//
// Запросы к каноническим drinks (read-only) + лабораторная unavailable, которую
// демо пересоздаёт детерминированно. Логи — в stderr, stdout — только результат.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/dsbasko/postgres-cookbook/lectures/03-crud-fluency/03-06-null-semantics-reckoning/internal/db"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

func main() {
	logger := log.New()

	reset := flag.Bool("reset", false, "накатить канон Brew + unavailable и выйти")
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
		fmt.Println("Канон Brew + таблица unavailable накатаны.")
		return nil
	}

	queries := db.New(pool)

	// 1) Факты трёхзначной логики и инструменты — на литералах.
	f, err := queries.NullLogic(ctx)
	if err != nil {
		return fmt.Errorf("NullLogic: %w", err)
	}
	fmt.Println("1) Трёхзначная логика NULL и инструменты:")
	fmt.Printf("   (NULL = NULL) IS NULL            = %-5v  (= с NULL даёт NULL, не true)\n", f.EqIsNull)
	fmt.Printf("   NULL IS NOT DISTINCT FROM NULL   = %-5v  (NULL-безопасное равенство)\n", f.IsNotDistinct)
	fmt.Printf("   NULLIF(100, 100) IS NULL         = %-5v  (NULLIF → NULL, когда равны)\n", f.NullifEqIsNull)
	fmt.Printf("   COALESCE(NULL, NULL, 42)         = %-5d  (первое не-NULL)\n", f.CoalesceVal)

	// 2) Засеваем список недоступных с затесавшимся NULL.
	if err := queries.TruncateUnavailable(ctx); err != nil {
		return fmt.Errorf("TruncateUnavailable: %w", err)
	}
	if err := queries.SeedUnavailable(ctx); err != nil {
		return fmt.Errorf("SeedUnavailable: %w", err)
	}
	fmt.Println("\n2) Список недоступных напитков unavailable = {4, NULL} (NULL затесался по ошибке).")

	// 3) «Сколько напитков доступно?» — ловушка NOT IN против NOT EXISTS.
	trap, err := queries.CountAvailableNotIn(ctx)
	if err != nil {
		return fmt.Errorf("CountAvailableNotIn: %w", err)
	}
	fix, err := queries.CountAvailableNotExists(ctx)
	if err != nil {
		return fmt.Errorf("CountAvailableNotExists: %w", err)
	}
	fmt.Println("\n3) «Сколько напитков доступно?» — два способа:")
	fmt.Printf("   NOT IN (...)      → %d   ← ловушка: NULL в списке обнулил ответ\n", trap)
	fmt.Printf("   NOT EXISTS (...)  → %d   ← правильно (5 напитков минус колд брю #4)\n", fix)

	return nil
}

// schemaDDL читает schema.sql юнита (DDL поверх канона: таблица unavailable).
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
