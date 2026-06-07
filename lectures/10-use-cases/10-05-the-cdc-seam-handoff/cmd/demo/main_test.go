package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
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

func collapseWS(s string) string { return strings.Join(strings.Fields(s), " ") }

// TestInitSQL_ByteCompatTokens — DB не нужна: db/init.sql содержит каноничный DDL
// CDC-источников и публикацию ДОСЛОВНО. Падает при любом переименовании колонки
// канона раньше, чем сломается эстафета в kafka-cookbook.
func TestInitSQL_ByteCompatTokens(t *testing.T) {
	data, err := os.ReadFile(initSQLPath())
	if err != nil {
		t.Fatalf("read init.sql: %v", err)
	}
	sql := collapseWS(string(data))

	tokens := []string{
		"CREATE TABLE IF NOT EXISTS drinks",
		"base_price BIGINT NOT NULL",
		"ALTER TABLE drinks REPLICA IDENTITY FULL",
		"CREATE TABLE IF NOT EXISTS articles",
		"ALTER TABLE articles REPLICA IDENTITY FULL",
		"CREATE TABLE IF NOT EXISTS customers",
		"id BIGINT PRIMARY KEY",
		"ALTER TABLE customers REPLICA IDENTITY FULL",
		"CREATE PUBLICATION dbz_publication FOR TABLE drinks, articles, customers",
	}
	for _, tok := range tokens {
		if !strings.Contains(sql, tok) {
			t.Errorf("init.sql missing canonical token %q", tok)
		}
	}
}

// applySeam готовит шов: канон + чистая публикация + init.sql.
func applySeam(t *testing.T, pool *pgxpool.Pool, ctx context.Context) {
	t.Helper()
	if err := brew.Reset(ctx, pool); err != nil {
		t.Fatalf("brew.Reset: %v", err)
	}
	if _, err := pool.Exec(ctx, `DROP PUBLICATION IF EXISTS `+publication); err != nil {
		t.Fatalf("drop publication: %v", err)
	}
	if err := applyInitSQL(ctx, pool); err != nil {
		t.Fatalf("applyInitSQL: %v", err)
	}
}

// TestPublicationCoversCanonTables — публикация покрывает РОВНО drinks, articles,
// customers.
func TestPublicationCoversCanonTables(t *testing.T) {
	pool, ctx := newTestPool(t)
	applySeam(t, pool, ctx)

	var tables string
	if err := pool.QueryRow(ctx,
		`SELECT string_agg(tablename, ',' ORDER BY tablename)
		   FROM pg_publication_tables WHERE pubname = $1`, publication).Scan(&tables); err != nil {
		t.Fatalf("publication tables: %v", err)
	}
	if want := strings.Join(cdcTables, ","); tables != want {
		t.Errorf("publication tables = %q, want %q", tables, want)
	}
}

// TestReplicaIdentityFull — три CDC-источника несут REPLICA IDENTITY FULL.
func TestReplicaIdentityFull(t *testing.T) {
	pool, ctx := newTestPool(t)
	applySeam(t, pool, ctx)

	for _, tbl := range cdcTables {
		var ri rune
		if err := pool.QueryRow(ctx,
			`SELECT relreplident FROM pg_class WHERE relname = $1`, tbl).Scan(&ri); err != nil {
			t.Fatalf("relreplident(%s): %v", tbl, err)
		}
		if ri != 'f' {
			t.Errorf("%s relreplident = %q, want 'f' (full)", tbl, string(ri))
		}
	}
}

// TestBeforeImageHasAllColumns — под REPLICA IDENTITY FULL before-image UPDATE
// содержит все 9 столбцов drinks (а не только PK).
func TestBeforeImageHasAllColumns(t *testing.T) {
	pool, ctx := newTestPool(t)
	applySeam(t, pool, ctx)

	if err := proveBeforeImage(ctx, pool); err != nil {
		t.Fatalf("proveBeforeImage: %v", err)
	}

	// Повторяем напрямую, чтобы проверить число столбцов в before-image.
	if _, err := pool.Exec(ctx,
		`SELECT pg_drop_replication_slot(slot_name) FROM pg_replication_slots WHERE slot_name = $1`, slotName); err != nil {
		t.Fatalf("drop slot: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`SELECT pg_create_logical_replication_slot($1, 'test_decoding')`, slotName); err != nil {
		t.Fatalf("create slot: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`SELECT pg_drop_replication_slot(slot_name) FROM pg_replication_slots WHERE slot_name = $1`, slotName)
	})
	if _, err := pool.Exec(ctx, `UPDATE drinks SET base_price = base_price + 1 WHERE id = 1`); err != nil {
		t.Fatalf("update: %v", err)
	}
	changes, err := slotChanges(ctx, pool)
	if err != nil {
		t.Fatalf("slotChanges: %v", err)
	}
	var cols int
	for _, c := range changes {
		if strings.Contains(c, ": UPDATE:") {
			cols = beforeImageColumns(c)
			break
		}
	}
	if cols != 9 {
		t.Errorf("before-image columns = %d, want 9 (REPLICA IDENTITY FULL)", cols)
	}
}
