// Команда demo юнита 07-01: доступ к jsonb и containment.
//
// Два режима:
//
//	demo          — операторы -> ->> #>> над options, затем @> и ? как фильтры;
//	demo -reset   — накатить канон Brew + order_options_lab и выйти (db-reset).
//
// Данные лабораторной таблицы фиксированы (seed в schema.sql) → вывод
// детерминирован. Логи — в stderr, stdout — только результат (для дословной
// вставки в README).
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

	"github.com/dsbasko/postgres-cookbook/lectures/07-jsonb-arrays-and-search/07-01-jsonb-access-and-containment/internal/db"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

func main() {
	logger := log.New()

	reset := flag.Bool("reset", false, "накатить канон Brew + order_options_lab и выйти")
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
		fmt.Println("Канон Brew + order_options_lab накатаны.")
		return nil
	}

	queries := db.New(pool)

	// 1) Доступ: -> (jsonb, в кавычках) vs ->> (text), #>> по пути в массив.
	rows, err := queries.AccessOps(ctx)
	if err != nil {
		return fmt.Errorf("AccessOps: %w", err)
	}
	fmt.Println("1) Доступ к полям options (-> даёт jsonb с кавычками, ->> — text, #>> — по пути):")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "КЛИЕНТ\t-> 'milk'\t->> 'milk'\tsize\tshots\t#>> '{extras,0}'")
	for _, r := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			r.Customer, r.MilkJsonb, r.MilkText, r.Size, r.Shots, r.FirstExtra)
	}
	if err := w.Flush(); err != nil {
		return err
	}

	// 2) Containment @> '{"milk":"oat"}' — пара ключ-значение в любом месте документа.
	oat, err := queries.OatMilkOrders(ctx)
	if err != nil {
		return fmt.Errorf("OatMilkOrders: %w", err)
	}
	fmt.Println("\n2) options @> '{\"milk\":\"oat\"}' — заказы на овсяном молоке:")
	for _, r := range oat {
		fmt.Printf("   %s (size %s)\n", r.Customer, r.Size)
	}

	// 3) Containment заглядывает в массив: @> '{"extras":["honey"]}'.
	honey, err := queries.HoneyInExtras(ctx)
	if err != nil {
		return fmt.Errorf("HoneyInExtras: %w", err)
	}
	fmt.Println("\n3) options @> '{\"extras\":[\"honey\"]}' — в массиве extras есть honey:")
	for _, r := range honey {
		fmt.Printf("   %s\n", r)
	}

	// 4) ? 'extras' — наличие ключа (даже если массив пуст, как у Дины).
	keyed, err := queries.HasExtrasKey(ctx)
	if err != nil {
		return fmt.Errorf("HasExtrasKey: %w", err)
	}
	fmt.Println("\n4) options ? 'extras' — указан ключ extras (пустой массив тоже считается):")
	for _, r := range keyed {
		fmt.Printf("   %s\n", r)
	}

	return nil
}

// schemaDDL читает schema.sql юнита (DDL+seed поверх канона: order_options_lab).
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
