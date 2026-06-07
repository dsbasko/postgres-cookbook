// Команда demo юнита 10-01: капстон «собери схему Brew».
//
// Это финал-сборка курса: один маленький подсервис (программа лояльности Brew)
// проходит через всё, что мы учили по отдельности — типы, ограничения, CRUD с
// RETURNING, индекс, проверенный планом, и транзакцию с ретраем на 40001.
//
// Два режима:
//
//	demo          — собрать схему лояльности, наполнить её, показать, как
//	                ограничения отбивают мусор, как индекс меняет план, и как
//	                ретрай вытягивает транзакцию из serialization_failure;
//	demo -reset   — накатить канон Brew (схема + seed) и выйти (db-reset).
//
// raw-pgx escape-hatch (go.mod, без sqlc): капстон строит свою схему DDL'ом,
// читает планы через EXPLAIN и крутит ретрай-петлю — sqlc тут не протагонист.
// Конфликт 40001 создаётся ДЕТЕРМИНИРОВАННО (как в 05-05): на первой попытке
// синхронно вклинивается «ночное начисление процентов» отдельной транзакцией.
// Лабораторные столы (cap_*) пересоздаются в начале, канон не трогаем.
// Логи — в stderr, stdout — только результат (для дословной вставки в README).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

// serializationFailure — SQLSTATE 40001 (см. 05-04/05-05): под SERIALIZABLE
// база завершает этим кодом транзакцию, которую нельзя сериализовать. Контракт:
// лови этот код и повтори транзакцию на свежем снимке.
const serializationFailure = "40001"

const maxAttempts = 5

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

	if err := buildSchema(ctx, pool); err != nil {
		return fmt.Errorf("buildSchema: %w", err)
	}
	fmt.Println("1) Схема собрана: cap_members (типы + PK + UNIQUE + CHECK), cap_ledger (FK на члена).")

	if err := seedMembers(ctx, pool); err != nil {
		return fmt.Errorf("seedMembers: %w", err)
	}

	fmt.Println("\n2) CRUD с RETURNING: завели трёх членов клуба, id вернул сам INSERT:")
	if err := dumpMembers(ctx, pool); err != nil {
		return err
	}

	fmt.Println("\n3) Ограничения отбивают мусор (печатаем SQLSTATE, а не текст ошибки):")
	if err := showConstraints(ctx, pool); err != nil {
		return fmt.Errorf("showConstraints: %w", err)
	}

	fmt.Println("\n4) Индекс, проверенный планом (EXPLAIN до и после CREATE INDEX):")
	if err := showIndexPlan(ctx, pool); err != nil {
		return fmt.Errorf("showIndexPlan: %w", err)
	}

	fmt.Println("\n5) Транзакция с ретраем на 40001 (начисление бонуса под SERIALIZABLE):")
	if err := showRetry(ctx, pool); err != nil {
		return fmt.Errorf("showRetry: %w", err)
	}

	return nil
}

// buildSchema собирает схему лояльности DDL'ом. cap_members демонстрирует выбор
// типов (деньги — BIGINT-центы) и ограничения (PK, UNIQUE, CHECK, NOT NULL),
// cap_ledger — внешний ключ на члена клуба. Индекс по ledger.member_id НЕ
// создаём здесь нарочно: его роль покажет EXPLAIN в шаге 4. DROP ... CASCADE +
// CREATE делают сборку идемпотентной.
func buildSchema(ctx context.Context, pool *pgxpool.Pool) error {
	const ddl = `
		DROP TABLE IF EXISTS cap_ledger;
		DROP TABLE IF EXISTS cap_members;

		CREATE TABLE cap_members (
			id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
			email         text   NOT NULL UNIQUE,
			tier          text   NOT NULL DEFAULT 'bronze'
			                     CHECK (tier IN ('bronze', 'silver', 'gold')),
			balance_cents bigint NOT NULL DEFAULT 0 CHECK (balance_cents >= 0)
		);

		CREATE TABLE cap_ledger (
			id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
			member_id   bigint      NOT NULL REFERENCES cap_members (id) ON DELETE CASCADE,
			delta_cents bigint      NOT NULL,
			reason      text        NOT NULL,
			created_at  timestamptz NOT NULL DEFAULT now()
		);`
	_, err := pool.Exec(ctx, ddl)
	return err
}

