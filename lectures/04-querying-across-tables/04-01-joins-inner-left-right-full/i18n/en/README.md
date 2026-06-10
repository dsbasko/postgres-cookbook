# 04-01 — JOIN: inner / left / right / full

Marketing dreamed up a "we miss you" campaign: send a promo code to everyone who created a Brew account but hasn't ordered even once. They need a list of customers with their order counts. You write the obvious thing: join `customers` to `orders` by customer, count the rows, hand it over. The list goes to marketing, ticket closed.

The next day a question lands: "Where's Karina? She signed up last week, she's in the admin panel, but she's not in your list." You open it — and indeed she isn't. The query didn't fail and didn't raise an error. It silently dropped exactly the people the campaign was for — customers with no orders at all. The report didn't lie in its numbers. It lied by omission: the row that shouldn't have been missing simply wasn't there.

It's not the whole query — it's one word in it: which `JOIN` you picked. There are four ways to connect two tables, and they differ precisely in whom they keep and whom they throw away. Karina dropped because we used the kind that keeps only matched pairs. Now all four in turn, on the same pair `customers` and `orders`, swapping one word at a time.

## INNER JOIN — matches only

`INNER JOIN` (you can write just `JOIN`) keeps rows that found a pair on both sides at once. A customer with no order and an order with no customer don't make it through. It's the right choice when you want exactly the matched pairs — "orders together with the data of the customer who placed them." But for an "all customers" report it's treacherous: those who haven't ordered yet it silently removes.

