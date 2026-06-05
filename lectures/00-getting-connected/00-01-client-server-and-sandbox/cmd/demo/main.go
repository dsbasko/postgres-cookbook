// Команда demo юнита 00-01: подключиться к песочнице и сделать первые запросы.
//
// Два режима:
//
//	demo          — подключиться, спросить версию сервера и показать меню Brew;
//	demo -reset   — накатить канон Brew (schema + seed) и выйти (цель db-reset).
//
// Поток данных тонкий и одинаковый во всех юнитах курса:
//
//	pg.NewPool → db.New(pool) → типизированный запрос (sqlc) → tabwriter в stdout.
//
// Логи идут в stderr (internal/log), в stdout — только результат запросов, чтобы
// вывод можно было дословно вставить в README (раздел «## Запуск»).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/dsbasko/postgres-cookbook/lectures/00-getting-connected/00-01-client-server-and-sandbox/internal/db"
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
	// Пул ленивый: соединения тут ещё нет — оно установится при первом запросе.
	pool, err := pg.NewPool(ctx)
	if err != nil {
		return fmt.Errorf("pg.NewPool: %w", err)
	}
	defer pool.Close()

	// Ping — первое реальное соединение. Если песочница не поднята, ошибка
	// прилетит здесь, а не в середине запроса.
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

	version, err := queries.ServerVersion(ctx)
	if err != nil {
		return fmt.Errorf("ServerVersion: %w", err)
	}
	count, err := queries.CountDrinks(ctx)
	if err != nil {
		return fmt.Errorf("CountDrinks: %w", err)
	}
	drinks, err := queries.ListDrinks(ctx)
	if err != nil {
		return fmt.Errorf("ListDrinks: %w", err)
	}

	fmt.Printf("Сервер: %s\n", version)
	fmt.Printf("Напитков в меню Brew: %d\n\n", count)

	// tabwriter выравнивает колонки по самому широкому значению — читаемая
	// таблица в stdout без ручного подсчёта пробелов. Цена хранится в центах
	// (BIGINT), печатаем как рубли.копейки.
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSKU\tНАЗВАНИЕ\tКАТЕГОРИЯ\tЦЕНА")
	for _, d := range drinks {
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%d.%02d\n",
			d.ID, d.Sku, d.Name, d.Category, d.BasePrice/100, d.BasePrice%100)
	}
	return w.Flush()
}
