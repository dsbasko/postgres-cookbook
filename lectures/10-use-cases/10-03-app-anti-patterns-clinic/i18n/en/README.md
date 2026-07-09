# 10-03 — App anti-patterns clinic

Brew's database is healthy: the indexes are in place, the schema is fine, the
hardware isn't strained. And yet the dashboard keeps flashing red — the menu endpoint
drags, email lookup takes seconds, the orders pager times out on page forty. The triage ticket
goes to Pavel — and he closes it in one line:

> **Pavel (on the ticket):** checked pg_stat_statements. queries are simple. database's
> healthy. fix the code.

And he's right — the database isn't broken, what's broken is **the way the application
talks to it**. This is a familiar picture: five identical smells that are easy to grow
in any service and nearly impossible to spot in a one-line synthetic test. Each one has
a cure, and the cure isn't taste — it's measurable: fewer round-trips, fewer columns, a
different plan. We'll walk five diseases in a row, with a remedy for each.

## 1. N+1 → batch

The service shows a list of customers and pulls each one's orders. The naive code fetches
the customers in one query, then in a loop asks for orders **one query per customer**:
`SELECT ... FROM orders WHERE customer_id = $1` — N times over. On our data that's
`1 + 3 = 4` round-trips to the database for a list of three customers. Each query is
cheap, but it's four trips across the network and back; with a thousand customers it
becomes a thousand-plus, and what drags isn't the database but the latency of the link
to it.

The cure is **one batch query** for everyone at once: `WHERE customer_id = ANY($1::text[])`.
One round-trip, the same answer — the same three orders. We count round-trips, not time:
time depends on hardware and network, while the number of calls to the database is a
structural fact of the code, and it's visible immediately.

```
N+1: the list, then one query per customer

  app ──①──► SELECT customers             ──► [c1, c2, c3]
  app ──②──► SELECT orders WHERE id = c1  ──► orders of c1
  app ──③──► SELECT orders WHERE id = c2  ──► orders of c2
  app ──④──► SELECT orders WHERE id = c3  ──► orders of c3
            1 + N round-trips to the DB (here 1 + 3 = 4)

batch: one query for everyone

  app ──①──► SELECT orders WHERE customer_id = ANY([c1,c2,c3])  ──► all orders
            1 round-trip, the same answer
```

This first disease isn't from the old legends about someone else's code. It's in your own
fresh PR: you fetch the customers in one list, then pull each one's orders one at a time in
a loop. Dmitry walks over with his mug, opens the diff — and instead of a dressing-down, he
counts out loud.

> **Dmitry:** The customer list — one query. And the orders?
>
> **You:** In a loop. One query per customer.
>
> **Dmitry:** So the trips are one more than the customers. And what did the database
> answer each? The same one batch would return. The extra trips aren't a logic bug —
> they're the query's shape. Gather it with one `= ANY`.

No dressing-down — just a count: you don't scold this disease, you measure it. You rebuild,
and the loop of trips collapses into one. The first disease is cured; the next lives in how
much extra you ask the database for at once.

## 2. SELECT * → explicit columns

The menu endpoint needs two fields — the drink's name and its price. But the code writes
`SELECT * FROM drinks` and gets **9 columns** instead of the two it needs (`name`,
`base_price`). Seven extra columns travel over the network on every row of every query —
and that's the smaller harm. The bigger one is a **brittle coupling to the schema**: add a
column to `drinks`, reorder the fields, and code that scans "everything" positionally or
hauls an extra payload silently breaks or starts shipping junk. We count the columns
directly — via the response's field descriptions: `9` against `2`.

`SELECT name, base_price` is a contract: exactly what the menu shows, not a byte more.

## 3. non-sargable → expression index

Email lookup. `WHERE email = '...'` rides the plain index `accounts_email_idx` — the plan
is an **Index Scan**, all good. But someone wraps the column in a function for
case-insensitivity: `WHERE lower(email) = '...'`. The index is built on `email`, not on
`lower(email)`, so the planner can't use it — the plan falls back to a **Seq Scan** over
all 50k rows. The predicate has become **non-sargable**: a function on the column blinds
the index.

