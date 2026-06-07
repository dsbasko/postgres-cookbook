package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
)

func newTestPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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

// setup applies canon + lab once for the whole package.
func setup(t *testing.T, pool *pgxpool.Pool, ctx context.Context) {
	t.Helper()
	if err := brew.Reset(ctx, pool); err != nil {
		t.Fatalf("brew.Reset: %v", err)
	}
	if err := setupLab(ctx, pool); err != nil {
		t.Fatalf("setupLab: %v", err)
	}
}

// TestKeysetReadsFewerRows — глубокий OFFSET читает весь префикс, keyset — нет.
func TestKeysetReadsFewerRows(t *testing.T) {
	pool, ctx := newTestPool(t)
	setup(t, pool, ctx)

	offsetRows, err := scanActualRows(ctx, pool, `SELECT id FROM events_lab ORDER BY id LIMIT 10 OFFSET 40000`)
	if err != nil {
		t.Fatalf("offset scan: %v", err)
	}
	keysetRows, err := scanActualRows(ctx, pool, `SELECT id FROM events_lab WHERE id > 40000 ORDER BY id LIMIT 10`)
	if err != nil {
		t.Fatalf("keyset scan: %v", err)
	}
	if offsetRows != 40010 {
		t.Errorf("offset scanned %d rows, want 40010", offsetRows)
	}
	if keysetRows != 10 {
		t.Errorf("keyset scanned %d rows, want 10", keysetRows)
	}
	if keysetRows >= offsetRows {
		t.Errorf("keyset (%d) should read far fewer rows than offset (%d)", keysetRows, offsetRows)
	}
}

// TestExpressionIndexFixesNonSargable — lower(email) идёт Seq Scan, после
// expression-индекса — Index-узлом.
func TestExpressionIndexFixesNonSargable(t *testing.T) {
	pool, ctx := newTestPool(t)
	setup(t, pool, ctx)

	const wrapped = `SELECT id FROM accounts_lab WHERE lower(email) = 'user000042@brew.example'`
	before, err := topPlanNode(ctx, pool, wrapped)
	if err != nil {
		t.Fatalf("plan before: %v", err)
	}
	if before != "Seq Scan" {
		t.Errorf("lower(email) before index = %q, want Seq Scan", before)
	}

	if _, err := pool.Exec(ctx, `CREATE INDEX IF NOT EXISTS accounts_lower_email_idx ON accounts_lab (lower(email))`); err != nil {
		t.Fatalf("create expression index: %v", err)
	}
	if _, err := pool.Exec(ctx, `ANALYZE accounts_lab`); err != nil {
		t.Fatalf("analyze: %v", err)
	}
	after, err := topPlanNode(ctx, pool, wrapped)
	if err != nil {
		t.Fatalf("plan after: %v", err)
	}
	if !strings.Contains(after, "Index") {
		t.Errorf("lower(email) after expression index = %q, want an Index node", after)
	}
}

// TestAnyParamMatchesIn — = ANY($1::bigint[]) находит ровно то же, что IN-список.
func TestAnyParamMatchesIn(t *testing.T) {
	pool, ctx := newTestPool(t)
	setup(t, pool, ctx)

	ids := make([]int64, 1000)
	literals := make([]string, 1000)
	for i := range ids {
		ids[i] = int64(i + 1)
		literals[i] = strings.TrimSpace(itoa(int64(i + 1)))
	}

	var inCount, anyCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM events_lab WHERE id IN (`+strings.Join(literals, ",")+`)`).Scan(&inCount); err != nil {
		t.Fatalf("IN: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM events_lab WHERE id = ANY($1::bigint[])`, ids).Scan(&anyCount); err != nil {
		t.Fatalf("ANY: %v", err)
	}
	if inCount != anyCount {
		t.Errorf("IN=%d, ANY=%d, want equal", inCount, anyCount)
	}
	if anyCount != 1000 {
		t.Errorf("matched %d, want 1000", anyCount)
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// TestSelectStarWiderThanNeeded — SELECT * по drinks отдаёт больше столбцов, чем
// нужно меню.
func TestSelectStarWiderThanNeeded(t *testing.T) {
	pool, ctx := newTestPool(t)
	setup(t, pool, ctx)

	star, err := pool.Query(ctx, `SELECT * FROM drinks WHERE id = 1`)
	if err != nil {
		t.Fatalf("select *: %v", err)
	}
	starCols := len(star.FieldDescriptions())
	star.Close()
	if starCols != 9 {
		t.Errorf("drinks SELECT * columns = %d, want 9", starCols)
	}
}
