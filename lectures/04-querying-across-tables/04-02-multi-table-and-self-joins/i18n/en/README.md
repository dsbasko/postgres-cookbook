# 04-02 — Multi-table and self-joins

Last lesson you brought Karina back into the report — but "customer and order count" is a summary. And midday a report from the coffee shop drops into the chat:

> **Ruslan (in chat, 12:07):** Guest at the counter. Asking what's in their order. In the admin panel — one row: customer_id, that's all. No name, no drinks. Need a receipt.

The moment support opens a specific order to answer a customer, a summary isn't enough: you need a **receipt** — what was ordered, at what price, under whose name. And a raw `orders` row is useless to a human: `customer_id` sits in it as a string identifier, and what's inside the order isn't in `orders` at all. The order number is in `orders`, the customer name in `customers`, the line items in `order_items`, the drink names in `drinks`. To print "order #1, Alice, cappuccino ×1" you have to assemble rows from all four tables — in one query, not four round-trips to the database.

And a separate, initially counterintuitive technique: a table can be joined **to itself**. It sounds odd until you meet a hierarchy: a barista has a manager, and a manager is just another employee from the same `staff` table. To show the manager's name next to the employee's, you join `staff` twice — that's a self-join.

> [!NOTE]
> Carried over from earlier lessons: `LEFT JOIN` and why it keeps an unmatched row (04-01), prices as cents-`BIGINT` (01-01), and the Brew table map from 00-01 (`orders`, `customers`, `order_items`, `drinks`).

## A JOIN chains through any number of tables

`JOIN` isn't limited to two tables: each additional `JOIN` attaches one more via its key. The chain `orders → customers → order_items → drinks` links the order to the customer (`c.id::text = o.customer_id`), the order to its items (`oi.order_id = o.id`), and the item to the drink (`d.id = oi.drink_id`). The order of `JOIN`s doesn't affect the result of an `INNER` chain (the query planner — the server component that decides how to physically run a query; covered in module 06 — picks how to join them), but it reads more easily "along the thread": from the order to its details.

`orders` here is the trunk: the customer hangs off it by `customer_id`, the line items by `order_id`, and each item's drink by `drink_id`.

```
orders ──┬─▶ customers      c.id::text = o.customer_id   → customer name
(order)  │
         └─▶ order_items     oi.order_id = o.id           → what was ordered
                  └─▶ drinks  d.id = oi.drink_id           → drink name
```

One order yields as many receipt rows as it has items: that's why order #1 with two items unfolds into two rows. This consequence has a name — row multiplication (fan-out): joining a parent table to a child repeats each parent row once per matching child row. In a receipt that's exactly what you want: one row per item. But this same multiplication quietly breaks the count the moment an aggregate sits on top of such a `JOIN`: `count(o.id)` would count items, not orders. Keep fan-out as a name — the next lesson 04-03 leans on it, where it becomes a trap.

We compute the line total (`quantity × price`) right in SQL: `oi.quantity * oi.unit_price`. The price is `BIGINT` (cents, see 01-01), the quantity is `INT`; we cast the product to `::bigint` so it's `int64` in Go too (without the cast sqlc would infer the type from the first operand — `int32` — which could overflow on large totals).

## Self-join: one table under two aliases

A self-join is an ordinary `JOIN` where both sides are the same table but under **different aliases**. The aliases are mandatory: without them `SELECT name FROM staff JOIN staff` is ambiguous — whose `name`? We give `staff e` ("employee") and `staff m` ("manager") and link them by the reference inside the row:

> [!NOTE]
> There's no `staff` table in the Brew map from 00-01 — it's a local table of this unit, defined in its `schema.sql` (applied on `db-reset` on top of the canon). It doesn't touch the Brew canon: it's here only as a clear example of a hierarchy with a reference to "one of its own" (a row points to another row of the same table) — the canonical tables have no such self-reference.

```sql
FROM staff e
LEFT JOIN staff m ON m.id = e.manager_id
```

`e.manager_id = m.id` "unfolds" the reference: for an employee's row we find their manager's row in the same table and take the name from there. The same `staff` table is read twice — on the left as "employees," on the right as "managers":

```
one staff table, read under two aliases:

  employee (e)               manager (m)
  e.name   e.manager_id  ─▶  m.id   m.name
  Boris         1             1      Anna
  Vera          1             1      Anna
  Gleb          1             1      Anna
  Anna        NULL            —      no reference → LEFT JOIN yields NULL
```

The `LEFT JOIN` matters here: the most senior person (Anna) has `manager_id = NULL`, and the `INNER` variant would drop her, while `LEFT` keeps her with an empty manager.

The employee Boris in this hierarchy is a **namesake of the customer Boris Petrov** from the `customers` directory: they're different people — one makes the coffee, the other buys it. Names recur in Brew, as in life; what tells them apart is the table the row lives in.

