# 00-01 — Meet Brew

Day one at Brew. The dev office is the second floor above the chain's very first coffee shop: a grinder rumbles downstairs, a dozen desks sit upstairs, and nobody here is surprised by the smell of espresso.

> **Marat:** Laptop issued, badge works — not bad for ten in the morning. I'm Marat, the backend team lead. The tour is short: our whole system is right here.
>
> **You:** One database?
>
> **Marat:** One database, nine tables: orders, line items, menu, customers, stock. People somehow expect something grand behind a chain of coffee shops.
>
> **Danya:** And the outbox. He always forgets the outbox, and it's our best table — you'll get it closer to winter. I'm Danya, the second backend dev, so now there are three of us. Advice for your first week: don't trust the word "just" around here. "Just add a column," "just fix a price" — that's how the register went down here.
>
> **Marat:** That happened once.
>
> **Danya:** That happened twice.

From behind a partition, without turning away from her two monitors, a woman with a liter mug — not a single word printed on it — speaks up.

> **Zoya:** Zoya. My database. The sandbox will be up by lunch. Don't ask for prod.
>
> **You:** What if I really need it?
>
> **Zoya:** Especially if you really need it.
>
> **Marat:** The boundary with Zoya is simple: everything inside the server is her territory. Everything that talks to the server with queries is ours. Where exactly that boundary runs, I'll be showing you all year. There's also Stas from marketing, one floor up: he shows up saying "small tweak," and it hasn't been true once yet.

Marat turns his laptop around. On the screen is the orders table, first row highlighted.

> **Marat:** Look. Order number one: Alice Ivanova, January fifteenth, nine o'clock sharp. A cappuccino and a cold brew, status paid. The first order in Brew's history — Viktor, our founder, still keeps the paper receipt framed. On this one row I'll show you just about everything: types, joins, transactions, indexes.
>
> **You:** A whole course — on one order?
>
> **Marat:** On one order, one database, and one year. Remember the two questions every investigation here starts with: "show me the query" and "what did the database say." By the end of the year these nine tables will ride out as a stream to a neighboring system — and you'll be the one opening that stream. But we'll start with a simpler question: what actually happens on the other end of the socket when an application talks to Postgres.

The socket question is the next unit's topic. This one isn't about SQL. It's a map: what Brew is, what we'll build over the course, how it ends, and which topics we'll cover along the way. From here on every unit opens with a concrete business pain and closes it with one Postgres technique — but it's worth seeing the whole route first, so you don't feel dropped into code from the first line.

## What Brew is

Brew started as a single coffee shop, and now it's becoming a chain: the Moscow spot (`BREW-CENTRAL`) is up and running, the second one — St. Petersburg's `BREW-NORTH` — is already set up in the database and waiting to open, its stock being pre-loaded. Plus a shared menu, a shared customer base, and its own application — the register, the site, promo campaigns, reports. All of it talks in queries to that same "one database" from the tour. Brew will keep growing — and that's not just backdrop. The bigger Brew gets, the more expensive a mistake becomes: what went unnoticed at one register turns into lost orders and a register that froze mid-shift across the chain. That's why the pain in this course escalates from module to module, along with Brew itself.

The data you'll live with for the whole course is exactly what Marat's tour named: the drinks menu (`drinks`), customers (`customers`), orders (`orders`) and their line items (`order_items`), shops (`shops`), per-shop stock (`inventory`), a blog (`articles`), and `outbox` — the very table Danya vouched for, through which order events leave for the outside world. The same set of tables runs through the entire course: from the first connection to the final capstone.

The anchor Marat highlighted on the screen — **order #1**, Alice Ivanova's — wasn't picked for looks. The course returns to this row again and again: on it we'll watch how JOINs work, what happens during a concurrent update, how to read a query plan. When later you hit the word "order" and it feels too generic — remember order #1.

## What we'll build, and how it ends

The course route is also the route of your growth at Brew. At the start you're a newcomer who glues SQL together with strings (and opens a hole for injection) and drops the connection pool under the first real load. By the end you're an engineer who defends invariants right in the schema, reads `EXPLAIN` instead of guessing, and writes a retry loop on a serialization conflict.

