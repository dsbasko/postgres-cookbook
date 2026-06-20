# 01-02 — text, boolean, and the NULL teaser

The "how many orders does each customer have" report at Brew was quick to write — and the loyalty mailing has already gone out based on it. In the morning, Evgeny shows up in the team chat.

> **Evgeny (in chat):** Cross-checking the loyalty mailing against the "orders per customer" report. Karina Sidorova — didn't get one. I have her profile open: signed up, but she's not in the report. Where did she go?
>
> **You:** She's in `customers`, the row is right there. Where does she disappear on the way into the report?
>
> **Dmitry:** How are the orders counted — `count(*)` or `count` on a column? Recount with `count(*)` and compare. And look at what Karina has in `order_id`.

You recount: `count(*)` finds one row more than the old `count(order_id)` did. The extra row is Karina: the customer is right there, orders — none, and her empty `order_id` is what the old count was silently skipping.

> **Evgeny (in chat):** She's back, I can see her. Noting it down: there's some NULL living in the database, and it eats my customers. When you figure it out — explain it in human terms.

Not a join bug: Karina has no orders, so her `order_id` is `NULL`, and `NULL` behaves differently than it looks.

This unit is about three types that look boring but are exactly what applications trip over: `text` (and why not `char(n)`), `boolean` (with its three-valued logic), and `NULL`. The key point about `NULL` is that it's not "empty" and not "zero" — it's **"unknown"**. The full `NULL` semantics are coming in 03-06; here it's a teaser, so the trap doesn't catch you off guard.

## text, not char(n) or varchar(n)

In Postgres the default string type is `text`, with no length limit. `varchar(n)` is `text` with a length check (and almost never needed: constrain length with a `CHECK` if you must). `char(n)`, though, is a separate trap: it pads the string with spaces to a fixed length. Because of that `'abc'::char(5)` actually stores `'abc  '`, and on comparison the trailing spaces are "eaten": `'abc'::char(5) = 'abc  '::char(5)` → `true`. In `text` spaces are significant: `'abc' = 'abc '` → `false`. So in the course (and in the Brew base schema) we keep `text` — predictable byte-wise comparison.

## boolean: true, false, and… NULL

`boolean` looks two-valued, but in SQL it's three-valued: `true`, `false`, and `NULL` (unknown). Logical values come easily right out of predicates: `base_price > 400` is already a `boolean`, and sqlc types such a column as Go `bool`. But the moment a `NULL` enters the expression, the result can become `NULL` — and that's the next section.

## NULL is "unknown", not "empty"

The key intuition: `NULL` means "the value is unknown." So **comparing with `NULL` via `=` gives not `false` but `NULL`**: `NULL = NULL` is "unknown = unknown" → also `NULL`. Two things follow:

- `WHERE col = NULL` never fires (the condition is never `true`) — to test for the absence of a value there's `IS NULL` / `IS NOT NULL`.
- Aggregates skip `NULL`: `count(*)` counts all rows, while `count(col)` counts only rows where `col` is not `NULL`. That same Karina is lost if you count `count(order_id)` instead of `count(*)`.

`NULL` appears in data naturally — for example, from a `LEFT JOIN`: for a customer with no orders, the columns from the right table are `NULL`. And that's the correct, type-safe way to express "there is no value": sqlc sees that a `LEFT JOIN` column is nullable and types it as `pgtype.Int8` (with a `Valid` field), not as a bare `int64`.

## boolean and NULL: a cheat sheet

| Expression | Result | What to remember |
|---|---|---|
| `base_price > 400` | `true` / `false` / `NULL` | a predicate is already a `boolean`, and it's three-valued |
| `NULL = NULL` | `NULL` | "unknown = unknown" is also unknown, not `true` |
| `col = NULL` | never `true` | test for absence with `IS NULL` / `IS NOT NULL` |
| `count(*)` | all rows | rows are counted as they are |
| `count(col)` | rows where `col` is not `NULL` | `NULL` is skipped — this is where Karina is lost |

This is the everyday working minimum; the full three-valued logic (`NOT IN` with `NULL`, `COALESCE`, `IS DISTINCT FROM`) is covered in 03-06.

## What our code shows

The first query is `NULL` in a comparison, on literals:

