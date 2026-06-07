// Команда demo юнита 07-04: массив против таблицы-связки.
//
// Два режима:
//
//	demo          — @> и = ANY по массиву, та же выборка на junction, частота
//	                тегов через GROUP BY, сборка массива обратно через array_agg;
//	demo -reset   — накатить канон Brew + drink_tags_arr/drink_tags и выйти.
//
// Данные обеих моделей фиксированы и эквивалентны → вывод детерминирован. Логи —
// в stderr, stdout — только результат (для дословной вставки в README).
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
	"text/tabwriter"

	"github.com/dsbasko/postgres-cookbook/lectures/07-jsonb-arrays-and-search/07-04-arrays-vs-junction-table/internal/db"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

func main() {
	logger := log.New()

	reset := flag.Bool("reset", false, "накатить канон Brew + drink_tags_arr/drink_tags и выйти")
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
		fmt.Println("Канон Brew + drink_tags_arr/drink_tags накатаны.")
		return nil
	}

	queries := db.New(pool)

	// 1) Массив: @> «содержит» и = ANY «принадлежит».
	coffeeArr, err := queries.ArrayTaggedCoffee(ctx)
	if err != nil {
		return fmt.Errorf("ArrayTaggedCoffee: %w", err)
	}
	cold, err := queries.ArrayHasTag(ctx, "cold")
	if err != nil {
		return fmt.Errorf("ArrayHasTag: %w", err)
	}
	fmt.Println("1) Массив text[] — операторы поиска:")
	fmt.Printf("   tags @> ARRAY['coffee']  → %s\n", strings.Join(coffeeArr, ", "))
	fmt.Printf("   'cold' = ANY(tags)       → %s\n", strings.Join(cold, ", "))

	// 2) Та же выборка на нормализованной junction — результат совпадает.
	coffeeJ, err := queries.JunctionTaggedCoffee(ctx)
	if err != nil {
		return fmt.Errorf("JunctionTaggedCoffee: %w", err)
	}
	fmt.Println("\n2) Junction — те же напитки с тегом coffee (WHERE tag = 'coffee'):")
	fmt.Printf("   → %s   (совпало с @> по массиву)\n", strings.Join(coffeeJ, ", "))

	// 3) Частота тегов: GROUP BY на junction — одна строка; на массиве нужен unnest.
	pop, err := queries.TagPopularity(ctx)
	if err != nil {
		return fmt.Errorf("TagPopularity: %w", err)
	}
	fmt.Println("\n3) Частота тегов (GROUP BY на junction — тривиально):")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ТЕГ\tНАПИТКОВ")
	for _, p := range pop {
		fmt.Fprintf(w, "%s\t%d\n", p.Tag, p.Used)
	}
	if err := w.Flush(); err != nil {
		return err
	}

	// 4) Мост: array_agg сворачивает junction обратно в массив.
	back, err := queries.TagsFromJunction(ctx)
	if err != nil {
		return fmt.Errorf("TagsFromJunction: %w", err)
	}
	fmt.Println("\n4) array_agg(tag ORDER BY tag) — junction свёрнут обратно в массив:")
	for _, r := range back {
		fmt.Printf("   %s = {%s}\n", r.DrinkSku, strings.Join(r.Tags, ", "))
	}

	return nil
}

// schemaDDL читает schema.sql юнита (DDL+seed: drink_tags_arr + drink_tags).
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
