# 04-06 — CTEs and materialization

Brew reports rarely fit into one flat `SELECT`. "How much each customer spent" is: first compute each order's total from its line items, then collapse orders per customer, then substitute the name. You can cram all that into nested subqueries, but reading such a query is like unpacking a matryoshka from the inside out.

A `CTE` (Common Table Expression, the `WITH` clause) flips this: it gives each intermediate step a name, and the query reads top to bottom, step by step. And along the way we'll unpack materialization — whether Postgres computes a `CTE` separately (and caches the result) or inlines it into the main query.

## CTE: named building blocks

`WITH name AS (query)` declares a temporary result that you then reference by name, like a table. `CTE`s can be chained: the second references the first, the main query references both. This turns "a subquery inside a subquery" into a linear pipeline:

```sql
WITH order_totals AS (        -- step 1: each order's total from line items
    SELECT o.id AS order_id, o.customer_id,
           sum(oi.quantity * oi.unit_price)::bigint AS cents
    FROM orders o JOIN order_items oi ON oi.order_id = o.id
    GROUP BY o.id, o.customer_id
),
per_customer AS (             -- step 2: collapse orders per customer
    SELECT customer_id, count(*) AS orders, sum(cents)::bigint AS spent
    FROM order_totals GROUP BY customer_id
)
SELECT c.name, p.orders, p.spent          -- step 3: substitute the name
FROM per_customer p JOIN customers c ON c.id::text = p.customer_id
ORDER BY p.spent DESC;
```

Each step is a separate named block, and the result flows top to bottom into the next:

```
WITH order_totals AS (...)    step 1 · each order's total from line items
         │
         ▼
     per_customer AS (...)    step 2 · collapse orders per customer
         │
         ▼
   SELECT … JOIN customers    step 3 · substitute the customer name
```

The main value of a `CTE` for an application is readability and reuse of an intermediate result — not "speedup." A `CTE` on its own doesn't make a query faster.

## Materialization: a fence vs inlining

Here's the subtlety. Postgres can treat a `CTE` in two ways:

- **Inline** — substitute the `CTE`'s body into the main query, like an ordinary subquery. Then the planner sees the whole query and can, for example, push a filter into the `CTE`.
- **Materialize (fence)** — compute the `CTE` separately, once, store the result in a temporary buffer, and read from it afterwards. The optimizer doesn't peek behind that "fence."

Since PG12 the default rule is: if a `CTE` is referenced **once** — it's inlined; if **more than once** (or it contains a write/`VOLATILE` function) — it's materialized (logically: computing once and reusing is cheaper than twice). These defaults can be overridden with keywords: `AS MATERIALIZED` forces the fence, `AS NOT MATERIALIZED` forces inlining.

|   | inline | materialize (fence) |
|---|---|---|
| what it does | the `CTE`'s body is substituted into the main query | the `CTE` is computed separately, once, into a buffer |
| optimizer | sees the whole query, pushes filters in | doesn't peek behind the "fence" |
| default (PG12+) | the `CTE` is referenced once | referenced more than once (or a write/`VOLATILE`) |
| force | `AS NOT MATERIALIZED` | `AS MATERIALIZED` |

Our second query references `order_totals` twice — in `FROM` and in a scalar subquery for the grand total — so it's materialized (we wrote `AS MATERIALIZED` explicitly, but even without the keyword the default would be the same).

## What our code shows

Two queries in `query.sql`. The first is a two-`CTE` pipeline (`CustomerSpend`, above). The second is a `CTE` used twice:

```sql
-- name: OrderShareOfTotal :many
WITH order_totals AS MATERIALIZED (
    SELECT o.id AS order_id, sum(oi.quantity * oi.unit_price)::bigint AS cents
    FROM orders o JOIN order_items oi ON oi.order_id = o.id GROUP BY o.id
)
SELECT order_id, cents,
       round(100.0 * cents / (SELECT sum(cents) FROM order_totals), 1)::text AS pct
FROM order_totals ORDER BY order_id;
```

`order_totals` is read in `FROM` and in `(SELECT sum(cents) FROM order_totals)` — hence each order's share of the grand total.

## Running it

Bring up the sandbox (from the repo root) and apply the canon:

```sh
docker compose up -d
make lecture L=04-querying-across-tables/04-06-ctes-and-materialization T=db-reset
make lecture L=04-querying-across-tables/04-06-ctes-and-materialization
```

(`T=run` is the default. From inside the unit directory it's `make db-reset`, `make run`.)

Output:

```
1) Траты клиента — CTE-конвейер (order_totals → per_customer → имя):
   клиент           заказов потрачено
   Алиса Иванова          2     19.30
   Борис Петров           1      3.00
   → суммы посчитаны из позиций order_items; Карины нет — у неё заказов нет.

2) Доля заказа от общего — CTE order_totals использован дважды (FROM + scalar-подзапрос):
   заказ      сумма  доля,%
   #1          9.70    43.5
   #2          3.00    13.5
   #3          9.60    43.0
   → ссылок на CTE две → Postgres материализует его (считает один раз, переиспользует).
```

(The demo prints in Russian.) The first query read as three steps and collapsed spend per customer (totals come from `order_items`, so Alice = 9.70 + 9.60 = 19.30). The second reused one `CTE` twice and computed each order's share of the grand total (9.70 + 3.00 + 9.60 = 22.30 → 43.5% + 13.5% + 43.0%).

## The fence

What we simplified.

- The difference between inlining and materialization is visible not in the result (it's the same) but in the **plan**: a materialized `CTE` puts up a "fence" the optimizer won't push filters through, and on large data a stray `AS MATERIALIZED` sometimes hurts (the server computes the whole `CTE` even though only a couple of rows are needed outside). You can see this only via `EXPLAIN` — that's module 06, so here we only name the levers without measuring them.
- A `CTE` is about readability, not speed. The belief "I'll rewrite the subquery into a `WITH` and it'll be faster" is a myth (before PG12 materialization was always on and sometimes even hurt).
- A recursive `CTE` (`WITH RECURSIVE`) is a separate big topic: traversing trees and graphs; unit 08-04 is devoted to it.

## Takeaways

- A `CTE` (`WITH name AS (...)`) gives an intermediate result a name; the query turns from a "subquery matryoshka" into a linear pipeline of steps.
- `CTE`s can be chained: the next references the previous — that's the main value for readability.
- A `CTE` on its own doesn't speed up a query; it's about code structure, not performance.
- Since PG12: one reference → the `CTE` is inlined; more than one (or a write inside) → it's materialized. The levers are `AS MATERIALIZED` / `AS NOT MATERIALIZED`.
- The inline/materialize difference shows up in the plan (`EXPLAIN`, module 06), not in the result.

That completes module 04, "Querying across tables": you can connect tables (`JOIN`/self-join), collapse rows into summaries (`GROUP BY`/`HAVING`, `DISTINCT ON`), ask questions with questions (subqueries, `EXISTS` vs `IN`), and assemble readable pipelines (`CTE`). Next up — module **05 "Transactions, MVCC, and concurrency"**: what happens when several sessions reach for this data at once.
