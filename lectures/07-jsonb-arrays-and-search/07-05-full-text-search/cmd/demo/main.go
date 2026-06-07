// Команда demo юнита 07-05: полнотекстовый поиск.
//
// Два режима:
//
//	demo          — tsvector тела статьи (стемминг/стоп-слова), поиск с ранжиро-
//	                ванием (вес заголовка), to_tsquery с &, морфология запроса;
//	demo -reset   — накатить канон Brew + kb_articles и выйти (db-reset).
//
// Контент английский, конфигурация 'english' → стемминг и ранги детерминированы
// (не зависят от локали). Логи — в stderr, stdout — только результат (для
// дословной вставки в README).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"text/tabwriter"

	"github.com/dsbasko/postgres-cookbook/lectures/07-jsonb-arrays-and-search/07-05-full-text-search/internal/db"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

func main() {
	logger := log.New()

	reset := flag.Bool("reset", false, "накатить канон Brew + kb_articles и выйти")
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
		fmt.Println("Канон Brew + kb_articles накатаны.")
		return nil
	}

	queries := db.New(pool)

	// 1) tsvector тела: стемминг + выброшенные стоп-слова + позиции лексем.
	tsv, err := queries.ShowTsvector(ctx)
	if err != nil {
		return fmt.Errorf("ShowTsvector: %w", err)
	}
	fmt.Println("1) tsvector тела статьи 2 (стемминг brewing→brew, hours→hour; стоп-слова выброшены):")
	fmt.Printf("   %s\n", tsv)

	// 2) поиск 'brew' с ранжированием — вес заголовка (A) поднимает статью 2.
	ranked, err := queries.SearchRanked(ctx)
	if err != nil {
		return fmt.Errorf("SearchRanked: %w", err)
	}
	fmt.Println("\n2) поиск 'brew', ранжирование ts_rank (вес A заголовка > B тела):")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tЗАГОЛОВОК\tРАНГ")
	for _, r := range ranked {
		fmt.Fprintf(w, "%d\t%s\t%s\n", r.ID, r.Title, r.Rank)
	}
	if err := w.Flush(); err != nil {
		return err
	}

	// 3) to_tsquery с &: обе лексемы обязательны.
	andRows, err := queries.SearchAnd(ctx)
	if err != nil {
		return fmt.Errorf("SearchAnd: %w", err)
	}
	fmt.Println("\n3) to_tsquery('milk & cappuccino') — нужны обе лексемы:")
	for _, r := range andRows {
		fmt.Printf("   %d  %s\n", r.ID, r.Title)
	}

	// 4) морфология: запрос 'brewing' стеммится в 'brew' и находит совпадения.
	stem, err := queries.StemmingMatch(ctx)
	if err != nil {
		return fmt.Errorf("StemmingMatch: %w", err)
	}
	fmt.Println("\n4) запрос 'brewing' (стем → brew) — морфология, чего не дал бы LIKE:")
	for _, r := range stem {
		fmt.Printf("   %d  %s\n", r.ID, r.Title)
	}

	return nil
}

// schemaDDL читает schema.sql юнита (DDL+seed поверх канона: kb_articles).
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
