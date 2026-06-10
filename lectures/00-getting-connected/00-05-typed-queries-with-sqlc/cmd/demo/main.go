// Команда demo юнита 00-04: те же запросы, что в 00-03, но через sqlc.
//
// Два режима:
//
//	demo          — поиск по категории (:many), напиток по SKU (:one), счётчик (:one);
//	demo -reset   — накатить канон Brew (schema + seed) и выйти (цель db-reset).
//
// Сравни с 00-03: там был ручной pool.Query + rows.Scan, тут — типизированные
// методы db.Queries, сгенерированные из query.sql. main.go стал тоньше, а
// порядок и типы колонок проверены против схемы на этапе компиляции. Логи —
// в stderr (internal/log), stdout — только результат (для вставки в README).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/dsbasko/postgres-cookbook/lectures/00-getting-connected/00-05-typed-queries-with-sqlc/internal/db"
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

	// db.New оборачивает пул в типизированный *Queries (сгенерён sqlc из query.sql).
	queries := db.New(pool)

	// :many + параметр $1='coffee' (аргумент типизирован из схемы как string).
	coffee, err := queries.ListDrinksByCategory(ctx, "coffee")
	if err != nil {
		return fmt.Errorf("ListDrinksByCategory: %w", err)
	}
	fmt.Println(`1) ListDrinksByCategory("coffee") — :many, $1 типизирован как string:`)
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSKU\tНАЗВАНИЕ\tКАТЕГОРИЯ\tЦЕНА")
	for _, d := range coffee {
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%d.%02d\n",
			d.ID, d.Sku, d.Name, d.Category, d.BasePrice/100, d.BasePrice%100)
	}
	if err := w.Flush(); err != nil {
		return err
	}

	// :one → ровно одна строка (типизированная структура GetDrinkBySKURow).
	cold, err := queries.GetDrinkBySKU(ctx, "CLD-01")
	if err != nil {
		return fmt.Errorf("GetDrinkBySKU: %w", err)
	}
	fmt.Printf("\n2) GetDrinkBySKU(\"CLD-01\") — :one, одна строка:\n   #%d  %s  %s  (%s)  %d.%02d\n",
		cold.ID, cold.Sku, cold.Name, cold.Category, cold.BasePrice/100, cold.BasePrice%100)

	// :one со скаляром → int64 напрямую, без структуры-обёртки.
	teaCount, err := queries.CountDrinksByCategory(ctx, "tea")
	if err != nil {
		return fmt.Errorf("CountDrinksByCategory: %w", err)
	}
	fmt.Printf("\n3) CountDrinksByCategory(\"tea\") — :one, скаляр int64:  %d\n", teaCount)

	return nil
}
