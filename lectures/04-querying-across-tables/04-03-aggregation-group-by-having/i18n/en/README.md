# 04-03 — Aggregation, GROUP BY / HAVING

The business rarely asks "show me all the rows." It asks in summaries: "how many drinks in each category and at what price?", "how many orders does each customer have and for how much?", "who has ordered at least twice?". The answer to such questions is aggregation: collapse many rows into one summary row per group.

And this is exactly where one of the costliest reporting mistakes lives: `count(*)` and `count(column)` look almost identical but count **different things**. On a customer with no orders the discrepancy shows immediately — and if you mix them up, a "customer activity" report quietly lies.

## GROUP BY and aggregate functions

`GROUP BY` slices the table into groups by a column's value (or several), and an aggregate function computes one number per group: `count` — how many, `sum` — the total, `min`/`max` — the bounds, `avg` — the average. The rule: everything in `SELECT` that isn't an aggregate must appear in `GROUP BY` — otherwise it's unclear which of the group's values to show. So `SELECT category, count(*) ... GROUP BY category` is correct, while `SELECT name, count(*) ... GROUP BY category` is not (`name` is many within a group).

We round the average price and cast it to `bigint` (`round(avg(base_price))::bigint`): `avg` returns `numeric`, but we want a whole number of cents and an `int64` in Go.

## count(\*) vs count(column) — not the same thing

This is the heart of the unit. The two `count` forms count **different things**:

- `count(*)` — how many **rows** are in the group, regardless of their contents.
- `count(column)` — how many rows where **that column is not NULL**.

On `customers LEFT JOIN orders` the difference surfaces on a customer with no orders. For Karina the `LEFT JOIN` leaves one row with `NULL` in the order columns. Then `count(*)` for her = 1 (the row exists), while `count(o.id)` = 0 (no orders, `o.id` is `NULL`). If a "how many orders does the customer have" report uses `count(*)`, Karina gets "1 order" — though she has zero. The correct order counter here is `count(o.id)`.

`sum(o.amount)` over a group with no orders returns `NULL` (not 0!) — so we wrap it in `COALESCE(..., 0)`, or Karina would have an empty revenue instead of zero.

All four forms on the very same Karina row (`LEFT JOIN`, no orders) give different results — and each difference is easy to mistake for a data bug, though it's behavior by definition:

| on Karina's row | gives | why |
|---|---|---|
| `count(*)` | `1` | counts rows; the `LEFT JOIN` left one row with `NULL` |
| `count(o.id)` | `0` | counts only non-`NULL`; `o.id` is empty |
| `sum(o.amount)` | `NULL` → `0` via `COALESCE` | no addends — that's `NULL`, not `0` |
| `avg(o.amount)` | `NULL` | an empty group has nothing to average |

## HAVING filters groups, not rows

`WHERE` removes rows **before** grouping; `HAVING` removes **finished groups** — by an aggregate's value. "Customers with two or more orders" can't be written as `WHERE count(o.id) >= 2`: at the `WHERE` stage the aggregate isn't computed yet. `HAVING count(o.id) >= 2` does it — it applies after `GROUP BY`, when each group's count is already known.

The query's steps run in a strict logical order, and where `WHERE` sits versus `HAVING` explains everything:

```
FROM / JOIN   →  collect rows from the tables
WHERE         →  drop rows                 (ROW filter, before grouping)
GROUP BY      →  slice into groups
aggregates    →  count / sum / min / max / avg per group
HAVING        →  drop finished groups      (GROUP filter, by an aggregate)
ORDER BY      →  order the result
```

`WHERE` still sees individual rows, `HAVING` sees already-computed groups; that's why `count(o.id) >= 2` lives only in `HAVING`.

## What our code shows

Three queries in `query.sql`. The menu summary:

```sql
-- name: MenuStatsByCategory :many
SELECT category, count(*) AS drinks, min(base_price)::bigint AS price_min,
       max(base_price)::bigint AS price_max, round(avg(base_price))::bigint AS price_avg
FROM drinks GROUP BY category ORDER BY category;
```

And per-customer stats with the two counters side by side — so the discrepancy is visible:

```sql
-- name: CustomerOrderStats:  count(*) AS rows_in_group,  count(o.id) AS orders, ...
--   ... FROM customers c LEFT JOIN orders o ON o.customer_id = c.id::text GROUP BY c.id, c.name;
-- name: RegularCustomers:    ... HAVING count(o.id) >= 2;
```

## Running it

Bring up the sandbox (from the repo root) and apply the canon:

```sh
docker compose up -d
make lecture L=04-querying-across-tables/04-03-aggregation-group-by-having T=db-reset
make lecture L=04-querying-across-tables/04-03-aggregation-group-by-having
```

(`T=run` is the default. From inside the unit directory it's `make db-reset`, `make run`.)

Output:

```
1) Сводка меню по категориям (GROUP BY category):
   катег.   напитк      min      max      avg
   coffee        3     3.00     4.80     4.10
   cold          1     5.20     5.20     5.20
   tea           1     2.50     2.50     2.50

2) Статистика по клиентам (customers LEFT JOIN orders, GROUP BY клиент):
   клиент            count(*) count(id)   выручка
   Алиса Иванова            2         2     20.10
   Борис Петров             1         1      3.00
   Карина Сидорова          1         0      0.00
   → у Карины count(*)=1 (строка есть), но count(o.id)=0 (заказов нет):
     count(*) считает строки, count(колонка) — только не-NULL значения.

3) Постоянные клиенты — HAVING count(o.id) >= 2:
   Алиса Иванова    заказов: 2, выручка: 20.10
   → HAVING фильтрует уже посчитанные группы; WHERE так не умеет.
```

(The demo prints in Russian.) Karina is the vivid case: `count(*)` and `count(o.id)` diverge precisely because the `LEFT JOIN` gave her a row with no order. `HAVING` left the single customer with two orders — Alice.

## The fence

What we simplified.

- `count(*)` and `count(column)` aren't "style" but different questions: "how many rows" vs "how many non-empty values." Reports confuse them most often, and the bug is silent — the numbers look plausible.
- We rounded the `numeric` average to whole cents deliberately. In production an "average ticket" to the hundredth of a kopeck is usually pointless, but you must round explicitly, not rely on display.
- We computed revenue from `orders.amount` (the order's recorded header total), not by recomputing from the `order_items` lines — that's a different source and, in general, a different total (the header legitimately drifts from the sum of lines). In a real report it matters to pin down what exactly counts as revenue, or two "correct" figures won't agree.
- On large tables the grouping itself wants suitable indexes and sometimes hits memory limits sorting groups — but that's plan territory (module 06).

## Takeaways

- `GROUP BY` slices a table into groups; an aggregate (`count`/`sum`/`min`/`max`/`avg`) computes one number per group.
- Everything in `SELECT` that isn't an aggregate must be in `GROUP BY`.
- `count(*)` counts rows; `count(column)` counts only rows with a non-NULL value. On a `LEFT JOIN` they're different numbers.
- `sum`/`avg` over an empty group give `NULL`, not 0 — wrap in `COALESCE` if you need zero.
- `WHERE` filters rows before grouping, `HAVING` filters finished groups by an aggregate's value.

Aggregates collapsed each group into one number — how many, for how much, on average. But the business often needs not a figure but a specific row from the group: not "how many orders Alice has" but her **latest** order in full — date, amount, status. Fetching exactly one row per group with one concise technique that's specific to Postgres is the **04-04 "DISTINCT ON"** unit.
