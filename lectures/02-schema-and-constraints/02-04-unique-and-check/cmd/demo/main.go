// Команда demo юнита 02-04: UNIQUE (и NULL в нём) и CHECK.
//
//	demo          — NULL ≠ NULL по умолчанию vs NULLS NOT DISTINCT; CHECK;
//	demo -reset   — накатить канон Brew + таблицы юнита и выйти (db-reset).
//
// Логи — в stderr, stdout — только результат (для вставки в README дословно).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/dsbasko/postgres-cookbook/lectures/02-schema-and-constraints/02-04-unique-and-check/internal/db"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

func main() {
	logger := log.New()

	reset := flag.Bool("reset", false, "накатить канон Brew + таблицы юнита и выйти")
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
		fmt.Println("Канон Brew + таблицы uniq_default/uniq_nnd/check_drink накатаны.")
		return nil
	}

	queries := db.New(pool)
	for _, r := range []func(context.Context) error{queries.ResetUniqDefault, queries.ResetUniqNND, queries.ResetCheckDrink} {
		if err := r(ctx); err != nil {
			return fmt.Errorf("reset: %w", err)
		}
	}

	// 1) UNIQUE по умолчанию: NULL ≠ NULL → несколько NULL проходят; непустой дубль — нет.
	if err := queries.InsertUniqDefaultNull(ctx); err != nil {
		return fmt.Errorf("InsertUniqDefaultNull #1: %w", err)
	}
	if err := queries.InsertUniqDefaultNull(ctx); err != nil {
		return fmt.Errorf("InsertUniqDefaultNull #2: %w", err)
	}
	nDefault, err := queries.CountUniqDefault(ctx)
	if err != nil {
		return fmt.Errorf("CountUniqDefault: %w", err)
	}
	if err := queries.InsertUniqDefaultA(ctx); err != nil {
		return fmt.Errorf("InsertUniqDefaultA #1: %w", err)
	}
	dupA := queries.InsertUniqDefaultA(ctx)
	fmt.Println("1) UNIQUE по умолчанию: NULL ≠ NULL (NULLs distinct)")
	fmt.Printf("   две строки slot = NULL          → обе приняты: строк = %d\n", nDefault)
	fmt.Printf("   дубль непустого slot = 'A'      → отклонён: SQLSTATE %s (unique_violation)\n", sqlState(dupA))

	// 2) UNIQUE NULLS NOT DISTINCT: NULL = NULL → второй NULL уже дубль.
	if err := queries.InsertUniqNNDNull(ctx); err != nil {
		return fmt.Errorf("InsertUniqNNDNull #1: %w", err)
	}
	dupNull := queries.InsertUniqNNDNull(ctx)
	nNND, err := queries.CountUniqNND(ctx)
	if err != nil {
		return fmt.Errorf("CountUniqNND: %w", err)
	}
	fmt.Println("2) UNIQUE NULLS NOT DISTINCT (PG15+): NULL = NULL")
	fmt.Printf("   две строки slot = NULL          → вторая отклонена: SQLSTATE %s; строк = %d\n", sqlState(dupNull), nNND)

	// 3) CHECK: нарушение price > 0 или size IN (...) → 23514.
	badPrice := queries.InsertCheckDrink(ctx, db.InsertCheckDrinkParams{Name: "Эспрессо", Price: 0, Size: "small"})
	badSize := queries.InsertCheckDrink(ctx, db.InsertCheckDrinkParams{Name: "Эспрессо", Price: 300, Size: "huge"})
	okErr := queries.InsertCheckDrink(ctx, db.InsertCheckDrinkParams{Name: "Эспрессо", Price: 300, Size: "small"})
	fmt.Println("3) CHECK (price > 0; size IN ('small','medium','large')):")
	fmt.Printf("   price = 0,   size = 'small'     → отклонён: SQLSTATE %s (check_violation)\n", sqlState(badPrice))
	fmt.Printf("   price = 300, size = 'huge'      → отклонён: SQLSTATE %s (check_violation)\n", sqlState(badSize))
	fmt.Printf("   price = 300, size = 'small'     → %s\n", okOrState(okErr))

	return nil
}

// sqlState достаёт детерминированный SQLSTATE из ошибки Postgres.
func sqlState(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return "<нет ошибки>"
}

// okOrState возвращает «принят» при отсутствии ошибки, иначе её SQLSTATE.
func okOrState(err error) string {
	if err == nil {
		return "принят"
	}
	return "отклонён: SQLSTATE " + sqlState(err)
}

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
