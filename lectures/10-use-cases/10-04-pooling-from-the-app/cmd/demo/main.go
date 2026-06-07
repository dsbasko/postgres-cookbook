// Команда demo юнита 10-04: пулинг из приложения.
//
// Капстон про то, что ломается, когда перед Postgres ставят пул в транзакционном
// режиме (pgbouncer transaction mode), и как с этим жить. Транзакционный пул
// держит на одном бэкенде ровно ОДНУ транзакцию, а не «сессию»: между
// транзакциями он может пересадить тебя на другой бэкенд. Поэтому всё, что живёт
// на уровне сессии (а не транзакции), внезапно перестаёт работать:
//
//   - session-level advisory-локи — взял на одном бэкенде, отпускаешь на другом;
//   - LISTEN/NOTIFY — подписка живёт на конкретном бэкенде;
//   - prepared statements — кэш подготовленных запросов привязан к бэкенду.
//
// Мы воспроизводим это на ЧИСТОМ Postgres, разводя работу по нескольким
// соединениям пула (= разным бэкендам) — ровно так транзакционный пул разносит
// твои операции. Реальный pgbouncer в проде стоял бы спереди (см. заборчик).
//
// Два режима:
//
//	demo          — показать три поломки и их фиксы;
//	demo -reset   — накатить канон Brew (схема + seed) и выйти (db-reset).
//
// raw-pgx escape-hatch (go.mod, без sqlc): урок про управление соединениями
// (Acquire/Release, выделенный коннект, режим протокола) — это API пула, не SQL.
// Логи — в stderr, stdout — только результат (для дословной вставки в README).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

const notifyChannel = "brew_pool"

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
	if reset {
		pool, err := pg.NewPool(ctx)
		if err != nil {
			return fmt.Errorf("pg.NewPool: %w", err)
		}
		defer pool.Close()
		if err := pool.Ping(ctx); err != nil {
			return fmt.Errorf("ping (песочница поднята? `docker compose up -d`): %w", err)
		}
		if err := brew.Reset(ctx, pool); err != nil {
			return fmt.Errorf("brew.Reset: %w", err)
		}
		fmt.Println("Канон Brew накатан: схема + seed-данные на месте.")
		return nil
	}

	// Несколько соединений в пуле → можем держать разные бэкенды одновременно.
	pool, err := pg.NewPool(ctx, pg.WithMaxConns(4))
	if err != nil {
		return fmt.Errorf("pg.NewPool: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping (песочница поднята? `docker compose up -d`): %w", err)
	}
	if err := brew.Reset(ctx, pool); err != nil {
		return fmt.Errorf("brew.Reset: %w", err)
	}

	if err := showAdvisoryLock(ctx, pool); err != nil {
		return fmt.Errorf("advisory: %w", err)
	}
	if err := showListenNotify(ctx, pool); err != nil {
		return fmt.Errorf("listen/notify: %w", err)
	}
	if err := showPreparedStatements(ctx); err != nil {
		return fmt.Errorf("prepared: %w", err)
	}
	return nil
}

// backendPID возвращает pid бэкенда, обслуживающего это соединение.
func backendPID(ctx context.Context, c *pgxpool.Conn) (int, error) {
	var pid int
	err := c.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid)
	return pid, err
}

// showAdvisoryLock демонстрирует, что session-level advisory-лок привязан к
// бэкенду: взяли на A, а «сессию» пул отдал бэкенду B — отпустить нельзя, лок
// течёт. Фикс — transaction-scoped лок: он живёт ровно одну транзакцию (её пул
// держит на одном бэкенде) и сам снимается на COMMIT.
func showAdvisoryLock(ctx context.Context, pool *pgxpool.Pool) error {
	fmt.Println("1) Session advisory-лок привязан к бэкенду (транзакционный пул его ломает)")

	connA, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer connA.Release()
	connB, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer connB.Release()

	pidA, err := backendPID(ctx, connA)
	if err != nil {
		return err
	}
	pidB, err := backendPID(ctx, connB)
	if err != nil {
		return err
	}
	fmt.Printf("   A и B — разные бэкенды: %t\n", pidA != pidB)

	const key = 42
	var ok bool
	if _, err := connA.Exec(ctx, `SELECT pg_advisory_lock($1)`, key); err != nil {
		return err
	}
	fmt.Println("   A: pg_advisory_lock(42) — взял")

	if err := connB.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&ok); err != nil {
		return err
	}
	fmt.Printf("   B: pg_try_advisory_lock(42) → %t (лок виден всем, держит A)\n", ok)

	// B пытается отпустить лок, которого не держит → false (+ WARNING в stderr).
	if err := connB.QueryRow(ctx, `SELECT pg_advisory_unlock($1)`, key).Scan(&ok); err != nil {
		return err
	}
	fmt.Printf("   B: pg_advisory_unlock(42) → %t (не его лок — снять нельзя, лок течёт)\n", ok)

	// Чистим за собой на правильном бэкенде.
	if err := connA.QueryRow(ctx, `SELECT pg_advisory_unlock($1)`, key).Scan(&ok); err != nil {
		return err
	}

	// Фикс: transaction-scoped лок. Снимается сам на COMMIT — пулу всё равно.
	heldIn, heldAfter, err := xactLockProbe(ctx, pool)
	if err != nil {
		return err
	}
	fmt.Printf("   фикс — pg_advisory_xact_lock: держится в транзакции %t, после COMMIT %t (снялся сам)\n",
		heldIn, heldAfter)
	return nil
}

