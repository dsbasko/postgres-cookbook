# 04-01 — JOIN: inner / left / right / full

In Brew the data lives apart: customers in `customers`, orders in `orders`. Each on its own is useless for the business question "which customers ordered, and how much." To answer it you must **connect** the tables on a shared key — that's a `JOIN`.

The most common beginner trap: use a plain (`INNER`) `JOIN` for a "all customers and their orders" report — and not notice that customers who haven't ordered yet silently dropped out. Brew has exactly such a customer — Karina: she signed up but hasn't placed an order. Which `JOIN` makes her disappear and which keeps her (with an empty order) is exactly what this unit is about.

An important canon detail: `customers.id` is `BIGINT`, while `orders.customer_id` is `TEXT` (by design: in the real CDC stream orders and the customer directory travel independently). So we connect them with an explicit cast `c.id::text = o.customer_id` — that's fine and irrelevant to our `JOIN` discussion.

## INNER JOIN — matches only

`INNER JOIN` (or just `JOIN`) keeps rows that have a pair **on both sides**. A customer with no orders and an order with no customer don't make it into the result. It's the right choice when you want exactly the matched pairs — "orders together with customer data." But for an "all customers" report it's treacherous: it silently drops those who haven't ordered yet.

## LEFT JOIN — all of the left, the right if present

`LEFT JOIN` keeps **all** rows of the left table and fills in a match from the right, or `NULL` if there's no pair. `customers LEFT JOIN orders` is "all customers, and their orders if any." Karina stays in the result with `order_id = NULL` and `status = NULL`. This is the most common `JOIN` in applications: "show the entity and its related data without losing entities that have no relations."

`LEFT JOIN` + an `IS NULL` check on the right side is the standard "find rows without a pair" technique (an anti-join): `... LEFT JOIN orders o ON ... WHERE o.id IS NULL` returns customers with no orders at all.

## RIGHT JOIN — the same, mirrored

`RIGHT JOIN` is `LEFT JOIN` flipped: it keeps all rows of the **right** table. `orders RIGHT JOIN customers` gives exactly the same as `customers LEFT JOIN orders`: all customers, orders if any. That's why `RIGHT` is almost never written in code: any `RIGHT` turns into a `LEFT` by swapping the tables, and `LEFT` reads left-to-right more naturally. You need to know it to read others' SQL, but in your own you almost always pick `LEFT`.

## FULL JOIN — mismatches on both sides

`FULL JOIN` keeps unmatched rows **on both sides at once**: left rows with no pair and right rows with no pair. You can't show it honestly on the Brew canon — every order references an existing customer, so there are no "orphan" rows on the right, and `FULL` would degenerate into `LEFT`. So we use a lab example — reconciling two stock-count sheets: the floor counted drinks `{1, 2}`, storage counted `{2, 4}`. A `FULL JOIN` on `drink_id` shows everything: a drink only on the floor (`storage = NULL`), only in storage (`floor = NULL`), and counted in both. It's the classic "merge two sources and highlight the discrepancies" task.

## What our code shows

Four queries in `query.sql`. The first three differ by exactly one word (`JOIN` / `LEFT JOIN` / `RIGHT JOIN`) on the same pair of tables:

```sql
-- name: LeftCustomersOrders :many
SELECT c.name AS customer, o.id AS order_id, o.status
FROM customers c
LEFT JOIN orders o ON o.customer_id = c.id::text
ORDER BY c.id, o.id;
```

sqlc sees that after a `LEFT JOIN` the `orders` columns can be `NULL` and types them as `pgtype.Int8` / `pgtype.Text` (whereas in the `INNER` variant the same columns are plain `int64` / `string`: a match is guaranteed). The fourth query is the `FULL JOIN` of the count sheets; the drink name comes from `drinks` via `COALESCE(f.drink_id, s.drink_id)` (the key exists on at least one side).

## Running it

Bring up the sandbox (from the repo root) and apply the canon:

```sh
docker compose up -d
make lecture L=04-querying-across-tables/04-01-joins-inner-left-right-full T=db-reset
make lecture L=04-querying-across-tables/04-01-joins-inner-left-right-full
```

(`T=run` is the default. From inside the unit directory it's `make db-reset`, `make run`.)

Output:

```
1) INNER JOIN customers↔orders — только совпавшие пары (строк: 3):
   Алиса Иванова    заказ #1 (paid)
   Алиса Иванова    заказ #3 (shipped)
   Борис Петров     заказ #2 (created)
   → Карины тут нет: у неё нет заказов, совпадать не с чем.

2) LEFT JOIN customers←orders — все клиенты, заказ если есть (строк: 4):
   Алиса Иванова    заказ #1   статус paid
   Алиса Иванова    заказ #3   статус shipped
   Борис Петров     заказ #2   статус created
   Карина Сидорова  заказ —    статус NULL
   → Карина осталась: заказа нет → order_id и status = NULL.

3) RIGHT JOIN orders→customers — тот же результат, что LEFT (строк: 4):
   Алиса Иванова    заказ #1   статус paid
   Алиса Иванова    заказ #3   статус shipped
   Борис Петров     заказ #2   статус created
   Карина Сидорова  заказ —    статус NULL
   → RIGHT = LEFT с переставленными таблицами; в коде почти всегда пишут LEFT.

4) FULL JOIN — сверка листов пересчёта (зал {1,2} vs склад {2,4}):
   напиток         зал    склад
   Эспрессо         10        —
   Капучино          5        3
   Колд брю          —        8
   → строки есть с обеих сторон: только в зале, только на складе, в обоих.
```

(The demo prints in Russian.) INNER gave 3 rows (Karina dropped), LEFT and RIGHT gave 4 each (Karina stayed with `NULL`), FULL merged the two sheets with discrepancies on the edges. The same dataset, four `JOIN`s — four different answers.

## The fence

What we simplified. First, the `ON` condition here is on an unindexed pair (`c.id::text = o.customer_id`), and on five rows that doesn't matter; but on large tables a `JOIN` without a suitable index on the join key is either a hash join that scans the whole table or a nested loop, and the cost grows fast (how exactly the server picks a join method and why an index on the key matters — module 06). Second, the `c.id::text` cast in `ON` is a consequence of `customer_id` being deliberately `TEXT` in the canon; in your own schema it's best to keep join keys of the same type (better still — a real foreign key), so the index lands and no cast is needed. And `FULL JOIN` is rare in applications — it's almost always a sign of "I'm merging two independent sources"; within one normalized schema data is usually linked directionally and `LEFT` is enough.

## Takeaways

- `INNER JOIN` keeps only pairs matched on both sides — for an "all entities" report it silently loses rows without a pair.
- `LEFT JOIN` keeps all rows of the left table; no pair on the right → its columns are `NULL` (sqlc types them as nullable).
- `LEFT JOIN ... WHERE right.key IS NULL` is the standard anti-join "find rows without a pair."
- `RIGHT JOIN` is the mirror of `LEFT`; in code you almost always write `LEFT` by swapping the table order.
- `FULL JOIN` keeps mismatches on both sides — it's a tool for reconciling two sources, not an everyday `JOIN` within a schema.

Next up — the **04-02 "Multi-table and self-joins"** unit: we'll assemble an order receipt from four tables at once and learn to join a table with itself (a coffee-shop staff hierarchy).
