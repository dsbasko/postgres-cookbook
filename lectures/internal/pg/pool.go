// Package pg собирает *pgxpool.Pool с дефолтами курса.
//
// Точка входа для лекций: вместо того чтобы в каждом cmd/main.go писать один и
// тот же ParseConfig + дефолтную строку подключения — берём NewPool тут.
// Дефолты подобраны под локальный sandbox-стенд (postgres:18-alpine из
// корневого docker-compose.yml) и переопределяются переменными окружения:
// либо целиком через DATABASE_URL, либо по частям через PG*-переменные.
//
// Форма по аналогии с internal/kafka.NewClient: вариативные Option дописываются
// последними и могут перетирать дефолты — escape-hatch для юнитов, которым
// нужен особый пул (например, MaxConns=1 для урока про lifecycle соединения).
package pg

import (
	"context"
	"fmt"
	"net"
	"net/url"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dsbasko/postgres-cookbook/lectures/internal/config"
)

// Дефолты под песочницу курса (см. docker-compose.yml). Если ни DATABASE_URL,
// ни соответствующая PG*-переменная не заданы, DSN() соберёт из них строку
// postgres://brew:brew@localhost:5432/brew?sslmode=disable.
const (
	DefaultHost     = "localhost"
	DefaultPort     = "5432"
	DefaultUser     = "brew"
	DefaultPassword = "brew"
	DefaultDatabase = "brew"
	DefaultSSLMode  = "disable"
)

// Option настраивает pgxpool.Config перед открытием пула. Передаётся в NewPool
// последним и может перетирать дефолты.
type Option func(*pgxpool.Config)

// WithMaxConns ограничивает размер пула. Удобно для уроков про connection
// lifecycle/pooling (модуль 00), где нужен предсказуемый pg_stat_activity.
func WithMaxConns(n int32) Option {
	return func(c *pgxpool.Config) {
		c.MaxConns = n
	}
}

// DSN возвращает строку подключения к Postgres. Приоритет: целиком заданный
// DATABASE_URL, иначе строка, собранная из PG*-переменных с дефолтами под
// песочницу. Соединение здесь не открывается — только формируется DSN.
func DSN() string {
	if dsn := config.EnvOr("DATABASE_URL", ""); dsn != "" {
		return dsn
	}

	u := &url.URL{
		Scheme: "postgres",
		User: url.UserPassword(
			config.EnvOr("PGUSER", DefaultUser),
			config.EnvOr("PGPASSWORD", DefaultPassword),
		),
		Host: net.JoinHostPort(
			config.EnvOr("PGHOST", DefaultHost),
			config.EnvOr("PGPORT", DefaultPort),
		),
		Path: "/" + config.EnvOr("PGDATABASE", DefaultDatabase),
	}
	q := url.Values{}
	q.Set("sslmode", config.EnvOr("PGSSLMODE", DefaultSSLMode))
	u.RawQuery = q.Encode()
	return u.String()
}

// NewPool открывает пул соединений к Postgres по DSN() с применёнными opts.
//
// Пул ленивый: реального соединения тут ещё нет — оно устанавливается при
// первом запросе. Чтобы убедиться, что БД доступна, вызовите pool.Ping(ctx).
// Caller владеет пулом и обязан вызвать pool.Close().
func NewPool(ctx context.Context, opts ...Option) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(DSN())
	if err != nil {
		return nil, fmt.Errorf("pg.NewPool: parse config: %w", err)
	}
	for _, opt := range opts {
		opt(cfg)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("pg.NewPool: %w", err)
	}
	return pool, nil
}
