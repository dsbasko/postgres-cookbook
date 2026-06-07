// Команда demo юнита 10-03: клиника анти-паттернов приложения.
//
// Пять болезней, которые приложение приносит в здоровую базу, и лечение каждой:
//
//  1. N+1 — цикл из запросов вместо одного батча (считаем round-trip'ы);
//  2. SELECT * — тянем все столбцы, а нужно два (считаем столбцы);
//  3. non-sargable — функция на колонке слепит индекс (читаем план);
//  4. глубокий OFFSET — листалка читает весь префикс; keyset — только страницу
//     (считаем фактически прочитанные строки через EXPLAIN ANALYZE);
//  5. огромный IN — тысяча литералов в тексте vs один параметр = ANY($1::[]).
//
// Два режима:
//
//	demo          — собрать лабораторные данные и прогнать пять пар «болезнь → лечение»;
//	demo -reset   — накатить канон Brew (схема + seed) и выйти (db-reset).
//
// raw-pgx escape-hatch (go.mod, без sqlc): урок про то, КАК приложение ходит в
// базу (число round-trip'ов, форма параметров, план) — это логика Go и EXPLAIN,
// не один query.sql. N+1 показываем на каноне customers/orders, тяжёлые сканы —
// на лабораторных столах (50k строк, generate_series — детерминированно).
// Логи — в stderr, stdout — только результат (для дословной вставки в README).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

const labRows = 50000

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

	// Канон нужен для N+1 (customers/orders) — накатываем seed.
	if err := brew.Reset(ctx, pool); err != nil {
		return fmt.Errorf("brew.Reset: %w", err)
	}
	if err := setupLab(ctx, pool); err != nil {
		return fmt.Errorf("setupLab: %w", err)
	}

	if err := showNPlusOne(ctx, pool); err != nil {
		return fmt.Errorf("N+1: %w", err)
	}
	if err := showSelectStar(ctx, pool); err != nil {
		return fmt.Errorf("select*: %w", err)
	}
	if err := showNonSargable(ctx, pool); err != nil {
		return fmt.Errorf("non-sargable: %w", err)
	}
	if err := showOffsetVsKeyset(ctx, pool); err != nil {
		return fmt.Errorf("offset/keyset: %w", err)
	}
	if err := showHugeIn(ctx, pool); err != nil {
		return fmt.Errorf("huge IN: %w", err)
	}
	return nil
}

// setupLab строит два лабораторных стола по 50k строк: events_lab (id —
// плотный ключ, для OFFSET/keyset и огромного IN) и accounts_lab (email — для
// non-sargable). Данные детерминированы (generate_series, без random).
func setupLab(ctx context.Context, pool *pgxpool.Pool) error {
	const ddl = `
		DROP TABLE IF EXISTS events_lab;
		CREATE TABLE events_lab (
			id    bigint PRIMARY KEY,
			label text   NOT NULL
		);
		INSERT INTO events_lab (id, label)
		SELECT g, 'event-' || g FROM generate_series(1, $1) AS g;

		DROP TABLE IF EXISTS accounts_lab;
		CREATE TABLE accounts_lab (
			id    bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
			email text   NOT NULL
		);
		INSERT INTO accounts_lab (email)
		SELECT 'user' || to_char(g, 'FM000000') || '@brew.example'
		FROM generate_series(1, $1) AS g;
		CREATE INDEX accounts_email_idx ON accounts_lab (email);`
	if _, err := pool.Exec(ctx, strings.ReplaceAll(ddl, "$1", fmt.Sprint(labRows))); err != nil {
		return err
	}
	_, err := pool.Exec(ctx, `ANALYZE events_lab; ANALYZE accounts_lab`)
	if err != nil {
		return err
	}
	// Серийный план — чтобы вывод EXPLAIN был воспроизводим.
	_, err = pool.Exec(ctx, `SET max_parallel_workers_per_gather = 0`)
	return err
}

