// Команда demo юнита 07-03: SQL/JSON path и сборка jsonb.
//
// Два режима:
//
//	demo          — jsonpath-запросы (_array/_first, фильтр ? (@.x>N)), предикаты
//	                @?/@@, точечная правка jsonb_set, сборка jsonb_agg по каноне;
//	demo -reset   — накатить канон Brew + drink_recipe_lab и выйти (db-reset).
//
// jsonpath-функции и сборочные функции sqlc типизирует как interface{} (он не
// знает их сигнатур из каталога), но pgx возвращает в них конкретные string/bool
// — печатаем через %v. Данные фиксированы → вывод детерминирован. Логи — в
// stderr, stdout — только результат (для дословной вставки в README).
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

	"github.com/dsbasko/postgres-cookbook/lectures/07-jsonb-arrays-and-search/07-03-sql-json-path-and-building/internal/db"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

func main() {
	logger := log.New()

	reset := flag.Bool("reset", false, "накатить канон Brew + drink_recipe_lab и выйти")
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
		fmt.Println("Канон Brew + drink_recipe_lab накатаны.")
		return nil
	}

	queries := db.New(pool)

	// 1) jsonpath: достать поля и отфильтровать элементы массива по условию.
	pq, err := queries.PathQueries(ctx)
	if err != nil {
		return fmt.Errorf("PathQueries: %w", err)
	}
	fmt.Println("1) jsonpath по рецепту «Латте» ($.ingredients[*], фильтр ? (@.grams > 100)):")
	fmt.Printf("   все ингредиенты   $.ingredients[*].name              = %v\n", pq.AllNames)
	fmt.Printf("   тяжёлые (>100 г)   ... ? (@.grams > 100).name         = %v\n", pq.HeavyNames)
	fmt.Printf("   первый             $.ingredients[0].name (first)      = %v\n", pq.FirstName)

	// 2) предикаты пути @? (есть совпадение) и @@ (условие истинно).
	preds, err := queries.PathPredicates(ctx)
	if err != nil {
		return fmt.Errorf("PathPredicates: %w", err)
	}
	fmt.Println("\n2) предикаты пути @? и @@ по всем рецептам:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "НАПИТОК\t@? есть milk\t@@ kcal > 100")
	for _, p := range preds {
		fmt.Fprintf(w, "%s\t%v\t%v\n", p.Name, p.HasMilk, p.Over100Kcal)
	}
	if err := w.Flush(); err != nil {
		return err
	}

	// 3) jsonb_set правит поле и отдаёт НОВЫЙ документ (хранимая строка цела).
	sf, err := queries.SetField(ctx)
	if err != nil {
		return fmt.Errorf("SetField: %w", err)
	}
	fmt.Println("\n3) jsonb_set(recipe, '{kcal}', '130') — правка возвращает новый документ:")
	fmt.Printf("   kcal до = %v, после = %v\n", sf.KcalBefore, sf.KcalAfter)

	// 4) jsonb_agg + jsonb_build_object: собрать меню из канона drinks в один массив.
	menu, err := queries.BuildMenu(ctx)
	if err != nil {
		return fmt.Errorf("BuildMenu: %w", err)
	}
	fmt.Println("\n4) jsonb_agg(jsonb_build_object(...)) — меню канона drinks одним документом:")
	fmt.Printf("   %v\n", menu)

	return nil
}

// schemaDDL читает schema.sql юнита (DDL+seed поверх канона: drink_recipe_lab).
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
