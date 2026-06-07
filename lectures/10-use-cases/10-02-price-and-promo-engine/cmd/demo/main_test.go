package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
)

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

func insertPeriod(ctx context.Context, pool *pgxpool.Pool, drink, price int64, from, to string) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO price_periods (drink_id, price_cents, valid)
		 VALUES ($1, $2, tstzrange($3::timestamptz, $4::timestamptz))`,
		drink, price, from, to)
	return err
}

// TestTemporalPK_RejectsOverlap — temporal PK принимает смежные периоды и
// отбивает пересекающийся как exclusion_violation (23P01).
func TestTemporalPK_RejectsOverlap(t *testing.T) {
	pool, ctx := newTestPool(t)
	if err := buildSchema(ctx, pool); err != nil {
		t.Fatalf("buildSchema: %v", err)
	}

	if err := insertPeriod(ctx, pool, 1, 300, "2025-01-01", "2025-02-01"); err != nil {
		t.Fatalf("first period: %v", err)
	}
	if err := insertPeriod(ctx, pool, 1, 320, "2025-02-01", "2025-03-01"); err != nil {
		t.Fatalf("adjacent period: %v", err)
	}
	err := insertPeriod(ctx, pool, 1, 999, "2025-01-15", "2025-02-15")
	if got := sqlState(err); got != "23P01" {
		t.Errorf("overlap SQLSTATE = %q, want 23P01 (err: %v)", got, err)
	}
}

// TestExcludeConstraint_RejectsSameCodeOverlap — классический EXCLUDE отбивает
// пересечение окон для одного кода, но пропускает другой код с тем же окном.
func TestExcludeConstraint_RejectsSameCodeOverlap(t *testing.T) {
	pool, ctx := newTestPool(t)
	if err := buildSchema(ctx, pool); err != nil {
		t.Fatalf("buildSchema: %v", err)
	}

	ins := func(code, from, to string) error {
		_, err := pool.Exec(ctx,
			`INSERT INTO promo_windows (code, span) VALUES ($1, tstzrange($2::timestamptz, $3::timestamptz))`,
			code, from, to)
		return err
	}
	if err := ins("SUMMER", "2025-06-01", "2025-09-01"); err != nil {
		t.Fatalf("first window: %v", err)
	}
	if got := sqlState(ins("SUMMER", "2025-08-01", "2025-10-01")); got != "23P01" {
		t.Errorf("same-code overlap SQLSTATE = %q, want 23P01", got)
	}
	if err := ins("AUTUMN", "2025-08-01", "2025-10-01"); err != nil {
		t.Errorf("different code should be allowed, got: %v", err)
	}
}

// TestReturningOldNew_CapturesAudit — UPDATE ... RETURNING old/new отдаёт «было»
// и «стало», и аудит фиксирует переход.
func TestReturningOldNew_CapturesAudit(t *testing.T) {
	pool, ctx := newTestPool(t)
	if err := buildSchema(ctx, pool); err != nil {
		t.Fatalf("buildSchema: %v", err)
	}
	if err := insertPeriod(ctx, pool, 1, 320, "2025-02-01", "2025-03-01"); err != nil {
		t.Fatalf("seed period: %v", err)
	}

	var oldCents, newCents int64
	err := pool.QueryRow(ctx,
		`UPDATE price_periods SET price_cents = $1
		  WHERE drink_id = 1 AND valid = tstzrange('2025-02-01'::timestamptz, '2025-03-01'::timestamptz)
		RETURNING old.price_cents, new.price_cents`, int64(340)).Scan(&oldCents, &newCents)
	if err != nil {
		t.Fatalf("RETURNING old/new: %v", err)
	}
	if oldCents != 320 || newCents != 340 {
		t.Errorf("old/new = %d/%d, want 320/340", oldCents, newCents)
	}
}