// showNPlusOne сравнивает «список + по запросу на элемент» с одним батч-запросом.
// Оба возвращают одинаковое число заказов; различается ЧИСЛО round-trip'ов.
func showNPlusOne(ctx context.Context, pool *pgxpool.Pool) error {
	fmt.Println("1) N+1 → батч (round-trip'ы до базы)")

	// Болезнь: сперва берём клиентов, затем заказы КАЖДОГО — по запросу на клиента.
	ids, err := selectCustomerIDs(ctx, pool)
	if err != nil {
		return err
	}
	queries := 1 // запрос на список клиентов
	naiveOrders := 0
	for _, id := range ids {
		var n int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM orders WHERE customer_id = $1`, id).Scan(&n); err != nil {
			return err
		}
		queries++ // ещё один round-trip на каждого клиента
		naiveOrders += n
	}

	// Лечение: один запрос на всех клиентов сразу (= ANY вместо цикла).
	var batchOrders int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM orders WHERE customer_id = ANY($1::text[])`, ids).Scan(&batchOrders); err != nil {
		return err
	}

	fmt.Printf("   N+1:  %d клиентов → %d запроса (1 список + %d на заказы), заказов %d\n",
		len(ids), queries, len(ids), naiveOrders)
	fmt.Printf("   батч: те же данные → 1 запрос (= ANY), заказов %d\n", batchOrders)
	return nil
}

func selectCustomerIDs(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	rows, err := pool.Query(ctx, `SELECT id::text FROM customers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// showSelectStar считает, сколько столбцов реально приехало по SELECT * против
// нужных двух. Лишние столбцы — это и сеть, и хрупкая привязка к схеме.
func showSelectStar(ctx context.Context, pool *pgxpool.Pool) error {
	fmt.Println("\n2) SELECT * → явные столбцы (сколько данных тянем)")

	star, err := pool.Query(ctx, `SELECT * FROM drinks WHERE id = 1`)
	if err != nil {
		return err
	}
	starCols := len(star.FieldDescriptions())
	star.Close()

	need, err := pool.Query(ctx, `SELECT name, base_price FROM drinks WHERE id = 1`)
	if err != nil {
		return err
	}
	needCols := len(need.FieldDescriptions())
	need.Close()

	fmt.Printf("   SELECT *:        вернул %d столбцов\n", starCols)
	fmt.Printf("   SELECT name,...: вернул %d столбца — ровно то, что показывает меню\n", needCols)
	return nil
}

// showNonSargable читает план поиска по email: функция на колонке (lower(email))
// слепит обычный индекс → Seq Scan, expression-индекс по lower(email) его лечит.
func showNonSargable(ctx context.Context, pool *pgxpool.Pool) error {
	fmt.Println("\n3) non-sargable → expression index (план поиска по email)")

	const sargable = `SELECT id FROM accounts_lab WHERE email = 'user000042@brew.example'`
	const wrapped = `SELECT id FROM accounts_lab WHERE lower(email) = 'user000042@brew.example'`

	node, err := topPlanNode(ctx, pool, sargable)
	if err != nil {
		return err
	}
	fmt.Printf("   email = ...        → %s (обычный индекс работает)\n", node)

	node, err = topPlanNode(ctx, pool, wrapped)
	if err != nil {
		return err
	}
	fmt.Printf("   lower(email) = ... → %s (функция слепила индекс)\n", node)

	if _, err := pool.Exec(ctx,
		`CREATE INDEX accounts_lower_email_idx ON accounts_lab (lower(email))`); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `ANALYZE accounts_lab`); err != nil {
		return err
	}
	node, err = topPlanNode(ctx, pool, wrapped)
	if err != nil {
		return err
	}
	fmt.Printf("   lower(email) = ... → %s (после expression-индекса)\n", node)
	return nil
}

// showOffsetVsKeyset меряет, сколько строк РЕАЛЬНО читает сканер: глубокий OFFSET
// прочитывает весь префикс (offset+limit), keyset — только страницу.
func showOffsetVsKeyset(ctx context.Context, pool *pgxpool.Pool) error {
	fmt.Println("\n4) глубокий OFFSET → keyset (сколько строк реально прочитано)")

	const offsetQ = `SELECT id FROM events_lab ORDER BY id LIMIT 10 OFFSET 40000`
	const keysetQ = `SELECT id FROM events_lab WHERE id > 40000 ORDER BY id LIMIT 10`

	offsetRows, err := scanActualRows(ctx, pool, offsetQ)
	if err != nil {
		return err
	}
	keysetRows, err := scanActualRows(ctx, pool, keysetQ)
	if err != nil {
		return err
	}
	fmt.Printf("   OFFSET 40000 LIMIT 10:        сканер прочитал %d строк ради 10\n", offsetRows)
	fmt.Printf("   WHERE id > 40000 LIMIT 10:    сканер прочитал %d строк (та же страница)\n", keysetRows)
	return nil
}

// showHugeIn показывает разницу формы: тысяча литералов в тексте запроса против
// одного параметра-массива = ANY($1). Обе формы дают тот же ответ.
func showHugeIn(ctx context.Context, pool *pgxpool.Pool) error {
	fmt.Println("\n5) огромный IN → = ANY($1::bigint[]) (форма параметров)")

	ids := make([]int64, 1000)
	for i := range ids {
		ids[i] = int64(i + 1)
	}

	// IN-список: тысяча литералов прямо в тексте запроса.
	literals := make([]string, len(ids))
	for i, id := range ids {
		literals[i] = fmt.Sprint(id)
	}
	inSQL := `SELECT count(*) FROM events_lab WHERE id IN (` + strings.Join(literals, ",") + `)`
	var inCount int
	if err := pool.QueryRow(ctx, inSQL).Scan(&inCount); err != nil {
		return err
	}

	// = ANY: один параметр-массив вместо тысячи плейсхолдеров.
	var anyCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM events_lab WHERE id = ANY($1::bigint[])`, ids).Scan(&anyCount); err != nil {
		return err
	}

	fmt.Printf("   IN (1,2,...,1000):     %d литералов в тексте запроса, нашли %d строк\n", len(ids), inCount)
	fmt.Printf("   = ANY($1::bigint[]):   1 параметр-массив на %d id, нашли %d строк\n", len(ids), anyCount)
	return nil
}

