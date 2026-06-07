// Команда demo юнита 09-04: LISTEN / NOTIFY.
//
// Два режима:
//
//	demo          — выделенное соединение слушает канал brew_events; триггер
//	                AFTER INSERT шлёт pg_notify; показываем две оговорки — NOTIFY
//	                придерживается до COMMIT (транзакционность) и теряется, если
//	                в момент отправки никто не слушает (at-most-once);
//	demo -reset   — накатить канон Brew (схема + seed) и выйти (db-reset).
//
// Это raw-pgx escape-hatch до sqlc: LISTEN/NOTIFY живут на уровне соединения
// (conn.WaitForNotification), а не как обычный запрос — для sqlc такого нет.
//
// Детерминизм: «до COMMIT уведомления нет» и «без слушателя уведомление
// потеряно» проверяются ОЖИДАНИЕМ с таймаутом — оно всегда истекает, когда
// уведомления действительно нет. Полученный payload фиксирован (id с 1, имя).
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

// waitTimeout — сколько ждём уведомления, прежде чем считать, что его нет.
// Короткое окно: когда уведомление есть, оно приходит мгновенно; когда нет —
// мы детерминированно упираемся в таймаут.
const waitTimeout = 400 * time.Millisecond

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
	// Нужны два соединения одновременно: одно слушает, второе пишет.
	pool, err := pg.NewPool(ctx, pg.WithMaxConns(4))
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
	fmt.Println("1) notify_lab + триггер AFTER INSERT → pg_notify('brew_events', ...) готовы.")

	// Слушатель занимает ВЫДЕЛЕННОЕ соединение: LISTEN привязан к конкретному
	// бэкенду, и читать уведомления надо с того же conn, что выполнял LISTEN.
	lc, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire listener: %w", err)
	}
	defer lc.Release()
	listener := lc.Conn()
	if _, err := listener.Exec(ctx, "LISTEN brew_events"); err != nil {
		return fmt.Errorf("LISTEN: %w", err)
	}
	fmt.Println("2) LISTEN brew_events (выделенное соединение слушает канал).")

	// ── Оговорка 1: транзакционность. NOTIFY придерживается до COMMIT. ──────
	fmt.Println("\n3) Транзакционность: INSERT внутри транзакции — уведомление ждёт COMMIT.")
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO notify_lab (name) VALUES ('Эспрессо')`); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	// Слушаем ДО коммита: триггер уже сработал, но NOTIFY ещё не отпущен.
	if got, err := waitNotify(ctx, listener); err != nil {
		_ = tx.Rollback(ctx)
		return err
	} else if got != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("неожиданно получили уведомление ДО COMMIT: %s", *got)
	}
	fmt.Printf("   до COMMIT: ждём %s... уведомления нет (NOTIFY придержан до COMMIT).\n", waitTimeout)
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	fmt.Println("   COMMIT.")
	// Теперь, после коммита, уведомление приходит.
	got, err := waitNotify(ctx, listener)
	if err != nil {
		return err
	}
	if got == nil {
		return errors.New("ожидали уведомление ПОСЛЕ COMMIT, но его нет")
	}
	fmt.Printf("   после COMMIT: получено уведомление, payload = %s\n", *got)

	// ── Оговорка 2: at-most-once. Без слушателя в момент NOTIFY — потеря. ───
	fmt.Println("\n4) At-most-once: если никто не слушает в момент NOTIFY — уведомление теряется.")
	if _, err := listener.Exec(ctx, "UNLISTEN brew_events"); err != nil {
		return fmt.Errorf("UNLISTEN: %w", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO notify_lab (name) VALUES ('Латте')`); err != nil {
		return err
	}
	fmt.Println("   UNLISTEN brew_events; INSERT 'Латте' (NOTIFY летит в пустоту).")
	if _, err := listener.Exec(ctx, "LISTEN brew_events"); err != nil {
		return fmt.Errorf("re-LISTEN: %w", err)
	}
	lost, err := waitNotify(ctx, listener)
	if err != nil {
		return err
	}
	if lost != nil {
		return fmt.Errorf("неожиданно получили «потерянное» уведомление: %s", *lost)
	}
	fmt.Printf("   LISTEN brew_events снова; ждём %s... уведомления нет (потеряно, NOTIFY не хранится).\n", waitTimeout)

	return nil
}

// waitNotify ждёт уведомление до waitTimeout. Возвращает (payload, nil), если
// пришло; (nil, nil), если истёк таймаут (уведомления нет) — это штатный исход,
// на нём строятся обе оговорки урока. WaitForNotification живёт на уровне
// соединения (pgx.Conn) — потому это и raw-pgx escape-hatch, а не sqlc.
func waitNotify(ctx context.Context, conn *pgx.Conn) (*string, error) {
	wctx, cancel := context.WithTimeout(ctx, waitTimeout)
	defer cancel()

	n, err := conn.WaitForNotification(wctx)
	if err != nil {
		// Истёкший таймаут — это «уведомления нет», штатный исход.
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, nil
		}
		return nil, err
	}
	return &n.Payload, nil
}

// setupLab пересоздаёт лабораторную таблицу, функцию-триггер и сам триггер.
// DROP ... IF EXISTS делает run идемпотентным; IDENTITY с RESTART через
// пересоздание таблицы даёт детерминированные id (первый INSERT → id 1).
func setupLab(ctx context.Context, pool *pgxpool.Pool) error {
	const ddl = `
		DROP TABLE IF EXISTS notify_lab;
		CREATE TABLE notify_lab (
			id   bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
			name text   NOT NULL
		);

		CREATE OR REPLACE FUNCTION notify_lab_notify() RETURNS trigger
		LANGUAGE plpgsql AS $$
		BEGIN
			-- pg_notify шлёт текстовый payload в канал. json_build_object даёт
			-- компактное событие; ::text — потому что payload канала это строка.
			PERFORM pg_notify('brew_events',
				json_build_object('id', NEW.id, 'name', NEW.name)::text);
			RETURN NEW;
		END;
		$$;

		CREATE OR REPLACE TRIGGER notify_lab_ains
			AFTER INSERT ON notify_lab
			FOR EACH ROW EXECUTE FUNCTION notify_lab_notify();`
	_, err := pool.Exec(ctx, ddl)
	return err
}