The finale is concrete — and Marat has already promised it. You'll open a `PUBLICATION` yourself and hand Brew's change stream to the sibling course [`kafka-cookbook`](https://github.com/dsbasko/kafka-cookbook): Postgres passes the baton, Kafka takes it. Two coffee stories — one world, one data model. That's why Brew's base tables in this course match the sibling's schema byte for byte: rename a column and the handoff breaks. There's a whole conversation about that in the capstone; for now just remember the course has an exit outward, not only an internal kitchen.

## The course map

Eleven modules, ordered by rising difficulty. Roughly bottom to top:

- **Getting connected** (this module) — client and server, the sandbox, `psql`, connecting from Go, the `sqlc` pipeline, the life of a connection and the pool.
- **Data types** — which type to pick and why: `numeric` vs `float` for money, `timestamptz` for time, `uuid` and `uuidv7`, enums/arrays, and an intro to `jsonb`.
- **Schema and constraints** — keys, `NOT NULL`, foreign keys, `UNIQUE`/`CHECK`, generated columns, and a migration mindset: which `ALTER` is instant and which freezes the register.
- **CRUD fluency** — `INSERT … RETURNING`, keyset pagination, safe `UPDATE`/`DELETE`, upsert via `ON CONFLICT`, and sober `NULL` semantics.
- **Querying across tables** — joins, aggregation, `DISTINCT ON`, subqueries, and CTEs: this is where data turns into answers to business questions.
- **Transactions, MVCC, and concurrency** — ACID, an MVCC mental model, row locks, isolation levels, retrying on `40001`, deadlocks.
- **Indexing and EXPLAIN** — performance through reading plans: B-tree and column order, when an index doesn't help, GIN, `CREATE INDEX CONCURRENTLY`.
- **JSONB, arrays, and search** — flexible data and in-database search: containment, SQL/JSON path, full-text search, and fuzzy search via `pg_trgm`.
- **Analytics, window functions, and LATERAL** — running totals, ranking, top-N per group, recursive CTEs, LATERAL as the N+1 killer.
- **Writes, eventing, and server logic** — `MERGE`/`COPY`, a job queue on `SKIP LOCKED`, the transactional outbox, `LISTEN`/`NOTIFY`, triggers.
- **Use cases** — end-to-end capstones with integration tests that tie the whole course into working apps, and that CDC seam outward.

What the course leaves out: replication, backups, server tuning, fleet monitoring, roles and extensions. That's the work of whoever runs the database, not whoever writes queries against it. Where a unit simplifies something a production system does differently, it says so honestly in a "fence" section.

## How the course works

The whole course runs against one sandbox — a Postgres 18 container plus Adminer, brought up with a single command from the repo root. There's one database, `brew`, for the whole course: each unit layers its own schema on top of the shared one via `make db-reset`. No per-unit container needed.

Every unit leaves an observable trace: rows changed, a plan you can read, a publication you can stream. We write SQL by hand — it doesn't dissolve into a query builder; for most units the pipeline is "`query.sql` by hand → `sqlc generate` → typed pgx code," and that's the backbone of the course (we'll unpack it in detail a few units in, once we reach typed queries). And when a lesson needs interactivity, system columns, or `EXPLAIN`, we set `sqlc` aside and write psql scripts or raw pgx — we choose the capability, not the tool.

One detail that keeps coming up: the demo output in the README is real and deterministic. No `now()`, uuid values, or random numbers in stdout. If a number is printed, it's the same on any machine and across any number of runs — so you can trust it and diff it byte for byte. This isn't pedantry for its own sake: reproducibility is what separates "works on my machine" from "works for everyone."

## How each unit is built

Units follow one template, and it's worth learning now — it repeats some sixty times ahead. A unit opens with a Brew business pain (a drink vanished from the menu, a report didn't add up to the kopek, the register froze under load), then the concept is assembled in prose stage by stage — the why before the how. Then comes the `## Running it` section with the real demo output, which we read back right away by facts. And at the very end of each unit there are two sections worth calling out separately, because they repeat everywhere.

**"The fence"** is the boundary of simplifications. Any teaching example cuts something for clarity: one database for the whole course, `sslmode=disable`, the password `brew`/`brew` right in the connection string. The fence is where the unit honestly shows where it simplified and how the same thing is done in production (often in the words "your DBA would do this instead"). The name is literal: past this fence the teaching sandbox ends and production begins. Read it so you don't carry a simplification from the lesson into shipping code — that's the difference people later get burned on in review.

**"Takeaways"** is three or four bullets with the gist of the unit: what should stay in your head when the details fade a month later. If you're skimming a unit or coming back to it as a cheat-sheet — read at least this.

## What our code shows

First contact. We connect to the sandbox, ask the server for its version (to confirm it's Postgres 18 on the other end), and take a census of the Brew world — checking Marat's words: how many rows sit in each of the 9 canon tables after the seed. No SQL heroics, just "look around before getting to work."

`query.sql` — two hand-written queries:

```sql
-- name: ServerVersion :one
SELECT version();

-- name: BrewWorld :many
-- Перепись мира Brew: сколько строк в каждой таблице канона.
SELECT entity, n FROM (
    SELECT 1 AS ord, 'customers'::text     AS entity, count(*) AS n FROM customers
    UNION ALL SELECT 2, 'drinks',               count(*) FROM drinks
    UNION ALL SELECT 3, 'articles',             count(*) FROM articles
    UNION ALL SELECT 4, 'orders',               count(*) FROM orders
    UNION ALL SELECT 5, 'outbox',               count(*) FROM outbox
    UNION ALL SELECT 6, 'processed_outbox_ids', count(*) FROM processed_outbox_ids
    UNION ALL SELECT 7, 'shops',                count(*) FROM shops
    UNION ALL SELECT 8, 'order_items',          count(*) FROM order_items
    UNION ALL SELECT 9, 'inventory',            count(*) FROM inventory
) w
ORDER BY ord;
```

> [!NOTE]
> The query looks imposing, but you don't need to write anything like it yet —
> we're just "taking a census." `UNION ALL` glues several `SELECT`s into one list
> (one row per table), `'customers'::text` labels the row with the table's name
> (`::` is a type cast), and the `ord` column sets the output order. We'll cover
> `UNION` and working with row sets in module 08 — for now read this as a single
> "census" and move on.

`main.go` stays thin — connect, run two typed queries, print the result:

```go
pool, err := pg.NewPool(ctx)            // connection pool to the sandbox
queries := db.New(pool)                 // typed wrapper from sqlc
version, err := queries.ServerVersion(ctx)
world, err := queries.BrewWorld(ctx)    // census: table → rows
```

## Running it

Bring up the sandbox (from the repo root) and apply the Brew base schema:

```sh
docker compose up -d
make lecture L=00-getting-connected/00-01-meet-brew T=db-reset
```

Run the demo:

```sh
make lecture L=00-getting-connected/00-01-meet-brew
```

(`T=run` is the default, so without `T=...` the demo runs straight away.)

Output:

```
Сервер: PostgreSQL 18.4 on aarch64-unknown-linux-musl, compiled by gcc (Alpine 15.2.0) 15.2.0, 64-bit

Мир Brew — 9 таблиц канона. Что лежит в них после seed:

ТАБЛИЦА               СТРОК
customers             3
drinks                5
articles              2
orders                3
outbox                0
processed_outbox_ids  0
shops                 2
order_items           4
inventory             5

Итого 24 строки — на этих данных поедет весь курс.
```

(The demo prints in Russian.) Read the output back. First, the version starts with `PostgreSQL 18` — on the other end of the socket is exactly the version this course targets (the tail about architecture and gcc may differ on your machine; that depends on how the `postgres:18-alpine` image was built). Second, the canon has 9 tables, and some are still empty: `outbox` and `processed_outbox_ids` are waiting for the eventing module — we'll fill them ourselves. Third, there are exactly three orders, including Alice's order #1; five drinks on the menu and two shops — the Moscow one open for business and the St. Petersburg one waiting for launch — that's the world we'll spend the next ten modules in.

> [!NOTE]
> **Check yourself.** Without peeking at the output: how many of the 9 canon
> tables are empty right after the seed — and why those ones? How many rows total
> sit in the Brew world at the course's start?

> [!TIP]
> **Answer.** Two are empty — `outbox` and `processed_outbox_ids`: these are the
> order-eventing tables, filled by module 09, not by the seed. The other seven are
> populated, 24 rows in total — as in the output above ("Итого 24 строки"). It may
> look like an empty table means "unfinished schema," but no: both belong to the
> canon from the start — their data just arrives later in the course.

## The fence

- This course is for people who **write the application**, not administer the server. Replication, backups, tuning, `max_connections`, monitoring — the DBA's turf, and almost every unit will mark that boundary explicitly. You're on the other end of the socket, a client, not the server's operator.
- The sandbox is `docker compose` on your laptop. `sslmode=disable`, the password `brew`/`brew` in a plaintext connection string, one shared database — all of it is fine locally only. In production connections are encrypted, the password comes from a secrets manager, and environments live in separate databases.
- One `brew` database for the whole course is a teaching simplification. In a real project you never run `make db-reset` (a `TRUNCATE` under the hood) against live data.

## Takeaways

- Brew is a chain of coffee shops; you're its backend developer. The whole course is a growth arc — from a newcomer who drops the pool to an engineer who opens a `PUBLICATION` and hands the stream to `kafka-cookbook`.
- The data is shared across the whole course: menu, customers, orders, stock, outbox. The tangible anchor is Alice's order #1, and we'll keep coming back to it.
- We work against one sandbox; the demo output is deterministic and diffed byte for byte, so the numbers can be trusted.

Next up — the **00-02 "Client, server, and the sandbox"** unit: the first technical step. What's actually on the other end of the socket, how a server differs from a client — and how to bring up a local copy you're not afraid to break.