```sql
-- name: NullComparison :one
SELECT
    ((NULL = NULL) IS NOT TRUE)::boolean  AS eq_null_is_not_true,
    ((NULL = NULL) IS NULL)::boolean      AS eq_null_is_unknown;
```

Both are `true`: `NULL = NULL` is not `TRUE` and is in fact `NULL`. Next is a real `NULL` from a `LEFT JOIN` and the `count(*)` vs `count(col)` contrast:

```sql
SELECT c.id AS customer_id, c.name, o.id AS order_id          -- CustomersWithOrders :many
FROM customers c LEFT JOIN orders o ON o.customer_id = c.id::text
ORDER BY c.id, o.id;

SELECT count(*) AS rows_total, count(o.id) AS rows_with_order  -- CountStarVsCol :one
FROM customers c LEFT JOIN orders o ON o.customer_id = c.id::text;
```

In Go `order_id` is a `pgtype.Int8`; we print `NULL` when `!Valid`. The last two queries are a `boolean` from a predicate (`base_price > 400`) and the behavior of `text`/`char(n)` (`'abc' = 'abc '` vs `char(5)` padding).

## Running it

```sh
docker compose up -d
make lecture L=01-data-types/01-02-text-boolean-and-null-teaser T=db-reset
make lecture L=01-data-types/01-02-text-boolean-and-null-teaser
```

Output:

```
1) (NULL = NULL) — это не TRUE и не FALSE, а NULL («неизвестно»):
   (NULL = NULL) IS NOT TRUE = true;  IS NULL = true
   → отсутствие значения проверяем через IS NULL, не через = NULL.

2) LEFT JOIN customers ↔ orders — order_id у клиента без заказов = NULL:
CUSTOMER_ID  ИМЯ              ORDER_ID
1            Алиса Иванова    1
1            Алиса Иванова    3
2            Борис Петров     2
3            Карина Сидорова  NULL

3) count(*) = 4 (все строки), count(o.id) = 3 (без NULL-заказа Карины)

4) boolean из выражения base_price > 400 (в Go это bool):
ID  НАЗВАНИЕ     IS_PREMIUM
1   Эспрессо     false
2   Капучино     true
3   Латте        true
4   Колд брю     true
5   Зелёный чай  false

5) text сравнивает по байтам, char(n) дополняет пробелами:
   'abc' = 'abc '           → false  (text: пробел значим)
   'abc'::char(5) = 'abc  ' → true   (char(n): паддинг съел пробелы)
```

(The demo prints in Russian.) There's Karina: in the `LEFT JOIN` her `order_id` is `NULL`, and `count(o.id)` (=3) doesn't count her, while `count(*)` (=4) does. An "orders per customer" report should show her with a zero, not lose her — and now you can see why the naive `count` did exactly that.

## The fence

This is only a teaser. The rest is in 03-06; here we hold three rules:

- **The full `NULL` semantics are still ahead.** `NOT IN` with `NULL` (the classic hole that returns nothing), `COALESCE`/`NULLIF`/`IS DISTINCT FROM`, three-valued logic in `WHERE`/`CHECK`/unique indexes — all of it in 03-06.
- **Absence of a value — only `IS NULL` / `IS NOT NULL`.** Never `= NULL`: such a condition won't become `true`.
- **You'll almost never meet `char(n)` for a good reason in production.** If you see it in someone else's schema, it's usually a historical mistake rather than a deliberate choice.

## Takeaways

- Keep `text`. `char(n)` silently pads with spaces and corrupts comparison; `varchar(n)` is `text` with a usually-pointless length check.
- `boolean` is three-valued: `true`, `false`, `NULL`. A predicate like `base_price > 400` is already a `bool`.
- `NULL` is "unknown", not "empty". `NULL = NULL` → `NULL`. Test for absence with `IS NULL`.
- `count(*)` counts rows, `count(col)` skips `NULL`. A `LEFT JOIN` produces real `NULL`s — sqlc types them as `pgtype.*` with a `Valid` field.

Next up — the **01-03 "date, time, and timestamptz"** unit: why time is always stored in `timestamptz`, how the same instant looks different under `SET TIME ZONE`, and what the trap of `timestamp` without a zone is.
