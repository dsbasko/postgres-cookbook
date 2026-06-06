// Команда demo юнита 03-05: PG18 RETURNING old/new — старое и новое значение
// строки одной командой.
//
// Два режима:
//
//	demo          — UPDATE/INSERT/DELETE ... RETURNING old.*, new.* на трёх примерах;
//	demo -reset   — накатить канон Brew (schema + seed) и выйти (db-reset).
//
// Это raw-pgx юнит (escape-hatch): парсер sqlc v1.30.0 ещё не понимает PG18-
// синтаксис `RETURNING old.col, new.col` (падает на "column does not exist"), а
// урок именно про него — поэтому пишем запросы строкой и сканируем сами. Демо
// работает на своём лабораторном столе order_status_lab (канон не трогает),
// который пересоздаётся в начале — вывод детерминирован. Логи — в stderr,
// stdout — только результат (для дословной вставки в README).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

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

	if err := setupLab(ctx, pool); err != nil {
		return fmt.Errorf("setupLab: %w", err)
	}
	fmt.Println("1) Стол order_status_lab засеян: заказы #1, #2, #3 в статусе 'created'.")

	// 2) UPDATE: переход #1 created → paid. old.* — строка ДО, new.* — ПОСЛЕ,
	// обе в одном запросе. На UPDATE существуют обе версии строки.
	var oldStatus, newStatus string
	var wasUnpaid, nowPaid bool
	err = pool.QueryRow(ctx, `
		UPDATE order_status_lab
		SET status = 'paid', paid_at = now()
		WHERE id = 1
		RETURNING old.status, new.status,
		          (old.paid_at IS NULL)     AS was_unpaid,
		          (new.paid_at IS NOT NULL) AS now_paid`,
	).Scan(&oldStatus, &newStatus, &wasUnpaid, &nowPaid)
	if err != nil {
		return fmt.Errorf("UPDATE RETURNING old/new: %w", err)
	}
	fmt.Println("\n2) UPDATE #1: created → paid (RETURNING old/new одним запросом):")
	fmt.Printf("   old.status=%s  new.status=%s   было неоплачено=%v  стало оплачено=%v\n",
		oldStatus, newStatus, wasUnpaid, nowPaid)

	// 3) INSERT: строки «до» нет, поэтому old.* пуст (NULL), new.* — вставленное.
	oldS, newS, err := scanOldNew(ctx, pool, `
		INSERT INTO order_status_lab (id, status) VALUES (4, 'created')
		RETURNING old.status, new.status`)
	if err != nil {
		return fmt.Errorf("INSERT RETURNING old/new: %w", err)
	}
	fmt.Println("\n3) INSERT #4 'created' (RETURNING old/new):")
	fmt.Printf("   old.status=%s  new.status=%s   → на INSERT строки «до» нет, old.* пуст\n",
		render(oldS), render(newS))

	// 4) DELETE: строки «после» нет, поэтому new.* пуст (NULL), old.* — удалённое.
	oldS, newS, err = scanOldNew(ctx, pool, `
		DELETE FROM order_status_lab WHERE id = 2
		RETURNING old.status, new.status`)
	if err != nil {
		return fmt.Errorf("DELETE RETURNING old/new: %w", err)
	}
	fmt.Println("\n4) DELETE #2 (RETURNING old/new):")
	fmt.Printf("   old.status=%s  new.status=%s   → на DELETE строки «после» нет, new.* пуст\n",
		render(oldS), render(newS))

	return nil
}

// setupLab создаёт лабораторный стол и засевает три заказа в статусе 'created'.
// CREATE TABLE IF NOT EXISTS + TRUNCATE делают функцию идемпотентной.
func setupLab(ctx context.Context, pool *pgxpool.Pool) error {
	const ddl = `
		CREATE TABLE IF NOT EXISTS order_status_lab (
			id      BIGINT       PRIMARY KEY,
			status  TEXT         NOT NULL,
			paid_at TIMESTAMPTZ
		);
		TRUNCATE order_status_lab;
		INSERT INTO order_status_lab (id, status) VALUES (1, 'created'), (2, 'created'), (3, 'created');`
	_, err := pool.Exec(ctx, ddl)
	return err
}

// scanOldNew исполняет запрос с RETURNING old.status, new.status и сканирует обе
// колонки в *string (на INSERT/DELETE одна из них NULL).
func scanOldNew(ctx context.Context, pool *pgxpool.Pool, sql string) (oldS, newS *string, err error) {
	err = pool.QueryRow(ctx, sql).Scan(&oldS, &newS)
	return oldS, newS, err
}

// render печатает значение или ∅ для NULL — чтобы «пустую» сторону old/new было
// видно в выводе.
func render(s *string) string {
	if s == nil {
		return "∅"
	}
	return *s
}