The cure is to index **the same function** you wrap the column with:
`CREATE INDEX ... ON accounts_lab (lower(email))`. After it, the same `lower(email) =
'...'` rides an **Index Scan** again. We read the top node type straight out of `EXPLAIN`:
Index Scan → Seq Scan → Index Scan again.

## 4. deep OFFSET → keyset

Paging through orders with `LIMIT/OFFSET`. To serve page forty-one,
`ORDER BY id LIMIT 10 OFFSET 40000` makes the scanner read and discard the entire prefix —
**40010 rows for ten**. The deeper you page, the costlier the page: OFFSET doesn't "jump"
to the right spot, it honestly reads everything up to it.

Keyset paging remembers where it stopped and walks from the boundary:
`WHERE id > 40000 ORDER BY id LIMIT 10` reads exactly **10 rows** for the same page. We
measure this not in clock time but in the actual rows read by the leaf node, via `EXPLAIN
ANALYZE`: 40010 against 10.

## 5. huge IN → = ANY($1::bigint[])

You need to fetch a thousand rows by a list of ids. The code glues them right into the
query text: `id IN (1,2,...,1000)` — **a thousand literals** in the SQL string. Every such
query with a fresh set of ids is a new, unique text: it gets re-parsed and re-planned, the
plan cache bloats, and with large lists you hit the limit on the number of parameters.

`= ANY($1::bigint[])` is **one array parameter** for all thousand ids. The query text is
the same for any set, parsed and planned once. Both forms find the same **1000 rows** —
the difference is purely in the parameter shape, and it's structural.

## The five smells at a glance

| Smell | Naive code | Cure | What it proves |
|---|---|---|---|
| N+1 | loop of `WHERE customer_id = $1` × N | `= ANY($1)` (or `JOIN`/`LATERAL`) | round-trips: 4 → 1 |
| `SELECT *` | `SELECT *` — 9 columns | explicit field list | columns: 9 → 2 |
| non-sargable | `WHERE lower(email) = …` on an index over `email` | index on `(lower(email))` | plan: Seq Scan → Index Scan |
| deep OFFSET | `LIMIT 10 OFFSET 40000` | keyset `WHERE id > 40000` | rows read: 40010 → 10 |
| huge IN | `id IN (1,…,1000)` as literals | `= ANY($1::bigint[])` | one text, one parse-plan |

Each row is a disease, the naive code and the cure, and the **number** that proves it.
All five are cured from the application side, not the database.

## What our code shows

`cmd/demo/main.go` is the clinic itself: five functions, one per disease, each printing the
naive variant and its cure side by side. `setupLab` builds two lab tables of 50k rows each
via `generate_series` (deterministic, no `random`): `events_lab` with a dense `id` key —
for OFFSET/keyset and the huge IN; `accounts_lab` with an `email` — for non-sargable. N+1
runs on the **base** `customers`/`orders` from `schema/brew.sql`. Before measuring, the
demo does `SET max_parallel_workers_per_gather = 0` so the plans are reproducible.

Why **raw-pgx** (an escape-hatch with a `go.mod`, no sqlc): the lesson isn't about the text
of a single query, it's about **how the application talks to the database** — how many
round-trips it makes, what shape it passes parameters in, what plan it gets. That's Go
control flow plus reading `EXPLAIN`, not a single `query.sql`, so sqlc has no role here.

## Running it

```sh
docker compose up -d
make lecture L=10-use-cases/10-03-app-anti-patterns-clinic T=db-reset
make lecture L=10-use-cases/10-03-app-anti-patterns-clinic
```

`T=run` is the default and can be omitted. From inside the unit directory it's shorter:
`make db-reset`, then `make run`. This is a capstone, so the unit also has `make test` — it
runs the integration test that asserts exactly these numbers (4 round-trips, 9 vs 2
columns, Seq→Index, 40010 vs 10, 1000 rows).

