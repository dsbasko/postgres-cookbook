# 09-03 — Transactional outbox: fact and event, atomically

A customer placed an order. Brew has to do two things: write the order to its own
database and announce it to the outside — so an email goes out, the analytics
dashboard updates, and, in our universe, so that `kafka-cookbook` picks the event
up. A naive backend writes to two places in a row: `INSERT` the order into
Postgres, then `publish` the event to a broker. And right between those two lines
lives a bug invisible on the happy path. The service crashes after the `INSERT`
but before the `publish` — the order exists, the event doesn't, no email went out,
Kafka knows nothing about the order. Or the other way around: the event was
published, but the order's transaction rolled back — and now an event about an
order that isn't in the database lives in the outside world. Two sources of truth
cannot be updated atomically: a distributed transaction between a database and a
broker is expensive and brittle.

The outbox closes the gap with a simple move: **don't write to two places at
once**.

## The idea: the event is just another row in the same database

Instead of "order to the DB + event to the broker" we write **both the order and
the event into the same database, in one transaction**. The event lands as a row
in the `outbox` table. Postgres provides the atomicity: either both inserts commit
or neither does — no window between them exists. An order without an event, or an
event without an order, is now impossible in principle.

Delivery to the outside is taken over by a separate process — the **relay**. It
reads unpublished `outbox` rows, sends them to the broker (or hands them to CDC),
and marks them delivered. Delivery has become a separate, repeatable task: if the
relay dies midway, it restarts and reads on; it doesn't promise "exactly once",
but it does promise "at least once" (the consumer on the other side dedupes by
`outbox_id` (table `processed_outbox_ids`)).

The `orders` and `outbox` tables here are **base tables**, from `schema/brew.sql`,
byte-compatible with `kafka-cookbook`. That is no accident: this very pair travels
into capstone 10-05 and onward — into the CDC handoff to the Kafka course. So the
unit defines no lab tables of its own and works on the real Brew base tables.

## Writing: order and event in one transaction

In code this is literally one transaction over two `INSERT`s:

```go
tx, _ := pool.Begin(ctx)
defer tx.Rollback(ctx)
q := db.New(tx)               // the same sqlc queries, but within the transaction

orderID, _ := q.InsertOrder(ctx, db.InsertOrderParams{...})
payload, _ := json.Marshal(map[string]any{"order_id": orderID, "amount": o.amount})
q.InsertOutbox(ctx, db.InsertOutboxParams{AggregateID: ..., Topic: "orders.created", Payload: payload})

tx.Commit(ctx)
```

`db.New(tx)` is the same typed sqlc wrapper, but bound to the transaction (as in
05-01/03-03). If the second `INSERT` fails, `defer tx.Rollback` undoes the first
too. The event physically cannot outlive the order, or vice versa.

`InsertOutbox` leaves `published_at` as `NULL` — that is the "not delivered yet"
marker.

## Delivery: a relay on FOR UPDATE SKIP LOCKED

The relay drains unpublished events with the same trick as the queue in 09-02:

```sql
SELECT id, aggregate_id, topic, payload
FROM outbox
WHERE published_at IS NULL
ORDER BY id
FOR UPDATE SKIP LOCKED
LIMIT $1;
```

`WHERE published_at IS NULL` rides the partial index `outbox_unpublished_idx` (it
exists in the base schema) — the relay doesn't scan the whole outbox history, only the
tail of undelivered rows. `FOR UPDATE SKIP LOCKED` lets you spin up **several**
relay workers: they drain the events with no duplicates and no blocking on each
other. Having delivered an event, the relay does `UPDATE outbox SET published_at =
now()` in the same transaction — "claimed and marked" atomically.

## The dual-write gap — and how outbox closes it

The problem and the fix, side by side:

```
Naive: two places, a gap between them
  ① INSERT order → Postgres ──COMMIT──►  order written
        ╳ crash right HERE ──────────────►  event lost
  ② publish event → broker ─────────────►  (never arrived)
     net: order without event; or, if ① rolls back, event without order

Outbox: one place, one transaction
  BEGIN
    INSERT order → orders                   ┐ both inserts are atomic:
    INSERT event → outbox (published_at=∅)  ┘ either both or neither
  COMMIT
    relay: SELECT … WHERE published_at IS NULL  FOR UPDATE SKIP LOCKED
           → delivered outside → UPDATE published_at = now()
```

Delivery is now a separate, repeatable relay task, not a second line next to the
business write. As a guarantee that gives **at-least-once**:

