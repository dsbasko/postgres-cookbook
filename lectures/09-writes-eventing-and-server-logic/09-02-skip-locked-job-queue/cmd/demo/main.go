// Команда demo юнита 09-02: очередь задач на FOR UPDATE SKIP LOCKED.
//
// Два режима:
//
//	demo          — N воркеров параллельно разбирают одну очередь задач; каждая
//	                задача достаётся ровно одному воркеру, без дублей и без того,
//	                чтобы воркеры блокировали друг друга;
//	demo -reset   — накатить канон Brew (схема + seed) и выйти (db-reset).
//
// Это raw-pgx, Go-центричный escape-hatch до sqlc: урок про конкурентность
// (несколько горутин-воркеров, каждый со своей транзакцией) и про приём
// FOR UPDATE SKIP LOCKED, а не про форму одного запроса.
//
// Какому воркеру какая задача достанется — НЕДЕТЕРМИНИРОВАНО (в этом и смысл
// SKIP LOCKED: воркеры сами балансируют нагрузку). Поэтому в stdout печатаем
// только ИНВАРИАНТЫ, которые не зависят от планировщика: всего обработано,
// дублей, потеряно. Они стабильны от прогона к прогону.
// Логи — в stderr, stdout — только результат (для дословной вставки в README).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

const (
	totalJobs  = 12 // задач в очереди
	numWorkers = 4  // параллельных воркеров
)

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
	// Воркеров несколько, каждому нужен свой коннект из пула одновременно —
	// поднимаем потолок до numWorkers, иначе они выстроятся в очередь за
	// соединением и «конкурентности» не выйдет.
	pool, err := pg.NewPool(ctx, pg.WithMaxConns(numWorkers))
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

	if err := setupQueue(ctx, pool); err != nil {
		return fmt.Errorf("setupQueue: %w", err)
	}
	fmt.Printf("1) В очередь jobs_lab поставлено задач: %d. Воркеров: %d.\n", totalJobs, numWorkers)
	fmt.Println("   Каждый воркер в цикле: BEGIN → SELECT ... FOR UPDATE SKIP LOCKED LIMIT 1 → обработать → COMMIT.")

	// Запускаем воркеров. Каждый возвращает список id задач, которые он забрал.
	// Кто сколько взял — зависит от планировщика, поэтому списки сводим вместе
	// и проверяем ИНВАРИАНТ: множество забранных id = {1..totalJobs}, без дублей.
	var (
		mu      sync.Mutex
		claimed []int64
		wg      sync.WaitGroup
	)
	wg.Add(numWorkers)
	for w := 1; w <= numWorkers; w++ {
		go func(workerID int) {
			defer wg.Done()
			ids, werr := worker(ctx, pool, workerID)
			mu.Lock()
			claimed = append(claimed, ids...)
			if werr != nil {
				err = errors.Join(err, fmt.Errorf("воркер %d: %w", workerID, werr))
			}
			mu.Unlock()
		}(w)
	}
	wg.Wait()
	if err != nil {
		return err
	}

	// 2) Инвариант №1: ни одной задачи не потеряно и ни одна не взята дважды.
	sort.Slice(claimed, func(i, j int) bool { return claimed[i] < claimed[j] })
	unique := uniqueCount(claimed)
	duplicates := len(claimed) - unique
	fmt.Println("\n2) Свод по забранным задачам (инварианты, не зависят от планировщика):")
	fmt.Printf("   забрано всего      : %d\n", len(claimed))
	fmt.Printf("   уникальных задач   : %d\n", unique)
	fmt.Printf("   дублей (один job двум воркерам): %d\n", duplicates)

	// 3) Инвариант №2: в базе все задачи done, в очереди не осталось ни одной.
	done, queued, err := queueCounts(ctx, pool)
	if err != nil {
		return err
	}
	fmt.Println("\n3) Состояние очереди в базе после прогона:")
	fmt.Printf("   status='done'   : %d\n", done)
	fmt.Printf("   status='queued' : %d\n", queued)

	return nil
}

