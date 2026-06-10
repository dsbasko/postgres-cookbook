// Команда demo юнита 00-01: первый контакт с песочницей Brew.
//
// Два режима:
//
//	demo          — подключиться, спросить версию сервера и сделать перепись мира
//	                Brew (сколько строк в каждой таблице канона);
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

	"github.com/dsbasko/postgres-cookbook/lectures/00-getting-connected/00-01-meet-brew/internal/db"
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
	world, err := queries.BrewWorld(ctx)
	if err != nil {
		return fmt.Errorf("BrewWorld: %w", err)
	}

	fmt.Printf("Сервер: %s\n\n", version)
	fmt.Println("Мир Brew — 9 таблиц канона. Что лежит в них после seed:")
	fmt.Println()

	// tabwriter выравнивает колонки по самому широкому значению (по числу рун, так
	// что кириллица в заголовке считается корректно).
	var total int64
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ТАБЛИЦА\tСТРОК")
	for _, row := range world {
		fmt.Fprintf(w, "%s\t%d\n", row.Entity, row.N)
		total += row.N
	}
	if err := w.Flush(); err != nil {
		return err
	}

	fmt.Printf("\nИтого %d строки — на этих данных поедет весь курс.\n", total)
	return nil
}
