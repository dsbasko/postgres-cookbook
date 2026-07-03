# 04-04 — DISTINCT ON

A frequent request in Brew: "show the latest order of each customer." Not the date of the latest order — the whole order: its number, amount, status. Via `GROUP BY` that's awkward: `max(created_at)` returns only the date, and to pull the other columns of that exact row you have to make a second pass (join the result back to `orders` on customer and date). Clunky, and easy to get wrong on equal dates.

Postgres offers a shortcut — `DISTINCT ON`. It isn't standard SQL (a Postgres-specific feature), but it solves "one row per group, and the whole row" in a single expression.

> [!NOTE]
> Worth recalling from earlier lessons: the tie-break on a unique column (`id`) from 03-02 — there it gave a stable order for pagination; here the same idea decides which row of a group to keep when sort keys are equal. Also — `JOIN` from 04-01 and prices as cents-`BIGINT` from 01-01.

## How DISTINCT ON works

First, recall plain `DISTINCT`. It removes **fully duplicate** rows: it keeps only the rows that are unique as a whole. "Which categories are on the menu?" is exactly that:

```sql
SELECT DISTINCT category FROM drinks;
```

`drinks` has many entries, but few categories (`coffee`, `tea`, …) — `DISTINCT` collapses the repeats and returns each category once. The key point: it looks at the **whole** selected row (here, the single `category` field) and keeps the unique values.

`DISTINCT ON` works differently. `SELECT DISTINCT ON (expression) ...` keeps the **first** row for each unique value of `expression` — but returns the WHOLE row, not just that expression. And what counts as "first" is set by `ORDER BY`. Hence the iron rule: `ORDER BY` must **begin** with the same expression as `DISTINCT ON`, and then comes the criterion "which row of the group to pick."

"The latest order per customer" reads like this:

```sql
SELECT DISTINCT ON (o.customer_id) ...
FROM orders o
ORDER BY o.customer_id, o.created_at DESC, o.id DESC;
```

`DISTINCT ON (o.customer_id)` — one row per customer. `ORDER BY o.customer_id` (the mandatory start) groups a customer's rows together, and `created_at DESC` puts the freshest order first in the group — that's the one `DISTINCT ON` keeps. `id DESC` is the tie-break in case of equal `created_at` (two rows with the same date — otherwise "first" is undefined).

First `ORDER BY` lays the rows out, then `DISTINCT ON` walks top to bottom and takes the first in each group, skipping the rest of that group:

```
orders sorted by ORDER BY (o.customer_id, o.created_at DESC, o.id DESC):

  customer_id   created_at    id      DISTINCT ON (o.customer_id)
  ──────────────────────────────     ───────────────────────────
  1 · Alice     2025-01-16   #3  ◀──  first for customer 1 → keep
  1 · Alice     2025-01-15   #1       customer 1 already taken → skip
  2 · Boris     2025-01-15   #2  ◀──  first for customer 2 → keep
```

By changing the `ORDER BY` tail to `amount DESC`, the same construct gives the most **expensive** order per customer. The selection criterion lives in `ORDER BY` — that's what makes `DISTINCT ON` flexible.

## DISTINCT ON vs the alternatives

The same task is solved by other tools too, and it's useful to know when to use which:

- **`GROUP BY` + `max()`** gives an aggregate (date/amount) but not the whole row. To return the entire order you need a repeated join — extra and fragile.
- **Window functions** (`ROW_NUMBER() OVER (PARTITION BY customer_id ORDER BY created_at DESC)` plus a `= 1` filter) — the standard, portable way; it also does "top-3 per customer," not just top-1. That's unit 08-02.
- **`DISTINCT ON`** — the shortest when you need exactly **one** row per group and the project is already on Postgres.

| technique | returns | rows per group | portability |
|---|---|---|---|
| `GROUP BY` + `max()` | an aggregate (date/amount), not the row — other columns need a repeated join | — | standard SQL |
| `DISTINCT ON (k)` | the whole row, one per `k` | exactly one | Postgres only |
| `ROW_NUMBER() … = 1` | the whole row; a `<= N` condition gives top-N | one or N | standard SQL (08-02) |

`DISTINCT ON` ≠ `DISTINCT`: plain `DISTINCT` removes fully duplicate rows, `DISTINCT ON (col)` keeps one row per value of `col`.

## What our code shows

Two queries in `query.sql` — the same construct, a different `ORDER BY` tail:

