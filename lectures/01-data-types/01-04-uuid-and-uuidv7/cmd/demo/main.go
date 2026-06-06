// Команда demo юнита 01-04: uuid как ключ — случайный v4 против PG18 uuidv7.
//
// Два режима:
//
//	demo          — версии v4/v7, встроенное время, монотонность uuidv7 как ключа;
//	demo -reset   — накатить канон Brew + таблицу loyalty_signups и выйти (db-reset).
//
// Значения uuid случайны и в README не печатаются — показываем проверяемые
// свойства. Логи — в stderr, stdout — только результат (для вставки в README).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/dsbasko/postgres-cookbook/lectures/01-data-types/01-04-uuid-and-uuidv7/internal/db"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

func main() {
	logger := log.New()

	reset := flag.Bool("reset", false, "накатить канон Brew + loyalty_signups и выйти")
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
		// brew.Apply: канон → DDL юнита (loyalty_signups) → seed. extraDDL —
		// содержимое schema.sql этого юнита.
		ddl, err := schemaDDL()
		if err != nil {
			return err
		}
		if err := brew.Apply(ctx, pool, ddl); err != nil {
			return fmt.Errorf("brew.Apply: %w", err)
		}
		fmt.Println("Канон Brew + таблица loyalty_signups накатаны.")
		return nil
	}

	queries := db.New(pool)

	// 1) проверяемые факты о версиях uuid.
	facts, err := queries.UUIDFacts(ctx)
	if err != nil {
		return fmt.Errorf("UUIDFacts: %w", err)
	}
	fmt.Println("1) gen_random_uuid() (v4) против uuidv7() — проверяемые свойства:")
	fmt.Printf("   версия:           v4 = %d,  v7 = %d\n", facts.V4Version, facts.V7Version)
	fmt.Printf("   встроено время?   v4: нет (timestamp = NULL) = %v;  v7: да = %v\n",
		facts.V4HasNoTimestamp, facts.V7HasTimestamp)

	// 2) uuidv7 как сортируемый по времени ключ: вставим три строки и проверим,
	// что порядок по id совпадает с порядком вставки (seq).
	if err := queries.TruncateSignups(ctx); err != nil {
		return fmt.Errorf("TruncateSignups: %w", err)
	}
	for _, email := range []string{"alice@brew.example", "bob@brew.example", "carol@brew.example"} {
		if _, err := queries.InsertSignup(ctx, email); err != nil {
			return fmt.Errorf("InsertSignup: %w", err)
		}
	}
	ord, err := queries.SignupsTimeOrdered(ctx)
	if err != nil {
		return fmt.Errorf("SignupsTimeOrdered: %w", err)
	}
	fmt.Printf("\n2) Вставили строк с ключом uuidv7: %d. Порядок по id = порядку вставки? %v\n",
		ord.N, ord.IdsMatchInsertionOrder)
	fmt.Println("   → uuidv7 монотонен во времени: годится как сортируемый по времени PK.")
	fmt.Println("     (v4 случаен — такой порядок был бы лишь совпадением.)")

	return nil
}

// schemaDDL читает schema.sql юнита (DDL поверх канона: таблица loyalty_signups).
// Путь резолвится через runtime.Caller относительно этого исходника — курс
// всегда запускается из исходников (go run / go test), так что функция не
// зависит от рабочего каталога вызывающего (go:embed сюда не дотянется: файл
// лежит на два уровня выше cmd/demo/).
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
