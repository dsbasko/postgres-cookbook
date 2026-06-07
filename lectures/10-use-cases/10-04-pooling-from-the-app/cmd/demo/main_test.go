package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
)

func newTestPool(t *testing.T, opts ...pg.Option) (*pgxpool.Pool, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	pool, err := pg.NewPool(ctx, opts...)
	if err != nil {
		t.Fatalf("pg.NewPool: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := pool.Ping(ctx); err != nil {
		t.Skipf("sandbox not reachable, skipping integration test: %v", err)
	}
	return pool, ctx
}

// TestSessionLockLeaksAcrossBackends — session-level advisory-лок виден всем
// бэкендам, но снять его может только взявший: отпуск с другого коннекта → false.
func TestSessionLockLeaksAcrossBackends(t *testing.T) {
	pool, ctx := newTestPool(t, pg.WithMaxConns(4))

	connA, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire A: %v", err)
	}
	defer connA.Release()
	connB, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire B: %v", err)
	}
	defer connB.Release()

	const key = 4242
	if _, err := connA.Exec(ctx, `SELECT pg_advisory_lock($1)`, key); err != nil {
		t.Fatalf("A lock: %v", err)
	}
	// Session-лок и так умрёт вместе с коннектом при закрытии пула — отдельная
	// очистка не нужна (и нельзя дёргать уже released-коннект).

	var tryB, unlockB bool
	if err := connB.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&tryB); err != nil {
		t.Fatalf("B try: %v", err)
	}
	if tryB {
		t.Error("B acquired a lock already held by A — should be false")
	}
	if err := connB.QueryRow(ctx, `SELECT pg_advisory_unlock($1)`, key).Scan(&unlockB); err != nil {
		t.Fatalf("B unlock: %v", err)
	}
	if unlockB {
		t.Error("B released a lock it does not own — should be false")
	}
}

// TestXactLockAutoReleases — transaction-scoped лок держится в транзакции и
// снимается сам на COMMIT (безопасен под транзакционным пулингом).
func TestXactLockAutoReleases(t *testing.T) {
	pool, ctx := newTestPool(t)
	heldIn, heldAfter, err := xactLockProbe(ctx, pool)
	if err != nil {
		t.Fatalf("xactLockProbe: %v", err)
	}
	if !heldIn {
		t.Error("xact lock should be held inside the transaction")
	}
	if heldAfter {
		t.Error("xact lock should be released after COMMIT")
	}
}

// TestDedicatedConnReceivesNotify — выделенный коннект, сам сделавший LISTEN,
// получает NOTIFY; коннект без LISTEN — нет.
func TestDedicatedConnReceivesNotify(t *testing.T) {
	pool, ctx := newTestPool(t, pg.WithMaxConns(4))

	listener, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire listener: %v", err)
	}
	defer listener.Release()
	if _, err := listener.Exec(ctx, "LISTEN "+notifyChannel); err != nil {
		t.Fatalf("LISTEN: %v", err)
	}
	if _, err := pool.Exec(ctx, `SELECT pg_notify($1, $2)`, notifyChannel, "hello"); err != nil {
		t.Fatalf("NOTIFY: %v", err)
	}
	got, err := waitNotify(ctx, listener, 2*time.Second)
	if err != nil {
		t.Fatalf("dedicated wait: %v", err)
	}
	if got != "hello" {
		t.Errorf("payload = %q, want hello", got)
	}

	other, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire other: %v", err)
	}
	defer other.Release()
	if _, err := pool.Exec(ctx, `SELECT pg_notify($1, $2)`, notifyChannel, "miss"); err != nil {
		t.Fatalf("NOTIFY 2: %v", err)
	}
	_, err = waitNotify(ctx, other, 500*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("non-listening conn err = %v, want DeadlineExceeded (heard nothing)", err)
	}
}

// TestSimpleProtocolQueryWorks — пул в режиме простого протокола исполняет
// параметрический запрос без prepared-кэша на бэкенде.
func TestSimpleProtocolQueryWorks(t *testing.T) {
	pool, ctx := newTestPool(t, func(c *pgxpool.Config) {
		c.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	})
	if err := brew.Reset(ctx, pool); err != nil {
		t.Fatalf("brew.Reset: %v", err)
	}
	var name string
	if err := pool.QueryRow(ctx, `SELECT name FROM drinks WHERE id = $1`, 1).Scan(&name); err != nil {
		t.Fatalf("simple-protocol query: %v", err)
	}
	if name != "Эспрессо" {
		t.Errorf("name = %q, want Эспрессо", name)
	}
}
