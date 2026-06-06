// Команда demo юнита 02-02: PRIMARY KEY (= NOT NULL + UNIQUE) и выбор ключа —
// натуральный (бизнес-код как PK) против суррогатного (синтетический id).
//
//	demo          — PK отвергает NULL и дубль; NOT NULL; переименование ключа;
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

	"github.com/dsbasko/postgres-cookbook/lectures/02-schema-and-constraints/02-02-not-null-pk-natural-vs-surrogate/internal/db"
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
		fmt.Println("Канон Brew + таблицы shop_natural/shop_surrogate накатаны.")
		return nil
	}

	queries := db.New(pool)

	if err := queries.ResetNatural(ctx); err != nil {
		return fmt.Errorf("ResetNatural: %w", err)
	}
	if err := queries.ResetSurrogate(ctx); err != nil {
		return fmt.Errorf("ResetSurrogate: %w", err)
	}

	// 1) PRIMARY KEY = NOT NULL + UNIQUE: отвергает и NULL в ключ, и дубль.
	if err := queries.InsertNatural(ctx, db.InsertNaturalParams{Code: "BREW-CENTRAL", Name: "Brew Central"}); err != nil {
		return fmt.Errorf("InsertNatural: %w", err)
	}
	nullPK := queries.InsertNaturalNullCode(ctx, "No-Code Shop")
	dupPK := queries.InsertNatural(ctx, db.InsertNaturalParams{Code: "BREW-CENTRAL", Name: "Dup"})
	fmt.Println("1) PRIMARY KEY = NOT NULL + UNIQUE (таблица на натуральном ключе code):")
	fmt.Printf("   NULL в PK-колонку code      → отклонён: SQLSTATE %s (not_null_violation)\n", sqlState(nullPK))
	fmt.Printf("   дубль code 'BREW-CENTRAL'   → отклонён: SQLSTATE %s (unique_violation)\n", sqlState(dupPK))

	// 2) NOT NULL на обычной колонке.
	nullName := queries.InsertNaturalNullName(ctx, "BREW-NONAME")
	fmt.Println("2) NOT NULL на обычной колонке:")
	fmt.Printf("   NULL в name                 → отклонён: SQLSTATE %s (not_null_violation)\n", sqlState(nullName))

	// 3) Переименование ключа: натуральный «уезжает», суррогатный id стоит.
	if err := queries.InsertNatural(ctx, db.InsertNaturalParams{Code: "BREW-OLD", Name: "Brew Old"}); err != nil {
		return fmt.Errorf("InsertNatural BREW-OLD: %w", err)
	}
	surID, err := queries.InsertSurrogate(ctx, db.InsertSurrogateParams{Code: "BREW-OLD", Name: "Brew Old"})
	if err != nil {
		return fmt.Errorf("InsertSurrogate: %w", err)
	}

	if err := queries.RenameNaturalCode(ctx, db.RenameNaturalCodeParams{Code: "BREW-OLD", Code_2: "BREW-NEW"}); err != nil {
		return fmt.Errorf("RenameNaturalCode: %w", err)
	}
	oldPresent, err := queries.NaturalCodeExists(ctx, "BREW-OLD")
	if err != nil {
		return fmt.Errorf("NaturalCodeExists old: %w", err)
	}
	newPresent, err := queries.NaturalCodeExists(ctx, "BREW-NEW")
	if err != nil {
		return fmt.Errorf("NaturalCodeExists new: %w", err)
	}

	if err := queries.RenameSurrogateCode(ctx, db.RenameSurrogateCodeParams{Code: "BREW-OLD", Code_2: "BREW-NEW"}); err != nil {
		return fmt.Errorf("RenameSurrogateCode: %w", err)
	}
	surIDAfter, err := queries.SurrogateIDByCode(ctx, "BREW-NEW")
	if err != nil {
		return fmt.Errorf("SurrogateIDByCode: %w", err)
	}

	fmt.Println("3) Переименование ключа 'BREW-OLD' → 'BREW-NEW':")
	fmt.Printf("   натуральный PK (code):  старого ключа нет (%v), новый есть (%v) — сменилось само значение ключа\n", oldPresent, newPresent)
	fmt.Printf("   суррогат (id):          id = %d → %d неизменен, сменился только атрибут code — identity строки стабильна\n", surID, surIDAfter)

	return nil
}

// sqlState достаёт детерминированный SQLSTATE из ошибки Postgres (текст
// сообщения недетерминирован — печатать его в README нельзя).
func sqlState(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return "<нет ошибки>"
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
