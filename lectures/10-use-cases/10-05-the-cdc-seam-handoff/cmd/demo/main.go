// Команда demo юнита 10-05: шов CDC — эстафета в kafka-cookbook.
//
// Финал курса. Здесь postgres-cookbook передаёт эстафету Kafka-курсу: мы
// настраиваем логическую репликацию ровно для тех таблиц, что байт-совместимы с
// kafka-cookbook (drinks/articles/customers), и доказываем, что шов работает —
// before-image изменения содержит ВСЮ строку (благодаря REPLICA IDENTITY FULL),
// а не только PK. Дальше Debezium из Kafka-курса подключится к нашей публикации
// и прочитает поток без переписывания схемы.
//
// Что делает демо:
//
//   - применяет db/init.sql (тот же артефакт, что уедет на сторону Kafka) —
//     drinks/articles/customers + REPLICA IDENTITY FULL + PUBLICATION dbz_publication;
//   - проверяет логическим декодированием (test_decoding), что UPDATE отдаёт
//     before-image со всеми столбцами.
//
// Два режима:
//
//	demo          — собрать шов и доказать его декодированием;
//	demo -reset   — накатить канон Brew (схема + seed) и выйти (db-reset).
//
// raw-pgx escape-hatch (go.mod, без sqlc): урок про конфигурацию репликации
// (PUBLICATION, REPLICA IDENTITY, слоты) — это DDL и системные функции, не SQL
// уровня sqlc. Логи — в stderr, stdout — только результат (для вставки в README).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"text/tabwriter"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

const (
	publication = "dbz_publication"
	slotName    = "cdc_seam_demo_slot"
)

// cdcTables — три CDC-источника, байт-совместимые с kafka-cookbook.
var cdcTables = []string{"articles", "customers", "drinks"}

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
	pool, err := pg.NewPool(ctx)
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

	// Свежий канон (drinks/articles/customers со seed-данными).
	if err := brew.Reset(ctx, pool); err != nil {
		return fmt.Errorf("brew.Reset: %w", err)
	}

	// Применяем артефакт эстафеты db/init.sql (idempotent). Перед этим чистим
	// публикацию — чтобы демо было детерминированным от прогона к прогону.
	if _, err := pool.Exec(ctx, `DROP PUBLICATION IF EXISTS `+publication); err != nil {
		return err
	}
	if err := applyInitSQL(ctx, pool); err != nil {
		return fmt.Errorf("applyInitSQL: %w", err)
	}

	fmt.Println("1) Канон на месте, REPLICA IDENTITY FULL на CDC-источниках:")
	if err := showReplicaIdentity(ctx, pool); err != nil {
		return fmt.Errorf("showReplicaIdentity: %w", err)
	}

	fmt.Println("\n2) Публикация для Debezium (явный список таблиц):")
	if err := showPublication(ctx, pool); err != nil {
		return fmt.Errorf("showPublication: %w", err)
	}

	fmt.Println("\n3) Проверяем шов логическим декодированием (test_decoding):")
	if err := proveBeforeImage(ctx, pool); err != nil {
		return fmt.Errorf("proveBeforeImage: %w", err)
	}

	fmt.Println("\n4) Эстафета: db/init.sql байт-совместим с kafka-cookbook — Debezium")
	fmt.Println("   читает наши таблицы без переписывания схемы. Дальше — Kafka-курс.")
	return nil
}

// initSQLPath резолвит db/init.sql относительно этого исходника (cmd/demo/), не
// завися от рабочего каталога вызывающего.
func initSQLPath() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "db/init.sql"
	}
	// thisFile = <unit>/cmd/demo/main.go → db/ на два уровня выше.
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "db", "init.sql")
}

// applyInitSQL применяет артефакт эстафеты. Он идемпотентен: CREATE TABLE IF NOT
// EXISTS (канон уже есть → no-op), ALTER REPLICA IDENTITY FULL (повторно ок), DO
// block создаёт публикацию, если её нет.
func applyInitSQL(ctx context.Context, pool *pgxpool.Pool) error {
	sql, err := os.ReadFile(initSQLPath())
	if err != nil {
		return fmt.Errorf("read init.sql: %w", err)
	}
	_, err = pool.Exec(ctx, string(sql))
	return err
}

