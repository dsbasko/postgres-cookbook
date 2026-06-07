// Команда demo юнита 09-03: transactional outbox.
//
// Два режима:
//
//	demo          — кладём заказы, каждый вместе с событием о нём в ОДНОЙ
//	                транзакции (атомарно); показываем, что откат тянет за собой
//	                и заказ, и событие; затем relay вычитывает события через
//	                FOR UPDATE SKIP LOCKED и «доставляет» их;
//	demo -reset   — накатить канон Brew (схема + seed) и выйти (db-reset).
//
// Это канонический sqlc-юнит: query.sql написан руками, internal/db/ сгенерён
// и закоммичен. Работает на КАНОНИЧЕСКИХ таблицах orders и outbox — той самой
// паре, что едет в capstone 10-05 и в CDC-эстафету к kafka-cookbook.
//
// Детерминизм: в начале run рабочие таблицы (orders/outbox/order_items)
// очищаются TRUNCATE ... RESTART IDENTITY → id заказов и событий идут с 1,
// вывод воспроизводится дословно. published_at (now()) не печатаем.
// Логи — в stderr, stdout — только результат.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dsbasko/postgres-cookbook/lectures/09-writes-eventing-and-server-logic/09-03-transactional-outbox/internal/db"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

// newOrder — заказ, который мы кладём вместе с событием о нём.
type newOrder struct {
	customerID string
	amount     string
}

var orders = []newOrder{
	{customerID: "1", amount: "5.00"},
	{customerID: "2", amount: "3.00"},
	{customerID: "1", amount: "9.60"},
}

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

	// Чистим рабочие таблицы канона → детерминированные id заказов/событий с 1.
	if _, err := pool.Exec(ctx,
		`TRUNCATE order_items, orders, outbox RESTART IDENTITY CASCADE`); err != nil {
		return fmt.Errorf("truncate: %w", err)
	}

	queries := db.New(pool)

	// 1) Атомарная запись: заказ + событие о нём в ОДНОЙ транзакции.
	fmt.Println("1) Кладём заказы — каждый ВМЕСТЕ с событием в одной транзакции:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "   заказ\tсобытие outbox\tтема")
	for _, o := range orders {
		orderID, outboxID, err := placeOrderWithEvent(ctx, pool, o)
		if err != nil {
			return fmt.Errorf("placeOrderWithEvent: %w", err)
		}
		fmt.Fprintf(w, "   #%d\t#%d\torders.created\n", orderID, outboxID)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	unpub, err := queries.CountUnpublished(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("   → событий ждёт доставки: %d\n", unpub)

	// 2) Атомарность: транзакция, которая упала ПОСЛЕ записи заказа, не оставляет
	// ни заказа, ни события. Откат тянет за собой обе вставки разом.
	fmt.Println("\n2) Транзакция «заказ записан, но проверка провалилась» → ROLLBACK:")
	if err := rolledBackOrder(ctx, pool); err != nil {
		return fmt.Errorf("rolledBackOrder: %w", err)
	}
	orderCnt, err := queries.CountOrders(ctx)
	if err != nil {
		return err
	}
	unpub, err = queries.CountUnpublished(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("   → заказов в таблице: %d, событий ждёт доставки: %d (откат не оставил ничего)\n", orderCnt, unpub)

	// 3) relay: вычитывает неопубликованные события под FOR UPDATE SKIP LOCKED,
	// «доставляет» (печатает) и помечает published — всё в одной транзакции.
	fmt.Println("\n3) relay вычитывает события (FOR UPDATE SKIP LOCKED) и доставляет их:")
	if err := relayOnce(ctx, pool); err != nil {
		return fmt.Errorf("relayOnce: %w", err)
	}
	unpub, err = queries.CountUnpublished(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("   → событий ждёт доставки: %d\n", unpub)

	return nil
}

// placeOrderWithEvent пишет заказ и событие о нём в ОДНОЙ транзакции. Если упадёт
// любая из вставок — откатятся обе: в этом весь смысл outbox (атомарность факта
// и события даёт сам Postgres, без распределённой транзакции с брокером).
func placeOrderWithEvent(ctx context.Context, pool *pgxpool.Pool, o newOrder) (orderID, outboxID int64, err error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx) // на закоммиченной — no-op

	q := db.New(tx)

	var amount pgtype.Numeric
	if err := amount.Scan(o.amount); err != nil {
		return 0, 0, fmt.Errorf("amount %q: %w", o.amount, err)
	}
	orderID, err = q.InsertOrder(ctx, db.InsertOrderParams{
		CustomerID: o.customerID,
		Amount:     amount,
		Status:     "created",
	})
	if err != nil {
		return 0, 0, err
	}

	payload, err := json.Marshal(map[string]any{"order_id": orderID, "amount": o.amount})
	if err != nil {
		return 0, 0, err
	}
	outboxID, err = q.InsertOutbox(ctx, db.InsertOutboxParams{
		AggregateID: strconv.FormatInt(orderID, 10),
		Topic:       "orders.created",
		Payload:     payload,
	})
	if err != nil {
		return 0, 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, err
	}
	return orderID, outboxID, nil
}

// rolledBackOrder пишет заказ в транзакции, после чего «бизнес-проверка падает»
// и мы откатываемся. Демонстрирует, что заказ (а с ним и любое событие) не
// переживает откат — атомарность сохраняется.
func rolledBackOrder(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	q := db.New(tx)
	var amount pgtype.Numeric
	if err := amount.Scan("1.00"); err != nil {
		return err
	}
	if _, err := q.InsertOrder(ctx, db.InsertOrderParams{
		CustomerID: "3",
		Amount:     amount,
		Status:     "created",
	}); err != nil {
		return err
	}
	// Здесь «проверка не прошла» (например, недостаточно остатка) → не коммитим.
	// Явный Rollback вместо ожидания defer — чтобы показать намерение.
	return tx.Rollback(ctx)
}

// relayOnce — один проход relay'я: в транзакции забирает неопубликованные
// события под FOR UPDATE SKIP LOCKED, «доставляет» (печатает) и помечает
// published. Несколько таких relay-воркеров могли бы идти параллельно благодаря
// SKIP LOCKED, не доставляя одно событие дважды (та же механика, что в 09-02).
func relayOnce(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	q := db.New(tx)
	events, err := q.ClaimUnpublished(ctx, 100)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "   событие\tтема\taggregate\tpayload")
	for _, e := range events {
		fmt.Fprintf(w, "   #%d\t%s\t%s\t%s\n", e.ID, e.Topic, e.AggregateID, string(e.Payload))
		if err := q.MarkPublished(ctx, e.ID); err != nil {
			return err
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
