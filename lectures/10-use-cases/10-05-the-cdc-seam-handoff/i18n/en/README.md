# 10-05 — The CDC seam: handoff to kafka-cookbook

Brew's menu changes on its own: a barista edits the espresso price, marketing
publishes a new blog article, a customer's phone number changes. And every such
change has to reach the neighbouring world — `kafka-cookbook`, where the very
same coffee-chain Brew already lives as a stream of events in Kafka, feeds
Elasticsearch, and powers menu search. The question of the whole course's finale
is just one: **how do we hand our stream of changes to the Kafka course without
rewriting anything on that side?**

The answer is Change Data Capture. We write no relay of our own (that was the
09-03 path), we set up no triggers. We configure Postgres logical replication on
exactly the tables we need, and Debezium from `kafka-cookbook` connects to it and
reads the stream itself. Postgres becomes the source of truth, and the WAL
becomes our event bus. This unit assembles the seam and proves it holds.

## CDC instead of a relay: the WAL is already a changelog

In 09-03 we manually wrote an event as a row into `outbox` and drained it with a
relay. CDC comes at it from the other side: **the change is already written** —
in the WAL, the write-ahead log that Postgres uses to guarantee durability
anyway. Logical decoding parses the WAL back into logical `INSERT`/`UPDATE`/`DELETE`
per table, and hands them to a consumer. You write no delivery code at all: the
relay is Postgres itself plus a decoder on the consumer's side.

To make the stream targeted rather than "the whole server", CDC relies on two
settings: the **PUBLICATION** (which tables we stream) and the **REPLICA
IDENTITY** (how much of the old row to write to the WAL on `UPDATE`/`DELETE`). We
set both explicitly.

## Three sources and their REPLICA IDENTITY FULL

Three canonical tables travel into the CDC handoff: `drinks` (the menu),
`articles` (the blog), `customers` (the customer directory). They already carry
`REPLICA IDENTITY FULL` in the canon — and that is not cosmetic.

By default (`REPLICA IDENTITY DEFAULT`), on `UPDATE`/`DELETE` Postgres writes only
the primary key of the old row to the WAL — enough for a physical replica to
locate the row. But Debezium on the other side builds full "before → after"
events from the stream, and for `UPDATE`/`DELETE` it needs the **before-image** —
the whole prior state of the row. With `DEFAULT` it sees a single `id` in the
before-image and cannot reconstruct what actually changed. `REPLICA IDENTITY
FULL` tells Postgres to write the **entire old row** to the WAL — then the
before-image holds every column.

## PUBLICATION: an explicit list instead of autocreate

The stream is addressed by a publication:

```sql
CREATE PUBLICATION dbz_publication FOR TABLE drinks, articles, customers;
```

We list the tables **by hand** rather than enabling `publication.autocreate` on
the Debezium side. That way one place shows exactly what goes into the stream:
three tables, no more, no less. Removing a table from the stream is `ALTER
PUBLICATION dbz_publication DROP TABLE <name>`, adding one is `ADD TABLE`. No
magic "it'll pick up whatever it finds".

## Proving the seam via test_decoding