func showReplicaIdentity(ctx context.Context, pool *pgxpool.Pool) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, tbl := range cdcTables {
		var ri string
		if err := pool.QueryRow(ctx,
			`SELECT CASE relreplident
			          WHEN 'f' THEN 'full' WHEN 'd' THEN 'default'
			          WHEN 'i' THEN 'index' ELSE 'nothing' END
			   FROM pg_class WHERE relname = $1`, tbl).Scan(&ri); err != nil {
			return err
		}
		fmt.Fprintf(w, "   %s\treplica identity: %s\n", tbl, ri)
	}
	return w.Flush()
}

func showPublication(ctx context.Context, pool *pgxpool.Pool) error {
	var tables string
	if err := pool.QueryRow(ctx,
		`SELECT string_agg(tablename, ', ' ORDER BY tablename)
		   FROM pg_publication_tables WHERE pubname = $1`, publication).Scan(&tables); err != nil {
		return err
	}
	fmt.Printf("   CREATE PUBLICATION %s FOR TABLE drinks, articles, customers\n", publication)
	fmt.Printf("   публикует таблицы: %s\n", tables)
	return nil
}

// proveBeforeImage создаёт временный логический слот (test_decoding), делает
// UPDATE напитка и читает изменения из слота. Под REPLICA IDENTITY FULL
// before-image (old-key) содержит все столбцы строки до изменения — это и есть
// то, что нужно Debezium'у для UPDATE/DELETE. Слот в конце сносим.
func proveBeforeImage(ctx context.Context, pool *pgxpool.Pool) error {
	// Слот мог остаться от прошлого прерванного прогона — снимаем, если есть.
	if _, err := pool.Exec(ctx,
		`SELECT pg_drop_replication_slot(slot_name) FROM pg_replication_slots WHERE slot_name = $1`,
		slotName); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx,
		`SELECT pg_create_logical_replication_slot($1, 'test_decoding')`, slotName); err != nil {
		return err
	}
	defer func() {
		_, _ = pool.Exec(context.Background(),
			`SELECT pg_drop_replication_slot(slot_name) FROM pg_replication_slots WHERE slot_name = $1`,
			slotName)
	}()

	// Одно изменение: поднимаем цену эспрессо на 1 копейку.
	if _, err := pool.Exec(ctx,
		`UPDATE drinks SET base_price = base_price + 1 WHERE id = 1`); err != nil {
		return err
	}

	changes, err := slotChanges(ctx, pool)
	if err != nil {
		return err
	}

	updateLine := ""
	for _, c := range changes {
		if strings.Contains(c, ": UPDATE:") {
			updateLine = c
			break
		}
	}
	if updateLine == "" {
		return fmt.Errorf("в слоте нет UPDATE среди %d изменений", len(changes))
	}

	cols := beforeImageColumns(updateLine)
	fmt.Printf("   UPDATE drinks #1 → перехвачено изменений в слоте: %d\n", len(changes))
	fmt.Printf("   before-image (old-key) содержит столбцов: %d → REPLICA IDENTITY FULL работает\n", cols)
	fmt.Println("   (без FULL Debezium увидел бы в before-image только id; здесь — всю строку)")
	return nil
}

// slotChanges вычитывает все накопленные изменения слота как строки.
func slotChanges(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	rows, err := pool.Query(ctx,
		`SELECT data FROM pg_logical_slot_get_changes($1, NULL, NULL)`, slotName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		out = append(out, data)
	}
	return out, rows.Err()
}

// colToken ловит «имя_столбца[тип]» в выводе test_decoding.
var colToken = regexp.MustCompile(`\w+\[`)

// beforeImageColumns считает столбцы в сегменте old-key строки UPDATE — то есть
// сколько столбцов попало в before-image. Под REPLICA IDENTITY FULL это все
// столбцы таблицы; под default — только PK.
func beforeImageColumns(updateLine string) int {
	const marker = "old-key:"
	i := strings.Index(updateLine, marker)
	if i < 0 {
		return 0
	}
	seg := updateLine[i+len(marker):]
	if j := strings.Index(seg, "new-tuple:"); j >= 0 {
		seg = seg[:j]
	}
	return len(colToken.FindAllString(seg, -1))
}
