package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
)

// newTestPool строит пул к песочнице и пропускает тест, если БД недоступна —
// `go test ./...` остаётся зелёным без поднятого docker compose.
func newTestPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	pool, err := pg.NewPool(ctx)
	if err != nil {
		t.Fatalf("pg.NewPool: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := pool.Ping(ctx); err != nil {
		t.Skipf("sandbox not reachable, skipping integration test: %v", err)
	}
	return pool, ctx
}

func sqlState(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

// TestCapstone_SchemaAndCRUD — схема собирается, CRUD с RETURNING наполняет её.
func TestCapstone_SchemaAndCRUD(t *testing.T) {
	pool, ctx := newTestPool(t)

	if err := buildSchema(ctx, pool); err != nil {
		t.Fatalf("buildSchema: %v", err)
	}
	for _, tbl := range []string{"cap_members", "cap_ledger"} {
		var reg *string
		if err := pool.QueryRow(ctx, "SELECT to_regclass($1)::text", tbl).Scan(&reg); err != nil {
			t.Fatalf("to_regclass(%s): %v", tbl, err)
		}
		if reg == nil {
			t.Errorf("table %q not created", tbl)
		}
	}

	if err := seedMembers(ctx, pool); err != nil {
		t.Fatalf("seedMembers: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM cap_members").Scan(&n); err != nil {
		t.Fatalf("count members: %v", err)
	}
	if n != 3 {
		t.Errorf("members = %d, want 3", n)
	}
	var tier string
	if err := pool.QueryRow(ctx,
		"SELECT tier FROM cap_members WHERE email = 'alice@brew.example'").Scan(&tier); err != nil {
		t.Fatalf("alice tier: %v", err)
	}
	if tier != "gold" {
		t.Errorf("alice tier = %q, want gold", tier)
	}
}

// TestCapstone_ConstraintsRejectGarbage — каждое ограничение отбивает свой класс
// мусора с ожидаемым SQLSTATE.
func TestCapstone_ConstraintsRejectGarbage(t *testing.T) {
	pool, ctx := newTestPool(t)
	if err := buildSchema(ctx, pool); err != nil {
		t.Fatalf("buildSchema: %v", err)
	}
	if err := seedMembers(ctx, pool); err != nil {
		t.Fatalf("seedMembers: %v", err)
	}

	cases := []struct {
		name string
		sql  string
		args []any
		want string // SQLSTATE
	}{
		{"dup email (UNIQUE)", `INSERT INTO cap_members (email, tier) VALUES ($1, 'bronze')`, []any{"alice@brew.example"}, "23505"},
		{"bad tier (CHECK)", `INSERT INTO cap_members (email, tier) VALUES ($1, 'platinum')`, []any{"dave@brew.example"}, "23514"},
		{"negative balance (CHECK)", `INSERT INTO cap_members (email, balance_cents) VALUES ($1, -100)`, []any{"erin@brew.example"}, "23514"},
		{"dangling FK", `INSERT INTO cap_ledger (member_id, delta_cents, reason) VALUES (999, 100, 'bonus')`, nil, "23503"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := pool.Exec(ctx, c.sql, c.args...)
			if got := sqlState(err); got != c.want {
				t.Errorf("SQLSTATE = %q, want %q (err: %v)", got, c.want, err)
			}
		})
	}
}

// TestCapstone_IndexChangesPlan — точечная выборка идёт Seq Scan'ом без индекса
// и перестаёт после CREATE INDEX (план берётся из EXPLAIN, без прогона).
func TestCapstone_IndexChangesPlan(t *testing.T) {
	pool, ctx := newTestPool(t)
	if err := buildSchema(ctx, pool); err != nil {
		t.Fatalf("buildSchema: %v", err)
	}
	if err := seedMembers(ctx, pool); err != nil {
		t.Fatalf("seedMembers: %v", err)
	}
	if err := showIndexPlan(ctx, pool); err != nil {
		t.Fatalf("showIndexPlan: %v", err)
	}

	// После showIndexPlan индекс уже создан — план не должен быть Seq Scan.
	const lookup = `SELECT delta_cents FROM cap_ledger WHERE member_id = 2`
	after, err := topPlanNode(ctx, pool, lookup)
	if err != nil {
		t.Fatalf("topPlanNode: %v", err)
	}
	if !strings.Contains(after, "Index") {
		t.Errorf("after index plan = %q, want an Index node", after)
	}
}

// TestCapstone_RetrySurvives40001 — withRetry переживает инъецированный
// serialization_failure: ≥2 попытки и баланс сходится (старт +1.00 проценты
// +5.00 бонус).
func TestCapstone_RetrySurvives40001(t *testing.T) {
	pool, ctx := newTestPool(t)
	if err := buildSchema(ctx, pool); err != nil {
		t.Fatalf("buildSchema: %v", err)
	}
	if err := seedMembers(ctx, pool); err != nil {
		t.Fatalf("seedMembers: %v", err)
	}

	var start int64
	if err := pool.QueryRow(ctx, "SELECT balance_cents FROM cap_members WHERE id = 1").Scan(&start); err != nil {
		t.Fatalf("start balance: %v", err)
	}

	const bonus = int64(500)
	injected := false
	attempts, err := withRetry(ctx, pool, func(ctx context.Context, tx pgx.Tx, attempt int) error {
		var balance int64
		if err := tx.QueryRow(ctx, "SELECT balance_cents FROM cap_members WHERE id = 1").Scan(&balance); err != nil {
			return err
		}
		if !injected {
			injected = true
			if err := nightlyInterest(ctx, pool); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, "UPDATE cap_members SET balance_cents = $1 WHERE id = 1", balance+bonus)
		return err
	})
	if err != nil {
		t.Fatalf("withRetry: %v", err)
	}
	if attempts < 2 {
		t.Errorf("attempts = %d, want >= 2 (conflict should have forced a retry)", attempts)
	}

	var end int64
	if err := pool.QueryRow(ctx, "SELECT balance_cents FROM cap_members WHERE id = 1").Scan(&end); err != nil {
		t.Fatalf("end balance: %v", err)
	}
	if want := start + 100 + bonus; end != want {
		t.Errorf("final balance = %d, want %d", end, want)
	}
}
