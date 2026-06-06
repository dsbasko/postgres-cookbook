# 04-02 — Multi-table and self-joins

A Brew order receipt isn't one table. The order number lives in `orders`, the customer name in `customers`, the line items in `order_items`, the drink names in `drinks`. To print a human-readable receipt ("order #1, Alice, cappuccino ×1") you have to assemble rows from all four — in one query, not four round-trips to the database.

And a separate, initially counterintuitive technique: a table can be joined **to itself**. It sounds odd until you meet a hierarchy: a barista has a manager, and a manager is just another employee from the same `staff` table. To show the manager's name next to the employee's, you join `staff` twice — that's a self-join.

## A JOIN chains through any number of tables

`JOIN` isn't limited to two tables: each additional `JOIN` attaches one more via its key. The chain `orders → customers → order_items → drinks` links the order to the customer (`c.id::text = o.customer_id`), the order to its items (`oi.order_id = o.id`), and the item to the drink (`d.id = oi.drink_id`). The order of `JOIN`s doesn't affect the result of an `INNER` chain (the planner picks how to join them), but it reads more easily "along the thread": from the order to its details.

We compute the line total (`quantity × price`) right in SQL: `oi.quantity * oi.unit_price`. The price is `BIGINT` (cents, see 01-01), the quantity is `INT`; we cast the product to `::bigint` so it's `int64` in Go too (without the cast sqlc would infer the type from the first operand — `int32` — which could overflow on large totals).

## Self-join: one table under two aliases

A self-join is an ordinary `JOIN` where both sides are the same table but under **different aliases**. The aliases are mandatory: without them `SELECT name FROM staff JOIN staff` is ambiguous — whose `name`? We give `staff e` ("employee") and `staff m` ("manager") and link them by the reference inside the row:

```sql
FROM staff e
LEFT JOIN staff m ON m.id = e.manager_id
```

`e.manager_id = m.id` "unfolds" the reference: for an employee's row we find their manager's row in the same table and take the name from there. The `LEFT JOIN` matters here: the most senior person (Anna) has `manager_id = NULL`, and the `INNER` variant would drop her, while `LEFT` keeps her with an empty manager.

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

## Running it

Bring up the sandbox (from the repo root) and apply the canon:

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

(The demo prints in Russian.) One query returned a receipt from four tables (note: order #1 is two rows, one per drink). The second joined `staff` to itself and unfolded `manager_id` into the manager's name.

## The fence

What we simplified. The multi-table `JOIN` here is on several keys at once, and each of them wants an index on large data — otherwise the planner joins tables by brute force; in the canon there's already an index under the FK `order_items.order_id` (`order_items_order_id_idx`), but `c.id::text = o.customer_id` is a cast, and a plain index on `customer_id` won't help it (indexing an expression — module 06). We deliberately keep the four-`INNER JOIN` chain short: in production such reports quickly grow to a dozen tables, and then "in what order and by what method to join" is decided not by readability but by the query plan. And a self-join unfolds the hierarchy exactly one level — for arbitrary depth you need a recursive CTE (08-04), and stacking N self-joins "just in case" is an anti-pattern.

## Takeaways

- `JOIN` isn't limited to two tables: a chain of `JOIN`s links any number of tables, each by its own key.
- Derived values (`quantity × price`) can be computed right in `SELECT`; a type cast (`::bigint`) removes surprises about the result's width.
- A self-join is a `JOIN` of a table with itself under different aliases; the aliases are mandatory, or the columns are ambiguous.
- A self-join unfolds an "employee → manager" hierarchy by one level; `LEFT JOIN` keeps the top (the one with no manager).
- Arbitrary hierarchy depth is a recursive CTE (08-04), not a stack of self-joins.

Next up — the **04-03 "Aggregation, GROUP BY/HAVING"** unit: we'll collapse rows into summaries (counts, sums, averages) and unpack the treacherous difference between `count(*)` and `count(column)`.
