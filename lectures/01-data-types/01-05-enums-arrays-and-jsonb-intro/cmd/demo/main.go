// Команда demo юнита 01-05: контейнерные типы — enum, массивы, intro в jsonb.
//
// Два режима:
//
//	demo          — порядок enum, text[] из tags (@>), базовые операторы jsonb;
//	demo -reset   — накатить канон Brew + тип drink_size и выйти (цель db-reset).
//
// Это ВВЕДЕНИЕ: глубокий jsonb/GIN/полнотекст — модуль 07. Логи — в stderr,
// stdout — только результат (для дословной вставки в README).
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

	"github.com/dsbasko/postgres-cookbook/lectures/01-data-types/01-05-enums-arrays-and-jsonb-intro/internal/db"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

func main() {
	logger := log.New()

	reset := flag.Bool("reset", false, "накатить канон Brew + тип drink_size и выйти")
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
		// brew.Apply: канон → DDL юнита (тип drink_size) → seed.
		ddl, err := schemaDDL()
		if err != nil {
			return err
		}
		if err := brew.Apply(ctx, pool, ddl); err != nil {
			return fmt.Errorf("brew.Apply: %w", err)
		}
		fmt.Println("Канон Brew + тип drink_size накатаны.")
		return nil
	}

	queries := db.New(pool)

	// 1) enum упорядочен по объявлению (small < medium < large), не по алфавиту.
	eo, err := queries.EnumOrder(ctx)
	if err != nil {
		return fmt.Errorf("EnumOrder: %w", err)
	}
	fmt.Println("1) enum drink_size = ('small','medium','large') — порядок по объявлению:")
	fmt.Printf("   'small' < 'large' = %v   (по алфавиту было бы наоборот)\n", eo.SmallLtLarge)
	fmt.Printf("   'large' < 'small' = %v\n", eo.LargeLtSmall)

	// 2) массивы: tags (строка в каноне) → text[]; оператор @> «массив содержит».
	tags, err := queries.TagsAsArray(ctx)
	if err != nil {
		return fmt.Errorf("TagsAsArray: %w", err)
	}
	fmt.Println("\n2) string_to_array(tags) → text[] (в Go это []string):")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tЗАГОЛОВОК\tTAGS ([]string)")
	for _, a := range tags {
		fmt.Fprintf(w, "%d\t%s\t[%s]\n", a.ID, a.Title, strings.Join(a.TagList, " "))
	}
	if err := w.Flush(); err != nil {
		return err
	}

	coffee, err := queries.ArticlesTaggedCoffee(ctx)
	if err != nil {
		return fmt.Errorf("ArticlesTaggedCoffee: %w", err)
	}
	fmt.Printf("   tags @> ARRAY['coffee'] → статей с тегом coffee: %d\n", len(coffee))

	// 3) jsonb intro: ->> даёт text, -> оставляет jsonb (с кавычками), ? — наличие.
	j, err := queries.JSONBIntro(ctx)
	if err != nil {
		return fmt.Errorf("JSONBIntro: %w", err)
	}
	fmt.Println("\n3) jsonb '{\"size\":\"L\",\"milk\":\"oat\",\"shots\":2}' — базовые операторы:")
	fmt.Printf("   ->> 'milk'  = %v        (text: без кавычек)\n", j.MilkText)
	fmt.Printf("   ->  'milk'  = %v      (jsonb: с кавычками)\n", j.MilkJson)
	fmt.Printf("   ->> 'shots' = %v          (text '2')\n", j.ShotsText)
	fmt.Printf("   ? 'milk'    = %v       (есть ли ключ)\n", j.HasMilk)

	return nil
}

// schemaDDL читает schema.sql юнита (DDL поверх канона: тип drink_size). Путь
// резолвится через runtime.Caller относительно этого исходника — курс всегда
// запускается из исходников (go run / go test), так что функция не зависит от
// рабочего каталога (go:embed сюда не дотянется: файл на два уровня выше cmd/demo/).
func schemaDDL() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("runtime.Caller: не удалось определить путь к исходнику")
	}
	// thisFile = <unit>/cmd/demo/main.go → schema.sql на два уровня выше.
	path := filepath.Join(filepath.Dir(thisFile), "..", "..", "schema.sql")
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read schema.sql: %w", err)
	}
	return string(b), nil
}