// topPlanNode возвращает тип верхнего узла плана через EXPLAIN (FORMAT JSON),
// без ANALYZE (план, а не фактический прогон).
func topPlanNode(ctx context.Context, pool *pgxpool.Pool, query string) (string, error) {
	var plan []map[string]any
	if err := pool.QueryRow(ctx, "EXPLAIN (FORMAT JSON, COSTS OFF) "+query).Scan(&plan); err != nil {
		return "", err
	}
	if len(plan) == 0 {
		return "", fmt.Errorf("пустой план")
	}
	node, _ := plan[0]["Plan"].(map[string]any)
	nodeType, _ := node["Node Type"].(string)
	return nodeType, nil
}

// scanActualRows прогоняет EXPLAIN (ANALYZE) и возвращает фактически прочитанные
// строки самого нижнего (листового) узла плана — то есть сколько строк сканер
// реально достал из таблицы/индекса. TIMING/COSTS/BUFFERS off — нам нужен только
// счётчик строк, он детерминирован.
func scanActualRows(ctx context.Context, pool *pgxpool.Pool, query string) (int, error) {
	var plan []map[string]any
	q := "EXPLAIN (ANALYZE, COSTS OFF, TIMING OFF, SUMMARY OFF, BUFFERS OFF, FORMAT JSON) " + query
	if err := pool.QueryRow(ctx, q).Scan(&plan); err != nil {
		return 0, err
	}
	if len(plan) == 0 {
		return 0, fmt.Errorf("пустой план")
	}
	node, _ := plan[0]["Plan"].(map[string]any)
	return leafActualRows(node), nil
}

// leafActualRows спускается до листа плана (узел без вложенных Plans) и отдаёт
// его "Actual Rows".
func leafActualRows(node map[string]any) int {
	if children, ok := node["Plans"].([]any); ok && len(children) > 0 {
		if child, ok := children[0].(map[string]any); ok {
			return leafActualRows(child)
		}
	}
	switch v := node["Actual Rows"].(type) {
	case float64:
		return int(v)
	default:
		return 0
	}
}
