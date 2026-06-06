// Команда demo юнита 05-05: ретрай на 40001 — приложение под SERIALIZABLE
// обязано повторять транзакцию при serialization_failure.
//
// Два режима:
//
//	demo          — withRetry прогоняет транзакцию Алисы: попытка 1 ловит 40001, ретрай на свежем снимке успешен;
//	demo -reset   — накатить канон Brew (схема + seed) и выйти (db-reset).
//
// Это raw-pgx юнит (escape-hatch): урок про управляющую логику Go (ретрай-петля,
// разбор кода ошибки pgconn), sqlc тут не нужен. Конфликт 40001 мы создаём
// ДЕТЕРМИНИРОВАННО: на первой попытке синхронно вклиниваем коммит второго
// бариста через ОТДЕЛЬНУЮ транзакцию — в проде это сделал бы другой инстанс
// приложения в тот же момент. Лабораторный стол shift_lab пересоздаётся в начале.
// Логи — в stderr, stdout — только результат (для дословной вставки в README).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

// serializationFailure — SQLSTATE 40001. Под SERIALIZABLE база завершает этим
// кодом транзакцию, которую нельзя «выстроить в очередь» с другими (см. 05-04).
// Контракт уровня: лови этот код и ПОВТОРИ транзакцию.
const serializationFailure = "40001"

const maxAttempts = 5

func main() {
	logger := log.New()

	reset := flag.Bool("reset", false, "накатить канон Brew (схема + seed) и выйти")
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
	fmt.Println("1) shift_lab: на полу 2 бариста (Алиса #1, Борис #2). Правило: на полу всегда ≥1.")

	// Алиса решает, может ли уйти с пола, под SERIALIZABLE с ретраями. Логика
	// одной транзакции: прочитать «сколько на полу», и если ≥2 — уйти (снять
	// свой флаг id=1). На ПЕРВОЙ попытке мы синхронно вклиниваем уход Бориса
	// (отдельная транзакция, коммитит первой) — это и создаёт конфликт 40001.
	fmt.Println("\n2) Алиса решает, может ли уйти — транзакция SERIALIZABLE с ретраями:")
	injected := false
	attempts, err := withRetry(ctx, pool, func(ctx context.Context, tx pgx.Tx, attempt int) error {
		var onFloor int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM shift_lab WHERE on_floor`).Scan(&onFloor); err != nil {
			return err
		}

		// Детерминированный конфликт: ровно один раз, на первой попытке, после
		// того как Алиса сняла свой снимок, Борис уходит и коммитит первым.
		if !injected {
			injected = true
			if err := borisStepsOff(ctx, pool); err != nil {
				return fmt.Errorf("инъекция конфликта: %w", err)
			}
			fmt.Println("   (параллельно: Борис ушёл с пола и закоммитил — конфликт назревает)")
		}

		if onFloor >= 2 {
			fmt.Printf("   попытка %d: на полу %d (на момент чтения) → можно уйти, снимаю свой флаг\n", attempt, onFloor)
			if _, err := tx.Exec(ctx, `UPDATE shift_lab SET on_floor = false WHERE id = 1`); err != nil {
				return err
			}
		} else {
			fmt.Printf("   попытка %d: на полу %d → уходить нельзя (на полу ≤1), остаюсь\n", attempt, onFloor)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("транзакция Алисы не удалась: %w", err)
	}
	fmt.Printf("   ✓ COMMIT успешен (заняло попыток: %d)\n", attempts)

	// Итог: на полу остался один бариста — инвариант сохранён. Ретрай прочитал
	// свежий снимок (Борис уже ушёл) и принял верное решение: Алиса осталась.
	fmt.Println("\n3) Итог: на полу 1 бариста — инвариант сохранён.")
	fmt.Println("   Ретрай прочитал свежий снимок и принял верное решение (Алиса осталась):")
	if err := dumpFloor(ctx, pool); err != nil {
		return err
	}

	return nil
}

// withRetry прогоняет txFn в транзакции SERIALIZABLE и ПОВТОРЯЕТ её при 40001
// (serialization_failure). Возвращает номер удачной попытки. Это и есть та
// петля, без которой SERIALIZABLE применять нельзя (см. 05-04). На каждой
// попытке — свежая транзакция и, значит, свежий снимок: повтор видит уже
// зафиксированные изменения конкурентов и может решить иначе.
func withRetry(ctx context.Context, pool *pgxpool.Pool, txFn func(context.Context, pgx.Tx, int) error) (int, error) {
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			return attempt, fmt.Errorf("BeginTx: %w", err)
		}

		// Сама работа транзакции; COMMIT отдельно, т.к. 40001 часто прилетает
		// именно на COMMIT (как мы видели в 05-04).
		err = txFn(ctx, tx, attempt)
		if err == nil {
			err = tx.Commit(ctx)
		}
		if err == nil {
			return attempt, nil
		}

		_ = tx.Rollback(ctx) // после неудачи всегда откатываем (на закоммиченной — no-op)

		if isSerializationFailure(err) {
			// 40001 может прилететь на конфликтующей команде ИЛИ на COMMIT —
			// для ретрая это неважно, ловим в обоих местах.
			fmt.Printf("   ↻ транзакция упала: %s (serialization_failure) — повторяю на свежем снимке\n", serializationFailure)
			continue // ретрай: новая транзакция, новый снимок
		}
		return attempt, err // не-ретрайная ошибка — пробрасываем
	}
	return maxAttempts, fmt.Errorf("исчерпаны %d попыток ретрая", maxAttempts)
}

// isSerializationFailure сообщает, что ошибка — это SQLSTATE 40001 от Postgres.
// Именно так в pgx достают код ошибки сервера: errors.As до *pgconn.PgError.
func isSerializationFailure(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == serializationFailure
}

// borisStepsOff — «другой инстанс приложения»: в ОТДЕЛЬНОЙ транзакции
// SERIALIZABLE читает то же множество (на полу), снимает свой флаг (id=2) и
// коммитит первым. Это создаёт пару read/write-зависимостей с транзакцией
// Алисы — ту самую «опасную структуру», из-за которой COMMIT Алисы упадёт 40001.
func borisStepsOff(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM shift_lab WHERE on_floor`).Scan(&n); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE shift_lab SET on_floor = false WHERE id = 2`); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// setupLab создаёт лабораторный стол и сажает двух барист на пол.
// CREATE TABLE IF NOT EXISTS + TRUNCATE + seed делают функцию идемпотентной.
func setupLab(ctx context.Context, pool *pgxpool.Pool) error {
	const ddl = `
		CREATE TABLE IF NOT EXISTS shift_lab (
			id       int     PRIMARY KEY,
			name     text    NOT NULL,
			on_floor boolean NOT NULL
		);
		TRUNCATE shift_lab;
		INSERT INTO shift_lab (id, name, on_floor) VALUES (1, 'Алиса', true), (2, 'Борис', true);`
	_, err := pool.Exec(ctx, ddl)
	return err
}

// dumpFloor печатает обоих барист и их присутствие на полу.
func dumpFloor(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx, `SELECT id, name, on_floor FROM shift_lab ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var name string
		var onFloor bool
		if err := rows.Scan(&id, &name, &onFloor); err != nil {
			return err
		}
		mark := "нет"
		if onFloor {
			mark = "да"
		}
		fmt.Printf("   #%d %-6s на полу: %s\n", id, name, mark)
	}
	return rows.Err()
}