// worker крутит цикл, пока в очереди есть задачи. Возвращает id задач, которые
// этот воркер успешно обработал.
//
// Ключ урока — один запрос внутри транзакции:
//
//	SELECT ... WHERE status='queued' ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1
//
// FOR UPDATE блокирует выбранную строку, SKIP LOCKED велит ПРОПУСКАТЬ строки,
// уже заблокированные другими воркерами (а не ждать их). Так два воркера никогда
// не возьмут одну задачу и не встанут в очередь друг за другом: каждый
// мгновенно получает следующую СВОБОДНУЮ строку.
func worker(ctx context.Context, pool *pgxpool.Pool, workerID int) ([]int64, error) {
	var processed []int64
	for {
		id, ok, err := claimOne(ctx, pool, workerID)
		if err != nil {
			return processed, err
		}
		if !ok {
			return processed, nil // очередь пуста — воркер завершается
		}
		processed = append(processed, id)
	}
}

// claimOne берёт одну задачу под блокировкой, «обрабатывает» (помечает done с
// именем воркера) и коммитит. Возвращает (id, true) при успехе или (_, false),
// если свободных задач больше нет.
func claimOne(ctx context.Context, pool *pgxpool.Pool, workerID int) (int64, bool, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback(ctx) // на закоммиченной — no-op; на ошибке — откат

	var id int64
	err = tx.QueryRow(ctx, `
		SELECT id FROM jobs_lab
		WHERE status = 'queued'
		ORDER BY id
		FOR UPDATE SKIP LOCKED
		LIMIT 1`).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil // свободных задач нет
	}
	if err != nil {
		return 0, false, err
	}

	// «Обработка»: помечаем задачу выполненной и фиксируем, кто её взял.
	if _, err := tx.Exec(ctx,
		`UPDATE jobs_lab SET status = 'done', claimed_by = $1 WHERE id = $2`,
		workerID, id); err != nil {
		return 0, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, false, err
	}
	return id, true, nil
}

// setupQueue пересоздаёт очередь и ставит в неё totalJobs задач в статусе
// 'queued'. DROP+CREATE+seed делают run идемпотентным.
//
// DDL и INSERT — двумя вызовами Exec намеренно: многооператорную строку pgx
// гонит простым протоколом (можно), но как только появляется аргумент ($1) —
// переходит на extended-протокол, который несколько команд в одной строке не
// принимает (SQLSTATE 42601). Поэтому DROP+CREATE без аргументов, а
// параметризованный INSERT — отдельно.
func setupQueue(ctx context.Context, pool *pgxpool.Pool) error {
	const ddl = `
		DROP TABLE IF EXISTS jobs_lab;
		CREATE TABLE jobs_lab (
			id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
			payload    text   NOT NULL,
			status     text   NOT NULL DEFAULT 'queued',
			claimed_by int    NULL
		);`
	if _, err := pool.Exec(ctx, ddl); err != nil {
		return err
	}
	_, err := pool.Exec(ctx,
		`INSERT INTO jobs_lab (payload) SELECT 'job #' || g FROM generate_series(1, $1) AS g`,
		totalJobs)
	return err
}

// queueCounts возвращает число задач в статусах done и queued.
func queueCounts(ctx context.Context, pool *pgxpool.Pool) (done, queued int, err error) {
	err = pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE status = 'done'),
			count(*) FILTER (WHERE status = 'queued')
		FROM jobs_lab`).Scan(&done, &queued)
	return done, queued, err
}

// uniqueCount считает уникальные значения в ОТСОРТИРОВАННОМ срезе.
func uniqueCount(sorted []int64) int {
	n := 0
	for i, v := range sorted {
		if i == 0 || v != sorted[i-1] {
			n++
		}
	}
	return n
}
