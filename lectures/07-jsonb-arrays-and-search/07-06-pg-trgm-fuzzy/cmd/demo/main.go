// Команда demo юнита 07-06: нечёткий поиск через pg_trgm.
//
// Два режима:
//
//	demo          — similarity к опечатке 'capucino', порог % (did-you-mean),
//	                ускоренный ILIKE по trgm-индексу;
//	demo -reset   — накатить канон Brew + pg_trgm + menu_search_lab и выйти.
//
// similarity — сравнение наборов триграмм → детерминировано (не зависит от
// локали). Логи — в stderr, stdout — только результат (для дословной вставки в
// README).
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

	"github.com/dsbasko/postgres-cookbook/lectures/07-jsonb-arrays-and-search/07-06-pg-trgm-fuzzy/internal/db"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

func main() {
	logger := log.New()

	reset := flag.Bool("reset", false, "накатить канон Brew + pg_trgm + menu_search_lab и выйти")
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
		fmt.Println("Канон Brew + pg_trgm + menu_search_lab накатаны.")
		return nil
	}

	queries := db.New(pool)

	// 1) similarity к опечатке 'capucino' по всему меню, по убыванию.
	scores, err := queries.SimilarityScores(ctx)
	if err != nil {
		return fmt.Errorf("SimilarityScores: %w", err)
	}
	fmt.Println("1) similarity(name, 'capucino') — схожесть по триграммам (опечатка в Cappuccino):")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "НАЗВАНИЕ\tSIMILARITY")
	for _, s := range scores {
		fmt.Fprintf(w, "%s\t%s\n", s.Name, s.Sim)
	}
	if err := w.Flush(); err != nil {
		return err
	}

	// 2) оператор % — порог схожести (default 0.3): «возможно, вы имели в виду».
	dym, err := queries.DidYouMean(ctx)
	if err != nil {
		return fmt.Errorf("DidYouMean: %w", err)
	}
	fmt.Println("\n2) name % 'capucino' — выше порога 0.3 (did-you-mean):")
	for _, r := range dym {
		fmt.Printf("   %s (similarity %s)\n", r.Name, r.Sim)
	}

	// 3) ускоренный ILIKE: подстрока в середине, которую B-tree не ускоряет.
	like, err := queries.AcceleratedLike(ctx)
	if err != nil {
		return fmt.Errorf("AcceleratedLike: %w", err)
	}
	fmt.Printf("\n3) name ILIKE '%s' — подстрока в середине, ускоряется trgm-GIN:\n", "%presso%")
	fmt.Printf("   %s\n", strings.Join(like, ", "))

	return nil
}

// schemaDDL читает schema.sql юнита (CREATE EXTENSION + menu_search_lab + seed).
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
