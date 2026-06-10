// Команда demo юнита 00-05: что пул делает с соединениями под капотом.
//
// Два режима:
//
//	demo          — заглянуть в жизненный цикл пула: лень, захват, pg_stat_activity, возврат;
//	demo -reset   — накатить канон Brew (schema + seed) и выйти (цель db-reset).
//
// Это raw-pgx юнит (escape-hatch): урок про сам пул (Acquire/Release/Stat) и про
// то, как соединения видны серверу в pg_stat_activity, — это API пула и системная
// вьюха, а не маппинг строк, поэтому sqlc тут не к месту. Чтобы наблюдать ровно
// свои бэкенды, пул помечен application_name через кастомный Option (escape-hatch
// поверх pg.WithMaxConns). Логи — в stderr, stdout — только результат.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

// appName помечает все соединения этого пула — по нему мы отфильтруем в
// pg_stat_activity ровно свои бэкенды, не считая чужих клиентов песочницы.
const appName = "brew-pool-demo"

// poolSize — небольшой предсказуемый размер, чтобы счётчики были наглядны.
const poolSize = 4

func main() {
	logger := log.New()

	reset := flag.Bool("reset", false, "накатить канон Brew (schema + seed) и выйти")
	flag.Parse()

	ctx, cancel := runctx.New()
	defer cancel()

	if err := run(ctx, *reset); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("demo failed", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, reset bool) error {
	// Два Option'а: штатный pg.WithMaxConns и кастомный — он проставляет
	// application_name в стартовый пакет каждого соединения. Сигнатура Option —
	// func(*pgxpool.Config), поэтому такой литерал передаётся как обычный опцион
	// (escape-hatch: пул донастраивается под нужды конкретного урока).
	pool, err := pg.NewPool(ctx,
		pg.WithMaxConns(poolSize),
		func(c *pgxpool.Config) {
			c.ConnConfig.RuntimeParams["application_name"] = appName
		},
	)
	if err != nil {
		return fmt.Errorf("pg.NewPool: %w", err)
	}
	defer pool.Close()

	if reset {
		if err := pool.Ping(ctx); err != nil {
			return fmt.Errorf("ping (песочница поднята? `docker compose up -d`): %w", err)
		}
		if err := brew.Reset(ctx, pool); err != nil {
			return fmt.Errorf("brew.Reset: %w", err)
		}
		fmt.Println("Канон Brew накатан: схема + seed-данные на месте.")
		return nil
	}

	fmt.Printf("Пул создан: MaxConns=%d, application_name=%q.\n\n", poolSize, appName)

	// 1) Пул ленивый: сразу после NewPool ни одного соединения ещё нет.
	fmt.Println("1) Сразу после NewPool пул ленив — соединений ещё нет:")
	printStat(pool)

	// 2) Захватываем все 4 соединения и НЕ возвращаем — пул вынужден открыть
	// 4 реальных бэкенда (новый коннект, раз свободных в пуле нет).
	conns := make([]*pgxpool.Conn, 0, poolSize)
	// Подстраховка на случай раннего return (ошибка/отмена): вернуть все уже
	// захваченные соединения, иначе deferred pool.Close() зависнет, ожидая
	// checked-out коннекты. Регистрируем ДО цикла — замыкание читает conns в
	// момент выхода, поэтому покрывает и обрыв Acquire в середине цикла (часть
	// коннектов уже захвачена). Явный Release в шаге 4 остаётся — он идемпотентен.
	defer func() {
		for _, c := range conns {
			c.Release()
		}
	}()
	for i := 0; i < poolSize; i++ {
		c, err := pool.Acquire(ctx)
		if err != nil {
			return fmt.Errorf("pool.Acquire #%d: %w", i, err)
		}
		conns = append(conns, c)
	}
	fmt.Printf("\n2) Захватили %d соединения (pool.Acquire) — пул открыл столько реальных бэкендов:\n", poolSize)
	printStat(pool)

	// 3) Спросим сам сервер, сколько бэкендов с нашим application_name он видит.
	// Пул исчерпан (все 4 заняты), поэтому запрос идём делать по одному из уже
	// захваченных соединений — иначе pool.Query заблокировался бы в ожидании.
	var backends int64
	const countSQL = "SELECT count(*) FROM pg_stat_activity WHERE application_name = $1"
	if err := conns[0].QueryRow(ctx, countSQL, appName).Scan(&backends); err != nil {
		return fmt.Errorf("count pg_stat_activity: %w", err)
	}
	fmt.Printf("\n3) Сколько бэкендов с application_name=%q видит Postgres (pg_stat_activity):  %d\n", appName, backends)

	// 4) Возвращаем соединения в пул. Release НЕ закрывает коннект — он остаётся
	// открытым и простаивает, готовый к переиспользованию (в этом весь смысл пула).
	for _, c := range conns {
		c.Release()
	}
	fmt.Printf("\n4) Вернули все %d в пул (conn.Release) — соединения не закрылись, а простаивают:\n", poolSize)
	printStat(pool)

	return nil
}

// printStat печатает срез статистики пула. TotalConns = открытые сейчас коннекты
// (занятые + простаивающие); AcquiredConns — выданные в работу; IdleConns —
// открытые, но свободные.
func printStat(pool *pgxpool.Pool) {
	s := pool.Stat()
	fmt.Printf("   всего=%d  занято=%d  простаивают=%d  (макс=%d)\n",
		s.TotalConns(), s.AcquiredConns(), s.IdleConns(), s.MaxConns())
}
