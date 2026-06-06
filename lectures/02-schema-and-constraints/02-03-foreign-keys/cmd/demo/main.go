// Команда demo юнита 02-03: внешние ключи и поведение при удалении родителя.
//
//	demo          — FK блокирует висящую ссылку; CASCADE / SET NULL / RESTRICT;
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
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/dsbasko/postgres-cookbook/lectures/02-schema-and-constraints/02-03-foreign-keys/internal/db"
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
		fmt.Println("Канон Brew + таблицы fk_* накатаны.")
		return nil
	}

	queries := db.New(pool)
	if err := queries.ResetFK(ctx); err != nil {
		return fmt.Errorf("ResetFK: %w", err)
	}

	// 1) FK не даёт сослаться на несуществующего клиента.
	dangling := queries.InsertOrder(ctx, db.InsertOrderParams{CustomerID: 999, Note: "ghost"})
	fmt.Println("1) FK блокирует «висящую» ссылку:")
	fmt.Printf("   заказ с customer_id = 999 (нет такого клиента) → отклонён: SQLSTATE %s (foreign_key_violation)\n", sqlState(dangling))

	// 2) Заводим клиента и его детей: 2 заказа (CASCADE) + 1 отзыв (SET NULL).
	custID, err := queries.InsertCustomer(ctx, "Алиса")
	if err != nil {
		return fmt.Errorf("InsertCustomer: %w", err)
	}
	for _, note := range []string{"эспрессо", "капучино"} {
		if err := queries.InsertOrder(ctx, db.InsertOrderParams{CustomerID: custID, Note: note}); err != nil {
			return fmt.Errorf("InsertOrder: %w", err)
		}
	}
	if err := queries.InsertReview(ctx, db.InsertReviewParams{
		CustomerID: pgtype.Int8{Int64: custID, Valid: true},
		Stars:      5,
	}); err != nil {
		return fmt.Errorf("InsertReview: %w", err)
	}
	orders0, err := queries.CountOrders(ctx)
	if err != nil {
		return fmt.Errorf("CountOrders: %w", err)
	}
	rev0, err := queries.CountReviews(ctx)
	if err != nil {
		return fmt.Errorf("CountReviews: %w", err)
	}
	fmt.Printf("2) Завели клиента id=%d: его заказов (ON DELETE CASCADE) = %d, отзывов (ON DELETE SET NULL) = %d\n",
		custID, orders0, rev0.Total)

	// 3) Удаляем клиента — срабатывают политики детей.
	if err := queries.DeleteCustomer(ctx, custID); err != nil {
		return fmt.Errorf("DeleteCustomer: %w", err)
	}
	orders1, err := queries.CountOrders(ctx)
	if err != nil {
		return fmt.Errorf("CountOrders after: %w", err)
	}
	rev1, err := queries.CountReviews(ctx)
	if err != nil {
		return fmt.Errorf("CountReviews after: %w", err)
	}
	fmt.Printf("3) DELETE клиента id=%d:\n", custID)
	fmt.Printf("   ON DELETE CASCADE → заказы удалены каскадом: осталось %d\n", orders1)
	fmt.Printf("   ON DELETE SET NULL → отзыв жив, ссылка обнулена: отзывов %d, из них customer_id IS NULL: %d\n",
		rev1.Total, rev1.NullCustomer)

	// 4) Дефолтное поведение FK (NO ACTION ≈ RESTRICT): родителя не удалить,
	// пока на него ссылается ребёнок.
	drinkID, err := queries.InsertDrink(ctx, "Латте")
	if err != nil {
		return fmt.Errorf("InsertDrink: %w", err)
	}
	if err := queries.InsertOrderItem(ctx, db.InsertOrderItemParams{DrinkID: drinkID, Qty: 2}); err != nil {
		return fmt.Errorf("InsertOrderItem: %w", err)
	}
	restricted := queries.DeleteDrink(ctx, drinkID)
	fmt.Println("4) ON DELETE по умолчанию (NO ACTION / RESTRICT):")
	fmt.Printf("   пока на напиток id=%d ссылается позиция заказа, DELETE напитка → отклонён: SQLSTATE %s\n",
		drinkID, sqlState(restricted))

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
