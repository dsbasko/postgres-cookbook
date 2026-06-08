# 08-05 — LATERAL joins: top-N per group and the N+1 killer

The customer profile in the Brew app renders a "3 biggest orders" block. The
backend first pulled the list of customers in one query, and then, in a loop,
fired one more query per customer to fetch that customer's three orders. A
thousand customers on the "top buyers" screen meant a thousand and one trips to
Postgres. This pattern is called `N+1`, and in production it looked like this:
the profile page loaded in a second, a dashboard with a hundred customers took
almost a minute, the connection pool choked, and half the latency was spent
purely on network round-trips between the service and the database. We wanted
something else: a single query that, inside the database itself, computes the
top-3 for every customer at once.

Doing this head-on runs into one SQL rule. And it is exactly that rule that
`LATERAL` lifts.

## Why a plain subquery in FROM won't do

A subquery in `FROM` is computed **independently**. It cannot see the sibling
tables from the same `FROM` — formally it is a separate "derived set of rows",
and Postgres must be able to compute it on its own, before any join. So you
cannot write "take the three orders of *this* customer" right inside `FROM`: the
name `c.id` simply does not exist there, the subquery knows nothing about it.

`LATERAL` lifts this ban. The subquery to the right of `JOIN LATERAL` gains the
right to reference columns of the tables **to its left**. Thinking procedurally,
it is literally the body of a loop "for each row on the left": Postgres walks the
customers and, for each one, runs the right-hand subquery with the current `c.id`
substituted in. The very idea that N+1 suffered from in the application moves
inside the database — and becomes a single query plan instead of a thousand
round-trips.

## Top-N per group in one query

Once the subquery is allowed to see the left row, top-3 per customer is
straightforward: for each `c` take that customer's orders
(`WHERE o.customer_id = c.id`), sort by amount descending and keep `LIMIT 3`.
Change the number in `LIMIT` — you get top-5 or top-1; the shape of the query
stays the same.

This is also where the generalization of `DISTINCT ON` from 04-04 hides. There
we took "one row per group" — essentially top-1. `LATERAL` is the same trick, of
which top-1 is just the special case `LIMIT 1`: `LIMIT 1` gives "the best/latest
per group", `LIMIT 3` gives top-3, and switching between them costs exactly one
digit. Where `DISTINCT ON` hits a ceiling (only one row), `LATERAL` calmly hands
back as many as you need.

## LEFT vs CROSS: what to do with order-less customers

Our data has Karina — a customer without a single order. For her the right-hand
subquery returns nothing. And here the flavor of the join matters. `CROSS JOIN
LATERAL` requires a match — if the right side is empty, the left row **drops
out**, and Karina disappears from the profiles. `LEFT JOIN LATERAL (...) ON true`
behaves like a normal `LEFT JOIN`: the left row is kept, and the columns of the
right-hand subquery become `NULL`. The `ON true` condition here is a formality:
all the selection logic already lives inside the subquery (in its `WHERE`), there
is nothing for the join to match on, so it stitches "as is".

`NULL` on the Go side is awkward for a clean `string`, so the SQL carries
`coalesce(..., '—')`: Karina shows up in the output with dashes instead of
vanishing.

The same "for each left row" loop, but in two places — outside over the network and inside the database:

```
N+1 in the app (over the network):       LATERAL (one plan inside the database):
  1 query → list of customers              FROM customers c
  then one more query per CUSTOMER:        LEFT JOIN LATERAL (top-3 ... c.id) ON true
    Alisa  → SELECT top-3 (Alisa)  ┐         for each left row c:
    Boris  → SELECT top-3 (Boris)  ├─ ×1000    Alisa  → 520, 450, 300
    Karina → SELECT top-3 (Karina) ┘           Boris  → 480, 250
  = 1000+1 round-trips                         Karina → (empty) → '—'
                                             = 1 round-trip, 1 pass
```

The "body of a loop for each row on the left" idea is the same; the difference is where it spins. Which `JOIN LATERAL` to use, and how it differs from its neighbours:

| Approach | Rows per group | Order-less Karina | When to use |
|---|---|---|---|
| N+1 in the app | as many as you like | depends on the code | never (a thousand round-trips) |
| `DISTINCT ON` (04-04) | exactly 1 | kept (via `LEFT JOIN`) | pure top-1 |
| `CROSS JOIN LATERAL` | top-N (`LIMIT n`) | **dropped** (no match) | top-N, order-less not needed |
| `LEFT JOIN LATERAL … ON true` | top-N (`LIMIT n`) | **kept** (`NULL` → `'—'`) | top-N and keep everyone |