```sql
-- name: LatestOrderPerCustomer :many
SELECT DISTINCT ON (o.customer_id) c.name, o.id, o.amount::numeric(10,2)::text, o.status, o.created_at::date::text
FROM orders o JOIN customers c ON c.id::text = o.customer_id
ORDER BY o.customer_id, o.created_at DESC, o.id DESC;
-- PriciestOrderPerCustomer: ... ORDER BY o.customer_id, o.amount DESC, o.id DESC;
```

We cast `amount` (NUMERIC) to `numeric(10,2)::text` for a stable "X.XX," and the date to `::date::text`. Both columns arrive in Go as `string` — no fuss with `pgtype.Numeric`/`pgtype.Date`.

## Running it

Bring up the sandbox (from the repo root) and apply the Brew base schema:

```sh
docker compose up -d
make lecture L=04-querying-across-tables/04-04-distinct-on T=db-reset
make lecture L=04-querying-across-tables/04-04-distinct-on
```

(`T=run` is the default. From inside the unit directory it's `make db-reset`, `make run`.)

Output:

```
1) Последний заказ на клиента — DISTINCT ON (customer_id), свежесть по created_at:
   клиент            заказ    сумма статус    дата
   Алиса Иванова    #3         9.60 shipped   2025-01-16
   Борис Петров     #2         3.00 created   2025-01-15
   → у Алисы два заказа (#1 и #3), DISTINCT ON оставил один свежий — #3.
     Карины нет: у неё заказов нет, а выбираем мы из orders.

2) Самый дорогой заказ на клиента — тот же DISTINCT ON, но хвост ORDER BY = amount DESC:
   клиент            заказ    сумма
   Алиса Иванова    #1        10.50
   Борис Петров     #2         3.00
   → у Алисы теперь #1 (10.50 > 9.60): сменили критерий — сменился победитель группы.
```

(The demo prints in Russian.) Alice has two orders; the first query kept the freshest (#3), the second the most expensive (#1). Only the `ORDER BY` tail changed — and the group's "winner" changed with it. Karina isn't in the result: we select from `orders`, and she has none (this isn't a `LEFT JOIN`).

> [!NOTE]
> **Check yourself.** Drop the trailing tie-break `id DESC` from `LatestOrderPerCustomer`, leaving `ORDER BY o.customer_id, o.created_at DESC`. Is the output the same on the current data? And what stops being guaranteed?

> [!TIP]
> **Answer.** On our seed the output doesn't change: Alice's two orders have **different** `created_at` (2025-01-15 and 2025-01-16), so `created_at DESC` alone already pins the "first" row of the group — #3 wins. But the guarantee is gone. The moment a group holds two rows with an **equal** leading sort key (here, the same `created_at`), `DISTINCT ON` picks the "first" of them **arbitrarily** — the order of such rows is undefined, and the output stops being stable between runs. `id DESC` is exactly that insurance from 03-02: it completes the order into a total one, so the winner is predictable always, not only while the data happens to have no ties.

## The fence

What we simplified.

- `DISTINCT ON` depends on the whole `ORDER BY`. Without an explicit tie-break (`id DESC`), with two orders sharing the same `created_at`, the "first" row is undefined — the result would "float" between runs. In production that's a source of unstable reports, so a tie-break on a unique column is mandatory.
- `DISTINCT ON` is non-standard: porting to another DBMS means rewriting it to window functions — portable code sometimes reaches straight for `ROW_NUMBER()` (08-02).
- On large tables `DISTINCT ON`'s efficiency hinges on an index for the `ORDER BY` — without one the server sorts the whole set (a plan question — module 06).
- If you need the "last N" rows per group rather than the last one, `DISTINCT ON` no longer fits — that's window functions again.

## Takeaways

- `DISTINCT ON (expression)` keeps one row per value of `expression` — and returns the WHOLE row, not just an aggregate.
- `ORDER BY` must begin with the `DISTINCT ON` expression; then comes the criterion "which row of the group to keep."
- Changing the `ORDER BY` tail picks a different row of the group (latest order ↔ most expensive) — the construct is flexible.
- `DISTINCT ON` ≠ `DISTINCT`: the former is one row per key, the latter removes fully duplicate rows.
- Need portability or "top-N per group" → window functions (08-02); `DISTINCT ON` is about "exactly one per group" in Postgres.

`DISTINCT ON` and aggregates answered "which row" and "how many." But often the question is about the **existence of a link**: "which drinks have never been ordered?", "which customers have already bought something?". Those are subqueries — and there that same `NOT IN`-with-a-`NULL`-in-the-list trap waits (the teaser from 01-02, unpacked in 03-06): it silently returns empty. Next up — the **04-05 "Subqueries: EXISTS vs IN"** unit, where that trap becomes the main argument for `EXISTS`.