One detail about the join condition. `customers.id` is `BIGINT`, while `orders.customer_id` is `TEXT` (that's the Brew base schema: in the real CDC stream orders and the customer directory travel independently, and an order holds the customer id as a string). So we bring the keys together with an explicit cast `c.id::text = o.customer_id`. The key type doesn't matter for the `JOIN` discussion — what matters is only that the condition links a customer to their order.

In Brew's data Alice has two orders, Boris one: they matched and made it into the result. Karina has no orders, nothing to match — INNER throws her out. That's why the marketing list "lost" exactly her: INNER answers "show the pairs that exist," not "show all customers."

## LEFT JOIN — all of the left, the right if present

The report was supposed to answer a different question: "all customers, and orders — if any." That's `LEFT JOIN`. It keeps all rows of the left table and fills in either a pair or `NULL` on the right when there's no pair.

`customers LEFT JOIN orders` reads as "all customers, and their orders for those who have them." Karina comes back into the result — with `order_id = NULL` and `status = NULL`. This is the most common `JOIN` in applications: "show the entity and its related data without losing entities that have no relations."

That property gives a handy trick. `LEFT JOIN` plus an `IS NULL` check on the right side is "find rows with no pair at all" (it's called an anti-join). The query `... LEFT JOIN orders o ON ... WHERE o.id IS NULL` returns only customers with no orders — the very list for the campaign the lesson opened with, assembled in a single query.

## RIGHT JOIN — the same, mirrored

`RIGHT JOIN` is `LEFT` flipped: it keeps all rows of the right table. Put orders on the left, customers on the right — `orders RIGHT JOIN customers` — and you get exactly what `customers LEFT JOIN orders` gives: all customers, orders if any, Karina with `NULL`.

That's why `RIGHT` is almost never written in code. Any `RIGHT` turns into a `LEFT` by swapping the tables, and `LEFT` reads left-to-right more naturally: "take all customers, glue on the orders." You need to know `RIGHT` to read others' SQL; in your own you almost always pick `LEFT`.

## FULL JOIN — mismatches on both sides

`FULL JOIN` keeps unmatched rows on both sides at once: left ones with no pair and right ones with no pair.

Let's be honest: you can't show it in the Brew data. Every order is tied to an existing customer, there are no "orphan" orders on the right — and `FULL` would degenerate into a plain `LEFT`. In application code it's rare too: within one normalized schema data is linked directionally, and `LEFT` is almost always enough. You can work a year and not write a single `FULL JOIN`.

But one scenario justifies it: when you reconcile two independent sources, and each may have "its own" rows the other doesn't. Take an end-of-day stock count. The floor recounted drinks and turned in sheet `{1, 2}`, storage turned in `{2, 4}`. Drink 2 is in both, drink 1 only on the floor, drink 4 only in storage. A `FULL JOIN` on `drink_id` merges both sheets into one table: what's in both, what's only on the floor (`storage = NULL`), what's only in storage (`floor = NULL`). That's its job — merge two sources and highlight where they diverged.

## What each JOIN keeps

All four are the same pair of sets under different rules. The left table, the right one, and the zone where they intersect; the `JOIN` decides which of the three zones make it into the result:

```
      left only            intersection        right only
   ┌─────────────────┐ ┌──────────────┐ ┌───────────────────┐
   │     Karina      │ │ Alice, Boris │ │   order with       │
   │ (customer with  │ │  (matched:   │ │   no customer      │
   │  no order)      │ │  a pair)     │ │ (none in data)     │
   └─────────────────┘ └──────────────┘ └───────────────────┘
```

INNER takes only the middle zone. LEFT — the middle plus the left (there's Karina). RIGHT — the middle plus the right. FULL — all three at once. Any side that's taken but unmatched arrives in the result as `NULL`:

| JOIN | keeps | where `NULL` | use when |
|------|-------|-------------|----------|
| `INNER` | the intersection only | nowhere (pairs guaranteed) | you want exactly the matched pairs |
| `LEFT` | left zone + intersection | in right columns with no pair | "all entities, related data if any" — the most common |
| `RIGHT` | right zone + intersection | in left columns with no pair | almost never: write `LEFT` by swapping tables |
| `FULL` | all three zones | on either side with no pair | reconciling two independent sources |

## What our code shows

`query.sql` has four queries. The first three — over `customers` and `orders` — differ by exactly one word. Here's `INNER`:

```sql
-- name: InnerCustomersOrders :many
SELECT c.name AS customer, o.id AS order_id, o.status
FROM customers c
JOIN orders o ON o.customer_id = c.id::text
ORDER BY c.id, o.id;
```

Change `JOIN` to `LEFT JOIN` — and nothing else:

```sql
-- name: LeftCustomersOrders :many
SELECT c.name AS customer, o.id AS order_id, o.status
FROM customers c
LEFT JOIN orders o ON o.customer_id = c.id::text
ORDER BY c.id, o.id;
```

Same columns, same pair of tables, same `ON` — the difference is one word, and the result changes: Karina comes back. The `RIGHT` variant is even shorter in spirit — it's `FROM orders o RIGHT JOIN customers c ...`, the same tables swapped.

A subtlety visible only in Go: sqlc notices that after a `LEFT JOIN` the `orders` columns can become `NULL` and types them as nullable — `pgtype.Int8` and `pgtype.Text`. In the `INNER` variant the same columns arrive as plain `int64` and `string`: there a match is guaranteed. One word in SQL changes even the types in the generated code.

The fourth query is the `FULL JOIN` of the two count sheets; the drink name comes from `drinks` via `COALESCE(f.drink_id, s.drink_id)` (the key exists on at least one side):

```sql
-- name: ReconcileFull :many
SELECT d.name AS drink, f.qty AS floor_qty, s.qty AS storage_qty
FROM count_floor f
FULL JOIN count_storage s ON s.drink_id = f.drink_id
JOIN drinks d ON d.id = COALESCE(f.drink_id, s.drink_id)
ORDER BY d.id;
```

`cmd/demo/main.go` is thin glue: it calls the typed methods from `internal/db/` and lays the rows out into columns. All the logic is in `query.sql`.

## Running it

Bring up the sandbox (from the repo root) and apply the base schema:

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

(The demo prints in Russian.) Read the output in order. INNER gave three rows: Alice's two orders and Boris's one. Karina isn't in it — exactly the loss the lesson opened with. LEFT and RIGHT gave four rows each: the same three plus Karina with `order_id` and `status` as `NULL`. The set of customers is the same, but now no one dropped. FULL merged the two count sheets: Cappuccino landed in both (5 and 3), Espresso was counted only on the floor, Cold Brew only in storage, and each discrepancy showed up as "—", i.e. `NULL` on the side where the drink is missing. The same dataset, four `JOIN`s — four different answers to "whom to keep."

## The fence

What we simplified.

- On five rows an `ON` over an unindexed pair is invisible, but on large tables a `JOIN` without an index on the join key is either a hash join (builds a hash table on one side) or a nested loop (for each left row scans for a right match), and the cost grows fast. How the server picks a join method and why an index on the key matters — module 06.
- The `c.id::text` cast in `ON` is needed only because `customer_id` is deliberately `TEXT` in the base schema. In your own schema keep join keys of the same type, better still a real foreign key: then the index lands and no cast is needed.
- A `FULL JOIN` within one normalized schema is almost always a sign the data should have been linked directionally and `LEFT` would have done. Its honest place is the seam between two independent sources, each with "its own" rows.

## Takeaways

- `INNER JOIN` keeps only pairs matched on both sides; for an "all entities" report it silently loses rows without a pair.
- `LEFT JOIN` keeps all rows of the left table; no pair on the right → its columns arrive `NULL` (sqlc types them as nullable).
- `LEFT JOIN ... WHERE right.key IS NULL` is the standard anti-join "find rows without a pair."
- `RIGHT JOIN` is the mirror of `LEFT`; in code you almost always write `LEFT` by swapping the table order.
- `FULL JOIN` keeps mismatches on both sides — it's a tool for reconciling two sources, not an everyday `JOIN` within a schema.

Karina is back in the report: one word, `LEFT` instead of `JOIN`, returned the row INNER kept losing without any error. But "customers and their orders" is just two tables. The moment the business asks what exactly is in an order, at what price, and in which shop, `order_items`, `drinks`, and `shops` get pulled toward `orders` — a whole receipt from several tables at once. And sometimes a table has to be joined to itself: to lay out who manages whom on a shift, say. That's the next unit, **04-02 "Multi-table and self-joins."**
