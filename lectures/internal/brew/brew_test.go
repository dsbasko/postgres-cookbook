package brew

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
)

// canonTables — все таблицы, которые brew.sql обязан создать.
var canonTables = []string{
	"orders", "outbox", "processed_outbox_ids",
	"drinks", "articles", "customers",
	"shops", "order_items", "inventory",
}

// collapseWS схлопывает любые пробельные последовательности (включая переводы
// строк) в один пробел — чтобы проверять DDL по токенам, не завися от верстки.
func collapseWS(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func readSchemaFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(SchemaDir(), name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

// TestSchemaFilesExist — schema/brew.sql и schema/seed.sql резолвятся и непусты.
// DB не нужна.
func TestSchemaFilesExist(t *testing.T) {
	for _, name := range []string{SchemaFile, SeedFile} {
		info, err := os.Stat(filepath.Join(SchemaDir(), name))
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if info.Size() == 0 {
			t.Fatalf("%s is empty", name)
		}
	}
}

// TestSchemaDir_EnvOverride — BREW_SCHEMA_DIR перетирает дефолтный путь.
func TestSchemaDir_EnvOverride(t *testing.T) {
	t.Setenv(SchemaDirEnv, "/tmp/custom-schema")
	if got := SchemaDir(); got != "/tmp/custom-schema" {
		t.Fatalf("SchemaDir() = %q, want /tmp/custom-schema", got)
	}
}

// TestBrewSchema_ByteCompatCanon — гард байт-совместимости: каноничные таблицы и
// колонки должны присутствовать в brew.sql ДОСЛОВНО. Если кто-то переименует
// колонку канона — этот тест упадёт раньше, чем сломается handoff 10-05. DB не
// нужна.
func TestBrewSchema_ByteCompatCanon(t *testing.T) {
	schema := collapseWS(readSchemaFile(t, SchemaFile))

	required := []struct {
		name  string
		token string
	}{
		{"orders table", "CREATE TABLE IF NOT EXISTS orders"},
		{"orders.customer_id is TEXT", "customer_id TEXT NOT NULL"},
		{"orders.amount is NUMERIC", "amount NUMERIC NOT NULL"},
		{"orders.status default", "status TEXT NOT NULL DEFAULT 'created'"},
		{"outbox table", "CREATE TABLE IF NOT EXISTS outbox"},
		{"outbox.payload is JSONB", "payload JSONB NOT NULL"},
		{"outbox.published_at nullable", "published_at TIMESTAMPTZ NULL"},
		{"outbox partial index", "CREATE INDEX IF NOT EXISTS outbox_unpublished_idx ON outbox (id) WHERE published_at IS NULL"},
		{"processed_outbox_ids table", "CREATE TABLE IF NOT EXISTS processed_outbox_ids"},
		{"processed_outbox_ids pk", "outbox_id BIGINT PRIMARY KEY"},
		{"drinks table", "CREATE TABLE IF NOT EXISTS drinks"},
		{"drinks.base_price is BIGINT", "base_price BIGINT NOT NULL"},
		{"drinks replica identity full", "ALTER TABLE drinks REPLICA IDENTITY FULL"},
		{"articles table", "CREATE TABLE IF NOT EXISTS articles"},
		{"articles replica identity full", "ALTER TABLE articles REPLICA IDENTITY FULL"},
		{"customers table", "CREATE TABLE IF NOT EXISTS customers"},
		{"customers.id is BIGINT PK", "id BIGINT PRIMARY KEY"},
		{"customers replica identity full", "ALTER TABLE customers REPLICA IDENTITY FULL"},
		{"shops table (rich)", "CREATE TABLE IF NOT EXISTS shops"},
		{"order_items table (rich)", "CREATE TABLE IF NOT EXISTS order_items"},
		{"inventory table (rich)", "CREATE TABLE IF NOT EXISTS inventory"},
	}

	for _, r := range required {
		t.Run(r.name, func(t *testing.T) {
			if !strings.Contains(schema, r.token) {
				t.Errorf("brew.sql missing canonical token %q", r.token)
			}
		})
	}
}

// newTestPool строит пул к песочнице и пропускает тест, если БД недоступна —
// чтобы `go test ./...` оставался зелёным без поднятого docker compose.
func newTestPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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

func tableExists(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string) bool {
	t.Helper()
	var reg *string
	if err := pool.QueryRow(ctx, "SELECT to_regclass($1)::text", name).Scan(&reg); err != nil {
		t.Fatalf("to_regclass(%s): %v", name, err)
	}
	return reg != nil
}

func countRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// TestReset_AppliesCanonAndIsIdempotent — интеграционный: Reset создаёт все
// таблицы канона, наполняет seed-данными и безопасно прогоняется дважды
// (повторный прогон не падает и не меняет счётчики строк).
func TestReset_AppliesCanonAndIsIdempotent(t *testing.T) {
	pool, ctx := newTestPool(t)

	// Ожидаемые количества строк после seed (стабильны между прогонами).
	wantCounts := map[string]int{
		"customers":   3,
		"drinks":      5,
		"articles":    2,
		"shops":       2,
		"orders":      3,
		"order_items": 4,
		"inventory":   5,
	}

	for _, pass := range []string{"first reset", "second reset"} {
		t.Run(pass, func(t *testing.T) {
			if err := Reset(ctx, pool); err != nil {
				t.Fatalf("Reset: %v", err)
			}

			for _, tbl := range canonTables {
				if !tableExists(t, ctx, pool, tbl) {
					t.Errorf("table %q does not exist after Reset", tbl)
				}
			}

			for tbl, want := range wantCounts {
				if got := countRows(t, ctx, pool, tbl); got != want {
					t.Errorf("count(%s) = %d, want %d", tbl, got, want)
				}
			}
		})
	}
}

// TestApply_ExtraDDL — интеграционный: per-unit добавка применяется поверх
// канона, и Apply остаётся идемпотентным при повторном прогоне.
func TestApply_ExtraDDL(t *testing.T) {
	pool, ctx := newTestPool(t)

	const extra = `CREATE TABLE IF NOT EXISTS brew_unit_demo (
		id    BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
		label TEXT   NOT NULL
	);`
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DROP TABLE IF EXISTS brew_unit_demo")
	})

	for _, pass := range []string{"first apply", "second apply"} {
		t.Run(pass, func(t *testing.T) {
			if err := Apply(ctx, pool, extra); err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if !tableExists(t, ctx, pool, "brew_unit_demo") {
				t.Error("extra DDL table brew_unit_demo not created")
			}
			// Канон по-прежнему на месте и наполнен.
			if got := countRows(t, ctx, pool, "drinks"); got != 5 {
				t.Errorf("count(drinks) = %d, want 5", got)
			}
		})
	}
}