## What our code shows

The heart of the lesson is `query.sql`. The `TopOrdersPerCustomer` query takes
all customers on the left and, for each, runs the right-hand subquery that sees
`c.id`:

```sql
SELECT
    c.name,
    coalesce(t.rn::text, '—')    AS rn,
    coalesce(t.cents::text, '—') AS cents,
    coalesce(t.day::text, '—')   AS day
FROM lat_customers_lab c
LEFT JOIN LATERAL (
    SELECT row_number() OVER (ORDER BY o.cents DESC, o.id) AS rn, o.cents, o.day
    FROM lat_orders_lab o
    WHERE o.customer_id = c.id
    ORDER BY o.cents DESC, o.id
    LIMIT 3
) t ON true
ORDER BY c.id, t.rn;
```

The reference `WHERE o.customer_id = c.id` is exactly what makes this LATERAL:
the subquery `t` reaches into a column of the left table. `LIMIT 3` cuts the
result down to top-3 per customer, `LEFT ... ON true` keeps Karina, and
`coalesce` turns her `NULL` into `'—'`. The second query,
`BiggestOrderPerCustomer`, is the same skeleton with `LIMIT 1`: the biggest order
per customer, precisely the `DISTINCT ON` case.

`cmd/demo/main.go` is thin: `pgxpool` → `db.New` → two typed calls →
`tabwriter`. There is no selection logic in Go, it all lives in the SQL.

## Running it

```sh
docker compose up -d
make lecture L=08-analytical-and-lateral/08-05-lateral-joins T=db-reset
make lecture L=08-analytical-and-lateral/08-05-lateral-joins
```

`T=run` is the default mode and can be omitted. From inside the unit directory
the same steps are shorter: `make db-reset`, then `make run`.

(The demo prints in Russian.)

```
1) Top-3 заказа на клиента (LEFT JOIN LATERAL, один запрос):
КЛИЕНТ  #  сумма  день
Алиса   1  520    2025-03-03
Алиса   2  450    2025-03-02
Алиса   3  300    2025-03-01
Борис   1  480    2025-03-02
Борис   2  250    2025-03-01
Карина  —  —      —
   → Карина без заказов сохранена ('—'); у Алисы 4 заказа, top-3 отсёк самый дешёвый (280).

2) Самый крупный заказ на клиента (LATERAL c LIMIT 1):
КЛИЕНТ  сумма  день
Алиса   520    2025-03-03
Борис   480    2025-03-02
Карина  —      —
```

Alisa has four orders, and `LIMIT 3` honestly cut off the cheapest one (280).
Boris, with his two orders, fit in entirely. Karina, with no orders, stayed in
both lists with dashes — that is the work of `LEFT JOIN LATERAL ... ON true`.

## The fence

- Without an index on the correlation condition (`o.customer_id`), `LATERAL` becomes N full scans of the orders table instead of one — the same N+1 trap, except now it has hidden inside the database and is invisible from the outside.
- For a pure top-1, `DISTINCT ON` from 04-04 is often shorter and clearer. `LATERAL` earns its keep precisely when you need more than one row per group.
- Mind the flavor of the join: `CROSS JOIN LATERAL` drops rows without a match — to "keep everyone" you need `LEFT JOIN LATERAL ... ON true`.
- Don't confuse `LATERAL` with a correlated subquery in `SELECT`: the latter must return exactly one row or a scalar, whereas `LATERAL` calmly returns **many** rows per left record.
- The plan and cost of such a query are already module 06; the right index for the correlation in production is something your DBA will pick.

## What to take away

`LATERAL` is a subquery in `FROM` that has been allowed to see the left row —
that is, "the body of a loop for each row on the left" expressed in SQL. It
solves top-N per group in a single query and thereby kills the application's N+1:
a thousand round-trips collapse into one plan. `LEFT JOIN LATERAL (...) ON true`
keeps order-less customers (Karina) — `CROSS JOIN LATERAL` would have dropped
them. At its core this is a generalization of `DISTINCT ON` from 04-04: `LIMIT 1`
is "the best/latest", `LIMIT 3` is top-3, the difference is a single digit. And
do not forget the index for the correlation condition, or the N+1 simply moves
inside the database.

We learned to unfold each row into its own set of rows. Next we need the opposite
motion — to fold rows back into totals, and across several breakdowns at once in
a single pass. In 08-06 that is the job of `grouping sets`, `rollup` and `cube`:
one aggregation computing sums by day, by customer and the grand total all at the
same time.