| Guarantee | What it means | Where in our scheme |
|---|---|---|
| at-most-once | at most once, loss possible | naive `publish` with no retry; `NOTIFY` (09-04) |
| at-least-once | at least once, duplicates possible | outbox + relay with retry — **our case** |
| exactly-once | exactly once | unreachable in delivery; emulated as at-least-once + an idempotent consumer: dedup by `outbox_id` (table `processed_outbox_ids`) |

This is the watershed between the **two ways** to push changes outward:
**09-03 outbox is application-level** (you write an event row and run the relay
yourself), while **10-05 CDC is database-level** (Postgres hands its own WAL to
logical replication, with no event table and no relay of your own; Debezium reads
the base tables directly). Not two steps of one process, but two different entry
points — you pick one.

## What our code shows

`query.sql` is the protagonist: `InsertOrder`/`InsertOutbox` (writing the pair),
`ClaimUnpublished` (relay read under `SKIP LOCKED`), `MarkPublished` (delivery
mark). `cmd/demo/main.go` is thin: it places three orders with events, shows that
a rolled-back transaction leaves neither order nor event, and runs the relay once.

## Running it

```sh
docker compose up -d
make lecture L=09-writes-eventing-and-server-logic/09-03-transactional-outbox T=db-reset
make lecture L=09-writes-eventing-and-server-logic/09-03-transactional-outbox
```

`T=run` is the default and can be omitted. From inside the unit directory it is
shorter: `make db-reset`, then `make run`.

```
1) Кладём заказы — каждый ВМЕСТЕ с событием в одной транзакции:
   заказ  событие outbox  тема
   #1     #1              orders.created
   #2     #2              orders.created
   #3     #3              orders.created
   → событий ждёт доставки: 3

2) Транзакция «заказ записан, но проверка провалилась» → ROLLBACK:
   → заказов в таблице: 3, событий ждёт доставки: 3 (откат не оставил ничего)

3) relay вычитывает события (FOR UPDATE SKIP LOCKED) и доставляет их:
   событие  тема            aggregate  payload
   #1       orders.created  1          {"amount": "5.00", "order_id": 1}
   #2       orders.created  2          {"amount": "3.00", "order_id": 2}
   #3       orders.created  3          {"amount": "9.60", "order_id": 3}
   → событий ждёт доставки: 0
```

Three orders landed together with three events. The fourth transaction wrote an
order and "failed a check" — after the rollback there are still 3 orders and 3
events: the rollback removed both inserts at once. The relay drained all three
events, "delivered" them (the `payload` came back as normalized jsonb — keys
sorted) and marked them published — nothing is left in the queue.

## The fence

- **At-least-once, not exactly-once.** You are still responsible for the delivery
  guarantee: if the relay died between `publish` and `UPDATE published_at`, the
  event goes out again after a restart. So the consumer side needs
  **idempotency** — dedup by `outbox_id` (table `processed_outbox_ids`).
- **Keep the write transaction short.** Do the actual send to the broker in the
  relay outside the read transaction — otherwise a slow network to the broker will
  hold locks and the visibility horizon (see 05-02 and the 09-02 fence).
- **`outbox` is a high-churn table, a source of bloat.** It grows constantly and
  is cleaned just as constantly. In production it needs periodic cleanup (deleting
  long-published rows) and attention to autovacuum — but that is operations, your
  DBA's territory, and we don't touch it here.
- **The relay-versus-CDC fork.** You can write the relay by hand (as here — read
  `outbox` and publish), or write no relay at all: feed the base tables into **logical
  replication** and pick the changes up via CDC (Debezium). The second path is
  capstone 10-05 (`REPLICA IDENTITY FULL` on the CDC sources + `CREATE
  PUBLICATION`); there CDC works at the database level and is presented as an
  **alternative** to the hand-written outbox relay, not its continuation. Debezium
  from `kafka-cookbook` reads our tables without rewriting the schema.

## Takeaways

The transactional outbox solves the two-sources-of-truth problem in one move:
don't write to the DB and the broker separately — write the **business fact and
the event about it into one database in one transaction**, with Postgres providing
the atomicity. Delivery to the outside moves into a separate **relay** that reads
unpublished `outbox` rows via `FOR UPDATE SKIP LOCKED` (several workers, no
duplicates) and marks them delivered. This is at-least-once: the consumer must be
idempotent. The base `orders`/`outbox` here are no accident — they are the
very pair that ships into CDC in 10-05.

Next — another way to learn of a change immediately, without polling a table in a
loop: the database itself pushes a notification. In 09-04 a trigger on `INSERT`
sends `pg_notify`, and a listener on the Go side receives the event in real time —
with important caveats about transactionality, size, and "at-most-once delivery".