// seedMembers заводит трёх членов клуба через INSERT ... RETURNING. Балансы и
// уровни фиксированы → детерминированный вывод.
func seedMembers(ctx context.Context, pool *pgxpool.Pool) error {
	members := []struct {
		email   string
		tier    string
		balance int64
	}{
		{"alice@brew.example", "gold", 1500},
		{"bob@brew.example", "silver", 300},
		{"carol@brew.example", "bronze", 0},
	}
	for _, m := range members {
		var id int64
		var tier string
		err := pool.QueryRow(ctx,
			`INSERT INTO cap_members (email, tier, balance_cents)
			 VALUES ($1, $2, $3) RETURNING id, tier`,
			m.email, m.tier, m.balance).Scan(&id, &tier)
		if err != nil {
			return err
		}
	}
	return nil
}

func dumpMembers(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx,
		`SELECT id, email, tier, balance_cents FROM cap_members ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "   id\temail\tуровень\tбаланс, ₽")
	for rows.Next() {
		var id, balance int64
		var email, tier string
		if err := rows.Scan(&id, &email, &tier, &balance); err != nil {
			return err
		}
		fmt.Fprintf(w, "   %d\t%s\t%s\t%d.%02d\n", id, email, tier, balance/100, balance%100)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return w.Flush()
}

// showConstraints пытается записать заведомо плохие данные и печатает SQLSTATE
// каждого отказа — ограничения превращают «правило бизнеса» в гарантию БД.
func showConstraints(ctx context.Context, pool *pgxpool.Pool) error {
	cases := []struct {
		label string
		sql   string
		args  []any
	}{
		{
			"дубль email (UNIQUE)",
			`INSERT INTO cap_members (email, tier) VALUES ($1, 'bronze')`,
			[]any{"alice@brew.example"},
		},
		{
			"уровень вне набора (CHECK tier)",
			`INSERT INTO cap_members (email, tier) VALUES ($1, 'platinum')`,
			[]any{"dave@brew.example"},
		},
		{
			"отрицательный баланс (CHECK balance)",
			`INSERT INTO cap_members (email, balance_cents) VALUES ($1, -100)`,
			[]any{"erin@brew.example"},
		},
		{
			"запись на несуществующего члена (FK)",
			`INSERT INTO cap_ledger (member_id, delta_cents, reason) VALUES (999, 100, 'bonus')`,
			nil,
		},
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "   попытка\tрезультат")
	for _, c := range cases {
		_, err := pool.Exec(ctx, c.sql, c.args...)
		fmt.Fprintf(w, "   %s\t%s\n", c.label, outcome(err))
	}
	return w.Flush()
}

// outcome переводит ошибку в короткую метку: «OK» либо «SQLSTATE NNNNN».
func outcome(err error) string {
	if err == nil {
		return "OK (записалось)"
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return "отбито, SQLSTATE " + pgErr.Code
	}
	return "ошибка: " + err.Error()
}

// showIndexPlan наполняет cap_ledger так, что один член клуба — «кит» (много
// строк), а у остальных строк мало, затем сравнивает план точечной выборки до и
// после CREATE INDEX. Параллельные воркеры и стоимостные строки выключены, узел
// плана берём из EXPLAIN (FORMAT JSON) → вывод воспроизводим.
func showIndexPlan(ctx context.Context, pool *pgxpool.Pool) error {
	// member 1 (Алиса) — кит: 20000 записей. У member 2 (Борис) — 3 записи.
	const fill = `
		INSERT INTO cap_ledger (member_id, delta_cents, reason)
		SELECT 1, 1, 'whale' FROM generate_series(1, 20000);
		INSERT INTO cap_ledger (member_id, delta_cents, reason)
		SELECT 2, 1, 'bob' FROM generate_series(1, 3);`
	if _, err := pool.Exec(ctx, fill); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `SET max_parallel_workers_per_gather = 0`); err != nil {
		return err
	}

	const lookup = `SELECT delta_cents FROM cap_ledger WHERE member_id = 2`

	before, err := topPlanNode(ctx, pool, lookup)
	if err != nil {
		return err
	}
	fmt.Printf("   до индекса:    выборка по member_id=2 → %s\n", before)

	if _, err := pool.Exec(ctx, `CREATE INDEX cap_ledger_member_idx ON cap_ledger (member_id)`); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `ANALYZE cap_ledger`); err != nil {
		return err
	}

	after, err := topPlanNode(ctx, pool, lookup)
	if err != nil {
		return err
	}
	fmt.Printf("   после индекса: та же выборка        → %s\n", after)
	return nil
}

// topPlanNode возвращает тип верхнего узла плана запроса (например, "Seq Scan"
// или "Index Only Scan") через EXPLAIN (FORMAT JSON). Без ANALYZE — план, не
// фактический прогон, поэтому функция не зависит от времени выполнения.
func topPlanNode(ctx context.Context, pool *pgxpool.Pool, query string) (string, error) {
	var plan []map[string]any
	row := pool.QueryRow(ctx, "EXPLAIN (FORMAT JSON, COSTS OFF) "+query)
	if err := row.Scan(&plan); err != nil {
		return "", err
	}
	if len(plan) == 0 {
		return "", fmt.Errorf("пустой план")
	}
	node, ok := plan[0]["Plan"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("нет узла Plan")
	}
	nodeType, _ := node["Node Type"].(string)
	return nodeType, nil
}

// showRetry начисляет Алисе бонус под SERIALIZABLE и переживает 40001. Конфликт
// создаётся детерминированно: на первой попытке, после того как мы прочитали
// баланс, синхронно вклинивается «ночное начисление процентов» (отдельная
// транзакция, коммитит первой) — ровно как другой инстанс приложения сделал бы
// в проде. withRetry повторяет на свежем снимке, и баланс сходится.
func showRetry(ctx context.Context, pool *pgxpool.Pool) error {
	const bonus = int64(500)

	var startBalance int64
	if err := pool.QueryRow(ctx,
		`SELECT balance_cents FROM cap_members WHERE id = 1`).Scan(&startBalance); err != nil {
		return err
	}
	fmt.Printf("   старт: баланс Алисы %d.%02d ₽\n", startBalance/100, startBalance%100)

	injected := false
	attempts, err := withRetry(ctx, pool, func(ctx context.Context, tx pgx.Tx, attempt int) error {
		var balance int64
		if err := tx.QueryRow(ctx,
			`SELECT balance_cents FROM cap_members WHERE id = 1`).Scan(&balance); err != nil {
			return err
		}

		if !injected {
			injected = true
			if err := nightlyInterest(ctx, pool); err != nil {
				return fmt.Errorf("инъекция конфликта: %w", err)
			}
			fmt.Println("   (параллельно: ночное начисление +1.00 ₽ закоммитило первым — конфликт назревает)")
		}

		fmt.Printf("   попытка %d: прочитал %d.%02d ₽, пишу +%d.%02d ₽\n",
			attempt, balance/100, balance%100, bonus/100, bonus%100)
		_, err := tx.Exec(ctx,
			`UPDATE cap_members SET balance_cents = $1 WHERE id = 1`, balance+bonus)
		return err
	})
	if err != nil {
		return fmt.Errorf("начисление бонуса не удалось: %w", err)
	}

	var endBalance int64
	if err := pool.QueryRow(ctx,
		`SELECT balance_cents FROM cap_members WHERE id = 1`).Scan(&endBalance); err != nil {
		return err
	}
	fmt.Printf("   ✓ COMMIT успешен за %d попытки; итог %d.%02d ₽ (старт +1.00 от процентов +5.00 бонус)\n",
		attempts, endBalance/100, endBalance%100)
	return nil
}

// withRetry прогоняет txFn в транзакции SERIALIZABLE и повторяет её на 40001
// (см. 05-05). Возвращает номер удачной попытки. На каждой попытке — свежая
// транзакция и свежий снимок: повтор видит закоммиченные изменения конкурента.
func withRetry(ctx context.Context, pool *pgxpool.Pool, txFn func(context.Context, pgx.Tx, int) error) (int, error) {
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			return attempt, fmt.Errorf("BeginTx: %w", err)
		}

		err = txFn(ctx, tx, attempt)
		if err == nil {
			err = tx.Commit(ctx)
		}
		if err == nil {
			return attempt, nil
		}

		_ = tx.Rollback(ctx)

		if isSerializationFailure(err) {
			fmt.Printf("   ↻ упало: %s (serialization_failure) — повторяю на свежем снимке\n", serializationFailure)
			continue
		}
		return attempt, err
	}
	return maxAttempts, fmt.Errorf("исчерпаны %d попыток ретрая", maxAttempts)
}

func isSerializationFailure(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == serializationFailure
}

// nightlyInterest — «другой инстанс приложения»: в отдельной транзакции
// SERIALIZABLE читает баланс Алисы, начисляет 1.00 ₽ процентов и коммитит
// первым. Пара read/write-зависимостей с транзакцией бонуса даёт 40001.
func nightlyInterest(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var balance int64
	if err := tx.QueryRow(ctx,
		`SELECT balance_cents FROM cap_members WHERE id = 1`).Scan(&balance); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE cap_members SET balance_cents = $1 WHERE id = 1`, balance+100); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