Assembling the configuration isn't enough — we have to show the before-image
really carries the whole row. So the demo creates a **temporary logical
replication slot** with the `test_decoding` output plugin, does one `UPDATE` on a
drink, drains the slot's changes via `pg_logical_slot_get_changes`, and counts how
many columns landed in the `old-key` segment (that is the before-image in
`test_decoding`'s format). Under `REPLICA IDENTITY FULL` there are all 9 columns
of `drinks`, not a single `id`. The slot is dropped right after the check.

`test_decoding` is a debugging plugin that prints changes as text; in production
Debezium uses its own decoder, not this one. We need it purely to **see** the
before-image with our own eyes and confirm that `FULL` works.

## What our code shows

`cmd/demo/main.go` is a raw-pgx escape-hatch: the lesson is replication
configuration (`PUBLICATION`, `REPLICA IDENTITY`, slots) and system decoding
functions — these are DDL and `pg_*` calls, not sqlc-level SQL, so there is no
`query.sql` and no `internal/db/` here. The demo runs in sequence: it applies the
canon, applies the handoff artifact `db/init.sql`, shows the `REPLICA IDENTITY` of
the three sources, prints the published tables, and proves the before-image via
`test_decoding`. The `db/init.sql` artifact is the very file that is
byte-compatible with `kafka-cookbook` and ships to its side; re-applying it is
idempotent (`CREATE TABLE IF NOT EXISTS`, a repeated `ALTER ... REPLICA IDENTITY
FULL`, a `DO` block for the publication).

## Running it

```sh
docker compose up -d
make lecture L=10-use-cases/10-05-the-cdc-seam-handoff T=db-reset
make lecture L=10-use-cases/10-05-the-cdc-seam-handoff
```

`T=run` is the default and can be omitted. From inside the unit directory it is
shorter: `make db-reset`, then `make run`. And `make test` runs the asserted
integration test: it checks that the publication covers exactly the three tables,
that all of them carry `REPLICA IDENTITY FULL`, and that the before-image holds 9
columns (without a running sandbox the test does `t.Skip`). The unit requires
`wal_level=logical` — it is already set in the course's root `docker-compose.yml`.

```
1) Канон на месте, REPLICA IDENTITY FULL на CDC-источниках:
   articles   replica identity: full
   customers  replica identity: full
   drinks     replica identity: full

2) Публикация для Debezium (явный список таблиц):
   CREATE PUBLICATION dbz_publication FOR TABLE drinks, articles, customers
   публикует таблицы: articles, customers, drinks

3) Проверяем шов логическим декодированием (test_decoding):
   UPDATE drinks #1 → перехвачено изменений в слоте: 3
   before-image (old-key) содержит столбцов: 9 → REPLICA IDENTITY FULL работает
   (без FULL Debezium увидел бы в before-image только id; здесь — всю строку)

4) Эстафета: db/init.sql байт-совместим с kafka-cookbook — Debezium
   читает наши таблицы без переписывания схемы. Дальше — Kafka-курс.
```

All three sources carry `full`. The publication streams exactly `drinks`,
`articles`, `customers`. And the key line is the third one: a single `UPDATE
drinks` left three changes in the slot (`BEGIN`/`UPDATE`/`COMMIT`), and the
`UPDATE`'s before-image holds **9 columns** — the whole `drinks` row. That is the
proof that `REPLICA IDENTITY FULL` does its job: Debezium gets the full "before",
not a stub of a single `id`.

## The fence

A logical replication slot that **nobody drains** pins the WAL: Postgres cannot
delete log segments until the slowest consumer has confirmed them — and the disk
slowly fills. Our demo honestly drops the slot at the end, but in production a
stuck slot (a dead Debezium, a disconnected consumer) is a real path to `No space
left on device`. Slots must be watched (`pg_replication_slots`) and dead ones
cleaned up — that is your DBA's territory, and we don't touch it here.

`test_decoding` is a debugging plugin, not what Debezium reads with: it prints
changes as text for the eye, while Debezium has its own decoder. We took it only
to **see** the before-image.

`REPLICA IDENTITY FULL` is a tradeoff: you pay in WAL for a full before-image.
Every `UPDATE`/`DELETE` now writes the entire old row to the log instead of a
single PK — on a hot table with wide rows that is a noticeable rise in WAL volume
and replication load. On our three directories (menu, blog, customers) writes are
rare and the cost is pennies; on a high-churn table this decision has to be
weighed.

And the end-to-end pipeline itself — `Debezium → Kafka → sinks` — we do **not**
run here. That is already the `kafka-cookbook` side: the next course. Our job is
to hand off a correct stream, and that job is done.

## Takeaways

CDC is a way to hand a stream of changes outward without writing a line of
delivery: the WAL is already a changelog, logical decoding parses it back into
`INSERT`/`UPDATE`/`DELETE`, and the stream is addressed by the `PUBLICATION`
(which tables) and `REPLICA IDENTITY` (how much of the old row to write to the
WAL). `REPLICA IDENTITY FULL` puts the whole row into the before-image — without
it the consumer can't reconstruct `UPDATE`/`DELETE`, but the WAL grows too. This
is the alternative to the transactional outbox of 09-03: there we wrote the event
into `outbox` by hand, here the database's own log is the source.

And this is where the whole course closes. The protagonist throughout was **SQL**:
sqlc units kept the queries at the centre, and escape-hatches (like this one)
dropped to the level of DDL, MVCC, and system functions exactly when sqlc got in
the way of seeing the point. The final frame is the canon byte-compatibility rule
we held to from the first module: this unit's `db/init.sql` matches the column
names and types of `kafka-cookbook`'s `init.sql` **verbatim** (guarded by the
`TestInitSQL_ByteCompatTokens` test), so Debezium reads our
`drinks`/`articles`/`customers` without rewriting the schema. Rename even one
canon column here and the handoff breaks.

Next is the sibling course `kafka-cookbook` (github.com/dsbasko/kafka-cookbook).
It picks up exactly this stream: Debezium listens to our `dbz_publication`, puts
the changes into Kafka, and from there sinks travel into Elasticsearch and build
search over the same coffee-chain Brew. One world, one data model, two courses —
Postgres has handed off the baton, Kafka takes it. This was the last unit. Thank
you for making it to the end.