This unfolds exactly one level — "an employee and their direct manager." To walk the hierarchy to any depth (the manager's manager and so on) a self-join isn't enough — you need a recursive CTE (unit 08-04).

## What our code shows

Two queries in `query.sql`. The multi-table receipt:

```sql
-- name: OrderReceipt :many
SELECT o.id AS order_id, c.name AS customer, d.name AS drink,
       oi.quantity, oi.unit_price, (oi.quantity * oi.unit_price)::bigint AS line_total
FROM orders o
JOIN customers c    ON c.id::text = o.customer_id
JOIN order_items oi ON oi.order_id = o.id
JOIN drinks d       ON d.id = oi.drink_id
ORDER BY o.id, oi.id;
```

And the hierarchy self-join (`StaffWithManager`, above). In `main.go` both reads are thin: the receipt is printed line by line, summing `line_total` into a grand total; the hierarchy as "employee → manager," substituting "— (старший)" where there's no manager.

> [!NOTE]
> **Check yourself.** How many rows will `OrderReceipt` return for order #1, and why? And why does the hierarchy self-join use `LEFT JOIN` rather than `INNER` — what would happen to Anna?

> [!TIP]
> **Answer.** Order #1 yields **two** rows — by fan-out, one per item (cappuccino and cold brew), as you can see in the "Running it" output. The `LEFT JOIN` is needed because Anna's `manager_id` is empty: an `INNER JOIN` would drop the unmatched row, and Anna (the top of the hierarchy) would vanish from the result; `LEFT JOIN` keeps her with `NULL` for the manager.

## Running it

Bring up the sandbox (from the repo root) and apply the Brew base schema:

```sh
docker compose up -d
make lecture L=04-querying-across-tables/04-02-multi-table-and-self-joins T=db-reset
make lecture L=04-querying-across-tables/04-02-multi-table-and-self-joins
```

(`T=run` is the default. From inside the unit directory it's `make db-reset`, `make run`.)

Output:

```
1) Чек заказа — JOIN по 4 таблицам (orders→customers→order_items→drinks):
   заказ клиент           напиток      кол     цена    сумма
   #1    Алиса Иванова    Капучино       1     4.50     4.50
   #1    Алиса Иванова    Колд брю       1     5.20     5.20
   #2    Борис Петров     Эспрессо       1     3.00     3.00
   #3    Алиса Иванова    Латте          2     4.80     9.60
   итого по всем позициям: 22.30

2) Иерархия персонала — self-join staff (e=сотрудник, m=руководитель):
   сотрудник роль         руководитель
   Анна     manager      — (старший)
   Борис    barista      Анна
   Вера     barista      Анна
   Глеб     shift-lead   Анна
   → у Анны руководителя нет (manager = NULL) — LEFT JOIN её не выкинул.
```

(The demo prints in Russian.) Read the output. The receipt unfolded the orders into line rows: order #1 gave two rows (cappuccino 4.50 and cold brew 5.20), order #3 — latte 4.80 ×2 = 9.60. The `JOIN` to `drinks` substituted drink names for `drink_id`, the `JOIN` to `customers` — a name for the string `customer_id`; the total 22.30 is the sum of all `line_total`s. The second query joined `staff` to itself: Boris, Vera, and Gleb have Anna in the manager column, and Anna herself has "— (старший)" because her `manager_id` is empty and the `LEFT JOIN` substituted `NULL` without dropping the row.

## The fence

What we simplified.

- The multi-table `JOIN` runs on several keys at once, and each wants an index on large data — otherwise the planner joins tables by brute force. There's already an index under the FK `order_items.order_id` in the base schema (`order_items_order_id_idx`); but `c.id::text = o.customer_id` is a `JOIN` on an expression, and a plain index on `customer_id` won't speed it up (how to index an expression and how the server picks a join method — module 06).
- We keep the four-`INNER JOIN` chain short on purpose. In production such reports quickly grow to a dozen tables, and then "in what order and by what method to join" is decided not by readability but by the query plan.
- A self-join unfolds the hierarchy exactly one level. For arbitrary depth you need a recursive CTE (08-04); stacking N self-joins "just in case" is an anti-pattern.

## Takeaways

- `JOIN` isn't limited to two tables: a chain of `JOIN`s links any number of tables, each by its own key.
- Derived values (`quantity × price`) can be computed right in `SELECT`; a type cast (`::bigint`) removes surprises about the result's width.
- A self-join is a `JOIN` of a table with itself under different aliases; the aliases are mandatory, or the columns are ambiguous.
- A self-join unfolds an "employee → manager" hierarchy by one level; `LEFT JOIN` keeps the top (the one with no manager).
- Arbitrary hierarchy depth is a recursive CTE (08-04), not a stack of self-joins.

The receipt unfolded an order into separate rows — one per drink. But the business more often asks the reverse: not "what's in each order" but "how many orders does a customer have," "for what amount," "what's the average." That means collapsing rows into a summary — and here a trap waits: `count(*)` and `count(column)` count different things, and on a `LEFT JOIN` the difference quietly skews the report (that same orderless Karina will be on the edge again). That's the **04-03 "Aggregation, GROUP BY/HAVING"** unit.
