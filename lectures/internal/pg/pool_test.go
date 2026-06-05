package pg

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// pgEnvVars — все переменные, влияющие на DSN(). В тестах обнуляем их через
// t.Setenv("", ""): EnvOr трактует пустую строку как «не задано», поэтому это
// эквивалент чистого окружения, но с авто-восстановлением после теста.
var pgEnvVars = []string{
	"DATABASE_URL",
	"PGHOST", "PGPORT", "PGUSER", "PGPASSWORD", "PGDATABASE", "PGSSLMODE",
}

func clearPGEnv(t *testing.T) {
	t.Helper()
	for _, k := range pgEnvVars {
		t.Setenv(k, "")
	}
}

func TestDSN(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "defaults to sandbox when nothing set",
			env:  map[string]string{},
			want: "postgres://brew:brew@localhost:5432/brew?sslmode=disable",
		},
		{
			name: "DATABASE_URL wins over PG* and defaults",
			env: map[string]string{
				"DATABASE_URL": "postgres://u:p@db.example:6543/app?sslmode=require",
				"PGHOST":       "ignored",
				"PGUSER":       "ignored",
			},
			want: "postgres://u:p@db.example:6543/app?sslmode=require",
		},
		{
			name: "built from PG* when DATABASE_URL absent",
			env: map[string]string{
				"PGHOST":     "10.0.0.5",
				"PGPORT":     "6432",
				"PGUSER":     "alice",
				"PGPASSWORD": "secret",
				"PGDATABASE": "shop",
				"PGSSLMODE":  "require",
			},
			want: "postgres://alice:secret@10.0.0.5:6432/shop?sslmode=require",
		},
		{
			name: "partial PG* override keeps sandbox defaults for the rest",
			env: map[string]string{
				"PGHOST":     "db.internal",
				"PGDATABASE": "analytics",
			},
			want: "postgres://brew:brew@db.internal:5432/analytics?sslmode=disable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearPGEnv(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			if got := DSN(); got != tt.want {
				t.Fatalf("DSN() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewPool_BadDSN(t *testing.T) {
	clearPGEnv(t)
	// Невалидное percent-encoding (%zz) — url.Parse отвергает такую строку,
	// поэтому ParseConfig внутри NewPool падает ещё до открытия пула.
	t.Setenv("DATABASE_URL", "postgres://user:%zz@localhost:5432/db")

	pool, err := NewPool(context.Background())
	if err == nil {
		if pool != nil {
			pool.Close()
		}
		t.Fatal("NewPool: expected error for malformed DSN, got nil")
	}
}

func TestWithMaxConns(t *testing.T) {
	clearPGEnv(t)
	cfg, err := pgxpool.ParseConfig(DSN())
	if err != nil {
		t.Fatalf("ParseConfig(default DSN): %v", err)
	}

	WithMaxConns(3)(cfg)
	if cfg.MaxConns != 3 {
		t.Fatalf("WithMaxConns(3): MaxConns = %d, want 3", cfg.MaxConns)
	}
}

// TestNewPool_Sandbox — интеграционный: требует поднятую песочницу
// (`docker compose up -d` из корня). Если БД недоступна, тест пропускается,
// чтобы `go test ./...` оставался зелёным без стенда.
func TestNewPool_Sandbox(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	pool, err := NewPool(ctx)
	if err != nil {
		t.Fatalf("NewPool: unexpected construction error: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Skipf("sandbox not reachable, skipping integration check: %v", err)
	}

	var one int
	if err := pool.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
		t.Fatalf("SELECT 1: %v", err)
	}
	if one != 1 {
		t.Fatalf("SELECT 1 = %d, want 1", one)
	}
}
