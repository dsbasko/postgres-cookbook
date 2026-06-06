// Команда demo юнита 03-03: UPDATE/DELETE безопасно — видеть масштаб и держать
// рискованную запись внутри транзакции.
//
// Два режима:
//
//	demo          — целевой UPDATE с RETURNING + «забыл WHERE» внутри tx + ROLLBACK;
//	demo -reset   — накатить канон Brew + таблицу price_lab и выйти (db-reset).
//
// price_lab пересоздаётся в начале демо (TRUNCATE + seed) → вывод детерминирован
// и идемпотентен. Логи — в stderr, stdout — только результат (для README).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/dsbasko/postgres-cookbook/lectures/03-crud-fluency/03-03-update-delete-safely/internal/db"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

// seedRow — одна строка лабораторного прайса.
type seedRow struct {
	name     string
	category string
	price    int64
}

var seed = []seedRow{
	{"Эспрессо", "coffee", 300},
	{"Капучино", "coffee", 450},
	{"Латте", "coffee", 480},
	{"Колд брю", "cold", 520},
	{"Зелёный чай", "tea", 250},
}

func main() {
	logger := log.New()

	reset := flag.Bool("reset", false, "накатить канон Brew + price_lab и выйти")
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
		fmt.Println("Канон Brew + таблица price_lab накатаны.")
		return nil
	}

	queries := db.New(pool)

	// Засеваем price_lab детерминированно (id 1..5).
	if err := queries.TruncatePriceLab(ctx); err != nil {
		return fmt.Errorf("TruncatePriceLab: %w", err)
	}
	for _, r := range seed {
		if err := queries.SeedPriceRow(ctx, db.SeedPriceRowParams{Name: r.name, Category: r.category, Price: r.price}); err != nil {
			return fmt.Errorf("SeedPriceRow: %w", err)
		}
	}
	fmt.Println("1) price_lab засеян (5 строк):")
	if err := dump(ctx, queries); err != nil {
		return err
	}

	// 2) Целевой UPDATE с RETURNING: меняем только кофе, видим ровно затронутое.
	raised, err := queries.RaiseCategory(ctx, db.RaiseCategoryParams{Delta: 50, Category: "coffee"})
	if err != nil {
		return fmt.Errorf("RaiseCategory: %w", err)
	}
	fmt.Println("\n2) Целевой UPDATE ... WHERE category='coffee' SET price+=50, RETURNING изменённое:")
	for _, r := range raised {
		fmt.Printf("   #%d %s %d.%02d\n", r.ID, r.Name, r.Price/100, r.Price%100)
	}
	fmt.Printf("   (RETURNING показал ровно %d затронутые строки)\n", len(raised))

	// 3) «Забыл WHERE» внутри транзакции: смотрим масштаб через RowsAffected и
	// откатываем — ни одно изменение не доходит до диска.
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("Begin: %w", err)
	}
	defer tx.Rollback(ctx) // на случай раннего выхода; явный Rollback ниже — это no-op после него

	qtx := queries.WithTx(tx)
	nUpd, err := qtx.RaiseAll(ctx, 100)
	if err != nil {
		return fmt.Errorf("RaiseAll: %w", err)
	}
	nDel, err := qtx.DeleteCategory(ctx, "coffee")
	if err != nil {
		return fmt.Errorf("DeleteCategory: %w", err)
	}
	fmt.Println("\n3) «Забыл WHERE» внутри транзакции — смотрим масштаб и откатываем:")
	fmt.Printf("   UPDATE без WHERE затронул бы строк: %d (вся таблица!)\n", nUpd)
	fmt.Printf("   DELETE WHERE category='coffee' затронул бы строк: %d\n", nDel)
	if err := tx.Rollback(ctx); err != nil {
		return fmt.Errorf("Rollback: %w", err)
	}
	fmt.Println("   → ROLLBACK: ни одно изменение не применено.")

	// 4) После отката состояние — ровно как в шаге 2 (кофе +50, остальное цело).
	fmt.Println("\n4) Состояние после ROLLBACK — как в шаге 2 (5 строк, кофе +50, остальное нетронуто):")
	if err := dump(ctx, queries); err != nil {
		return err
	}

	return nil
}

// dump печатает всю таблицу price_lab по порядку id.
func dump(ctx context.Context, q *db.Queries) error {
	rows, err := q.ListPriceLab(ctx)
	if err != nil {
		return fmt.Errorf("ListPriceLab: %w", err)
	}
	for _, r := range rows {
		fmt.Printf("   #%d %s %s %d.%02d\n", r.ID, r.Name, r.Category, r.Price/100, r.Price%100)
	}
	return nil
}

// schemaDDL читает schema.sql юнита (DDL поверх канона: таблица price_lab).
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