```
1) N+1 → батч (round-trip'ы до базы)
   N+1:  3 клиентов → 4 запроса (1 список + 3 на заказы), заказов 3
   батч: те же данные → 1 запрос (= ANY), заказов 3

2) SELECT * → явные столбцы (сколько данных тянем)
   SELECT *:        вернул 9 столбцов
   SELECT name,...: вернул 2 столбца — ровно то, что показывает меню

3) non-sargable → expression index (план поиска по email)
   email = ...        → Index Scan (обычный индекс работает)
   lower(email) = ... → Seq Scan (функция слепила индекс)
   lower(email) = ... → Index Scan (после expression-индекса)

4) глубокий OFFSET → keyset (сколько строк реально прочитано)
   OFFSET 40000 LIMIT 10:        сканер прочитал 40010 строк ради 10
   WHERE id > 40000 LIMIT 10:    сканер прочитал 10 строк (та же страница)

5) огромный IN → = ANY($1::bigint[]) (форма параметров)
   IN (1,2,...,1000):     1000 литералов в тексте запроса, нашли 1000 строк
   = ANY($1::bigint[]):   1 параметр-массив на 1000 id, нашли 1000 строк
```

All five pairs read the same way: on the left, what the naive code does; on the right, what
the cure changes, plus the number that proves it. N+1 shrank from 4 queries to one;
`SELECT *` hauled 9 columns instead of 2; `lower(email)` dropped the plan into a Seq Scan,
and the expression index brought back the Index Scan; deep OFFSET read 40010 rows for ten,
keyset read exactly 10; a thousand literals in the text collapsed into a single array
parameter, and the answer stayed the same — 1000 rows.

## The fence

These are all **app-side smells**: the database is healthy, it's the code
that gets cured. Caveats for each cure:

- **Keyset is fast under a matching index, but at the cost of flexibility.** It can't jump
  to an arbitrary page ("show page 4000 right now") — it walks from the previous boundary.
  If the product needs numbered page navigation, keyset won't fit; its mechanics are covered
  in 03-02, and indexes plus reading `EXPLAIN` in module 06.
- **`= ANY` isn't the only N+1 cure.** The same list-with-orders is often more natural to
  assemble with one `JOIN` (module 04) or a `LATERAL` "top-N per customer" subquery (08-05)
  — the choice depends on exactly what you need to return.
- **`= ANY($1)` fixes more than plan-cache bloat.** A single array parameter also dodges the
  **parameter-count limit** a giant `IN` list runs into, and removes the re-parse-and-plan
  on every new set of ids.
- **Non-sargable is cured by exactly one rule.** Index **the same function** you wrap the
  column with (`lower(email)` in the query → `(lower(email))` in the index). If the query
  has `lower` but the index is on the raw `email`, it's useless; more on this in 06-03.
- **`SELECT *` breaks for more than the network.** Code tied to "all columns" fails or
  starts hauling junk when columns are **added or reordered**. An explicit column list is
  also insurance against schema changes.

## Takeaways

A healthy database is easy to make hurt from five directions — and all five are cured from
the application side, measurably, not by taste. **N+1** — a loop of queries instead of one
batch (`= ANY` or `JOIN`/`LATERAL`); count round-trips. **SELECT \*** — extra columns and a
brittle coupling to the schema; list the fields. **Non-sargable** — a function on the column
blinds the index; index the same function. **Deep OFFSET** — the scanner reads the whole
prefix; keyset reads only the page. **Huge IN** — a thousand literals in the text; one array
parameter `= ANY($1)`. The common denominator: look not at a single query but at how the
application talks to the database in aggregate — the number of calls, the parameter shape,
the plan.

Next — the last capstone 10-04: **connection pooling from the app**. We'll talk about why
opening a connection per request is expensive, how `pgxpool` holds and reuses connections,
and where a service quietly exhausts its pool.
