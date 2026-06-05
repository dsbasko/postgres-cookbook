// Package brew накатывает канон схемы Brew на базу песочницы курса.
//
// Канон — это шесть байт-совместимых с kafka-cookbook таблиц (orders, outbox,
// processed_outbox_ids, drinks, articles, customers) плюс наши таблицы для
// богатых примеров (shops, order_items, inventory). Байт-совместимость нужна
// для capstone-handoff 10-05: init.sql postgres-курса слово-в-слово совпадает
// с init.sql kafka-курса, поэтому CDC-эстафета (Debezium читает outbox/drinks/
// ...) работает без переписывания схемы.
//
// Reset/Apply исполняют schema/brew.sql и schema/seed.sql из корня репозитория.
// Оба идемпотентны: DDL — через IF NOT EXISTS, seed — через TRUNCATE ... RESTART
// IDENTITY перед вставкой, поэтому `make db-reset` можно гонять сколько угодно
// раз — состояние БД будет одинаковым (стабильные id → дословно воспроизводимый
// вывод демо в README).
package brew

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// SchemaFile — DDL канона Brew (таблицы, индексы, REPLICA IDENTITY).
	SchemaFile = "brew.sql"
	// SeedFile — детерминированные демо-данные Brew.
	SeedFile = "seed.sql"
	// SchemaDirEnv переопределяет каталог со schema-файлами — escape-hatch для
	// тестов и нестандартных запусков. По умолчанию — schema/ в корне репо.
	SchemaDirEnv = "BREW_SCHEMA_DIR"
)

// SchemaDir возвращает каталог с brew.sql/seed.sql: значение BREW_SCHEMA_DIR,
// иначе schema/ в корне репозитория. Корень резолвится относительно этого
// исходника (курс всегда запускается из исходников: go run / go test), так что
// функция не зависит от текущего рабочего каталога вызывающего.
func SchemaDir() string {
	if dir := os.Getenv(SchemaDirEnv); dir != "" {
		return dir
	}
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "schema"
	}
	// thisFile = <repo>/lectures/internal/brew/brew.go → корень на три уровня выше.
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "schema")
}

// Reset накатывает канон Brew (schema/brew.sql + schema/seed.sql) на пул.
// Идемпотентно: повторный вызов возвращает БД к тому же эталонному состоянию.
func Reset(ctx context.Context, pool *pgxpool.Pool) error {
	return Apply(ctx, pool)
}

// Apply накатывает baseline-схему, затем per-unit DDL-добавки, затем seed —
// порядок baseline → добавки → seed. extraDDL обычно содержит schema.sql юнита
// (DDL поверх канона) и должен быть идемпотентным (CREATE TABLE IF NOT EXISTS).
func Apply(ctx context.Context, pool *pgxpool.Pool, extraDDL ...string) error {
	dir := SchemaDir()

	schemaSQL, err := os.ReadFile(filepath.Join(dir, SchemaFile))
	if err != nil {
		return fmt.Errorf("brew.Apply: read %s: %w", SchemaFile, err)
	}
	if _, err := pool.Exec(ctx, string(schemaSQL)); err != nil {
		return fmt.Errorf("brew.Apply: schema: %w", err)
	}

	for i, ddl := range extraDDL {
		if _, err := pool.Exec(ctx, ddl); err != nil {
			return fmt.Errorf("brew.Apply: extra DDL #%d: %w", i, err)
		}
	}

	seedSQL, err := os.ReadFile(filepath.Join(dir, SeedFile))
	if err != nil {
		return fmt.Errorf("brew.Apply: read %s: %w", SeedFile, err)
	}
	if _, err := pool.Exec(ctx, string(seedSQL)); err != nil {
		return fmt.Errorf("brew.Apply: seed: %w", err)
	}
	return nil
}
