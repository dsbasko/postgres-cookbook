// Команда demo юнита 04-01: четыре вида JOIN.
//
// Два режима:
//
//	demo          — INNER / LEFT / RIGHT на customers↔orders + FULL-сверка двух листов;
//	demo -reset   — накатить канон Brew + таблицы пересчёта и выйти (db-reset).
//
// INNER/LEFT/RIGHT идут по каноническим customers↔orders (Карина без заказов —
// несовпавшая строка), FULL — по лабораторным листам пересчёта. Логи в stderr,
// stdout — только результат (для вставки в README).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/dsbasko/postgres-cookbook/lectures/04-querying-across-tables/04-01-joins-inner-left-right-full/internal/db"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
	"github.com/jackc/pgx/v5/pgtype"
)

func main() {
	logger := log.New()

	reset := flag.Bool("reset", false, "накатить канон Brew + таблицы пересчёта и выйти")
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
		fmt.Println("Канон Brew + листы пересчёта (count_floor/count_storage) накатаны.")
		return nil
	}

	queries := db.New(pool)

	// 1) INNER JOIN: только клиенты, у которых есть заказы.
	inner, err := queries.InnerCustomersOrders(ctx)
	if err != nil {
		return fmt.Errorf("InnerCustomersOrders: %w", err)
	}
	fmt.Printf("1) INNER JOIN customers↔orders — только совпавшие пары (строк: %d):\n", len(inner))
	for _, r := range inner {
		fmt.Printf("   %-16s заказ #%d (%s)\n", r.Customer, r.OrderID, r.Status)
	}
	fmt.Println("   → Карины тут нет: у неё нет заказов, совпадать не с чем.")

	// 2) LEFT JOIN: все клиенты, заказ — если есть.
	left, err := queries.LeftCustomersOrders(ctx)
	if err != nil {
		return fmt.Errorf("LeftCustomersOrders: %w", err)
	}
	fmt.Printf("\n2) LEFT JOIN customers←orders — все клиенты, заказ если есть (строк: %d):\n", len(left))
	for _, r := range left {
		fmt.Printf("   %-16s заказ %-4s статус %s\n", r.Customer, orderRef(r.OrderID), nullText(r.Status))
	}
	fmt.Println("   → Карина осталась: заказа нет → order_id и status = NULL.")

	// 3) RIGHT JOIN: зеркало LEFT (таблицы переставлены).
	right, err := queries.RightOrdersCustomers(ctx)
	if err != nil {
		return fmt.Errorf("RightOrdersCustomers: %w", err)
	}
	fmt.Printf("\n3) RIGHT JOIN orders→customers — тот же результат, что LEFT (строк: %d):\n", len(right))
	for _, r := range right {
		fmt.Printf("   %-16s заказ %-4s статус %s\n", r.Customer, orderRef(r.OrderID), nullText(r.Status))
	}
	fmt.Println("   → RIGHT = LEFT с переставленными таблицами; в коде почти всегда пишут LEFT.")

	// 4) FULL JOIN: сверка двух листов пересчёта — несовпадения с обеих сторон.
	if err := queries.TruncateCounts(ctx); err != nil {
		return fmt.Errorf("TruncateCounts: %w", err)
	}
	if err := queries.SeedFloor(ctx); err != nil {
		return fmt.Errorf("SeedFloor: %w", err)
	}
	if err := queries.SeedStorage(ctx); err != nil {
		return fmt.Errorf("SeedStorage: %w", err)
	}
	recon, err := queries.ReconcileFull(ctx)
	if err != nil {
		return fmt.Errorf("ReconcileFull: %w", err)
	}
	fmt.Printf("\n4) FULL JOIN — сверка листов пересчёта (зал {1,2} vs склад {2,4}):\n")
	fmt.Printf("   %-12s %6s %8s\n", "напиток", "зал", "склад")
	for _, r := range recon {
		fmt.Printf("   %-12s %6s %8s\n", r.Drink, qty(r.FloorQty), qty(r.StorageQty))
	}
	fmt.Println("   → строки есть с обеих сторон: только в зале, только на складе, в обоих.")

	return nil
}

// orderRef форматирует nullable order_id: «#3» или «—», если заказа нет.
func orderRef(id pgtype.Int8) string {
	if !id.Valid {
		return "—"
	}
	return fmt.Sprintf("#%d", id.Int64)
}

// nullText печатает текст или «NULL», если значения нет (LEFT/RIGHT без пары).
func nullText(t pgtype.Text) string {
	if !t.Valid {
		return "NULL"
	}
	return t.String
}

// qty печатает количество из листа пересчёта или «—», если в этом листе напитка
// нет (несовпавшая сторона FULL JOIN).
func qty(n pgtype.Int4) string {
	if !n.Valid {
		return "—"
	}
	return fmt.Sprintf("%d", n.Int32)
}

// schemaDDL читает schema.sql юнита (DDL поверх канона: листы пересчёта). Путь
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