// xactLockProbe берёт transaction-scoped advisory-лок, проверяет, что он держится
// внутри транзакции, коммитит и проверяет, что после COMMIT он снят.
func xactLockProbe(ctx context.Context, pool *pgxpool.Pool) (heldIn, heldAfter bool, err error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return false, false, err
	}
	defer conn.Release()

	tx, err := conn.Begin(ctx)
	if err != nil {
		return false, false, err
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(99)`); err != nil {
		_ = tx.Rollback(ctx)
		return false, false, err
	}
	var inCount int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM pg_locks WHERE locktype = 'advisory'`).Scan(&inCount); err != nil {
		_ = tx.Rollback(ctx)
		return false, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, false, err
	}
	var afterCount int
	if err := conn.QueryRow(ctx,
		`SELECT count(*) FROM pg_locks WHERE locktype = 'advisory'`).Scan(&afterCount); err != nil {
		return false, false, err
	}
	return inCount > 0, afterCount > 0, nil
}

// showListenNotify демонстрирует, что подписка LISTEN живёт на конкретном
// бэкенде. Выделенный коннект, который сам сделал LISTEN, получает уведомление;
// случайный бэкенд из пула, не делавший LISTEN, — не слышит ничего.
func showListenNotify(ctx context.Context, pool *pgxpool.Pool) error {
	fmt.Println("\n2) LISTEN/NOTIFY живёт на бэкенде — нужен выделенный коннект")

	// Выделенный слушатель: держим коннект и сами делаем на нём LISTEN.
	listener, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer listener.Release()
	if _, err := listener.Exec(ctx, "LISTEN "+notifyChannel); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `SELECT pg_notify($1, $2)`, notifyChannel, "order #1"); err != nil {
		return err
	}
	got, err := waitNotify(ctx, listener, 1500*time.Millisecond)
	if err != nil {
		return err
	}
	fmt.Printf("   выделенный коннект (сам делал LISTEN): получил %q\n", got)

	// Случайный бэкенд из пула, который НЕ делал LISTEN, уведомления не услышит.
	other, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer other.Release()
	if _, err := pool.Exec(ctx, `SELECT pg_notify($1, $2)`, notifyChannel, "order #2"); err != nil {
		return err
	}
	_, err = waitNotify(ctx, other, 500*time.Millisecond)
	missed := errors.Is(err, context.DeadlineExceeded)
	fmt.Printf("   коннект без LISTEN (как при пересадке пулом): услышал что-то %t (таймаут — ничего)\n", !missed)
	return nil
}

// waitNotify ждёт уведомление на коннекте не дольше d. Таймаут возвращается как
// context.DeadlineExceeded — это «ничего не пришло», а не сбой.
func waitNotify(ctx context.Context, conn *pgxpool.Conn, d time.Duration) (string, error) {
	wctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	n, err := conn.Conn().WaitForNotification(wctx)
	if err != nil {
		return "", err
	}
	return n.Payload, nil
}

// showPreparedStatements показывает фикс для prepared-statement-кэша под
// транзакционным пулингом: режим простого протокола (pgx не кэширует
// подготовленные запросы на бэкенде, поэтому пересадка между бэкендами не ломает
// запрос). Открываем отдельный пул с этим режимом и гоняем параметрический запрос.
func showPreparedStatements(ctx context.Context) error {
	fmt.Println("\n3) Prepared statements под пулингом → режим простого протокола")

	simple, err := pg.NewPool(ctx, func(c *pgxpool.Config) {
		c.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	})
	if err != nil {
		return err
	}
	defer simple.Close()

	var name string
	if err := simple.QueryRow(ctx,
		`SELECT name FROM drinks WHERE id = $1`, 1).Scan(&name); err != nil {
		return err
	}
	fmt.Printf("   simple protocol: SELECT с параметром вернул %q — без кэша prepared-запросов на бэкенде\n", name)
	fmt.Println("   (по умолчанию pgx кэширует prepared statements per-backend — под транзакционным пулом это ломается)")
	return nil
}
