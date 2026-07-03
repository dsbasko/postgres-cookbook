# 04-05 — Subqueries: EXISTS vs IN

A query often answers a question through another question: "drinks above the average price" (what's the average?), "customers who have orders" (do they?), "drinks not in any promo." The inner question is a **subquery**: a query inside a query. It comes in three forms, and the choice between two of them — `IN` and `EXISTS` — isn't a matter of taste: on data with `NULL` they give **different** answers, and `NOT IN` can silently return "nothing."

We already saw this trap in 03-06 as a lesson on three-valued `NULL` logic. Here we look at it from another angle — as the main reason to choose `EXISTS` for "not among."

Coming in, you already know the pairing: we covered `NOT IN` + `NULL` in 03-06 (there it illustrated three-valued logic). So below we recap the `NOT (… OR NULL) = NULL` mechanics briefly and go straight to the conclusion: which subquery form to choose.

## Three subquery forms

**Scalar** — the subquery returns one value and is substituted like a plain number/string:

```sql
WHERE base_price > (SELECT avg(base_price) FROM drinks)
```

The average is computed once, and the comparison is against that number. If such a subquery returns more than one row, it's a runtime error (that's what makes it scalar).

That "more than one row → error" has a missing twin — **zero rows**. A scalar subquery with no rows returns neither emptiness nor an error, but `NULL`. Then `WHERE base_price > (subquery)` becomes `base_price > NULL` — which is `NULL` (UNKNOWN), and the row silently drops out of the result. The loud case (`> 1` row) stops you with an error; the quiet one (`0` rows) silently trims rows — far harder to notice.

> [!WARNING]
> A scalar subquery with zero rows is more dangerous than one with extra rows. `> 1` row throws an error and stops the query — you see it at once. But `0` rows yields `NULL`: the comparison `x > NULL` becomes `NULL`, the row is filtered out silently, and the query returns a plausible-looking but incomplete result. In a module where `NULL` is the running villain, it's this quiet case that bites: add a `WHERE` to the scalar subquery that one day selects nobody, and the outer query starts silently losing rows. If "no rows" is a valid outcome, wrap the subquery in `coalesce((SELECT …), <fallback value>)` or pull it into a separate step and check it explicitly.

**IN** — checks that a value is in the set from the subquery: `id IN (SELECT drink_id FROM order_items)` — "a drink whose id appears among the ordered ones."

**EXISTS** — a correlated subquery: for each outer row it asks "is there at least one matching row inside." `EXISTS` doesn't care about values — only the fact of existence, so inside you write `SELECT 1` and it stops at the first match.

> [!NOTE]
> There's a fourth form too — a **derived table**: a subquery in `FROM` that behaves like an ordinary table (`FROM (SELECT …) AS t`). We leave it aside here; we'll get to the idea in 04-06 via `WITH` steps (CTEs) — the same subquery, but named and readable top-down.

## IN vs EXISTS: why it matters

For "is among," `IN` and `EXISTS` usually give the same result, and the planner often turns one into the other. The difference surfaces on **negation** (`NOT IN` vs `NOT EXISTS`) when the subquery can return `NULL`.

Postgres expands `x NOT IN (a, b, NULL)` as `NOT (x=a OR x=b OR x=NULL)`. The term `x=NULL` is always `NULL` (not `false`!). If `x` equals neither `a` nor `b`, you get `NOT (false OR false OR NULL)` = `NOT (NULL)` = `NULL` — and a row with a `NULL` condition does **not** pass the filter. A single `NULL` in the set is enough to make `NOT IN` return empty for everyone:

```
finding drinks NOT on promo; promos (featured_drink_id) = {1, NULL}

  d.id NOT IN (1, NULL)
       = NOT ( d.id = 1  OR  d.id = NULL )
                                └── comparing with NULL → NULL, not false

  drink #4 (not in any promo):
       NOT ( false OR NULL ) = NOT (NULL) = NULL   → row fails the filter

  one NULL in the set → NOT IN drops EVERYONE → answer 0 (though there are 4)
```

`NOT EXISTS` doesn't break this way: it asks "is there no matching row," and a subquery row with `NULL` matches nothing (`NULL` equals nothing) — so it excludes nobody extra. Hence the simple rule: **for "not among," use `NOT EXISTS`** (or `NOT IN` with a guaranteed non-`NULL` subquery).

| form | question | what it returns / how it behaves |
|---|---|---|
| scalar `(SELECT …)` | "which single value?" | one value; more than one row → runtime error |
| `IN (subquery)` | "is the value in the set?" | reliable; but `NOT IN` breaks if a `NULL` is in the set |
| `EXISTS (subquery)` | "is there at least one row?" | the fact of existence (`SELECT 1`); `NOT EXISTS` is `NULL`-safe |

## What our code shows

Subqueries over the base tables:

```sql
-- AbovePriceAvg:           WHERE base_price > (SELECT avg(base_price) FROM drinks)
-- DrinksOrdered:           WHERE id IN (SELECT drink_id FROM order_items)
-- CountCustomersWithOrders WHERE EXISTS (SELECT 1 FROM orders o WHERE o.customer_id = c.id::text)
```

And the trap on the lab `promo`, where `featured_drink_id` may be `NULL` (a "whole menu" promo):

```sql
-- CountNotFeaturedNotIn:     WHERE id NOT IN (SELECT featured_drink_id FROM promo)   -- → 0 (trap)
-- CountNotFeaturedNotExists: WHERE NOT EXISTS (SELECT 1 FROM promo p WHERE p.featured_drink_id = d.id)  -- → 4
```

## Running it

Bring up the sandbox (from the repo root) and apply the Brew base schema:

```sh
docker compose up -d
make lecture L=04-querying-across-tables/04-05-subqueries-exists-vs-in T=db-reset
make lecture L=04-querying-across-tables/04-05-subqueries-exists-vs-in
```

(`T=run` is the default. From inside the unit directory it's `make db-reset`, `make run`.)

Output:

```
1) Scalar-подзапрос — напитки дороже средней цены (avg=4.00):
   #2 Капучино     4.50
   #3 Латте        4.80
   #4 Колд брю     5.20

2) IN-подзапрос — напитки, которые хоть раз заказывали (4): Эспрессо, Капучино, Латте, Колд брю
   → зелёного чая нет: его не заказывали ни разу.

3) EXISTS-подзапрос — клиентов хотя бы с одним заказом: 2 (Карина без заказов не в счёт).

4) «Сколько напитков НЕ на акции?» — акции = {эспрессо #1, всё меню (NULL)}:
   NOT IN (...)      → 0   ← ловушка: NULL в списке обнулил ответ
   NOT EXISTS (...)  → 4   ← правильно (5 напитков минус эспрессо #1)
```

(The demo prints in Russian.) The first three forms behaved as expected. And item 4 is the main one: `NOT IN` with the list `{1, NULL}` returned 0 (though four drinks aren't on promo), while `NOT EXISTS` returned an honest 4. A single `NULL` in the set nullified the whole `NOT IN`.

## The fence

What we simplified.

- The `NOT IN` + `NULL` trap isn't rare: a subquery over a nullable column (and schemas have plenty) will sooner or later return `NULL`, and `NOT IN` will silently lie. So in production "not among" is written with `NOT EXISTS` or `NULL` is filtered out explicitly (`... WHERE featured_drink_id IS NOT NULL`).
- Performance: on our data there's no difference, but on large tables `EXISTS`/`NOT EXISTS` is usually friendlier to indexes (stops at the first match), and an `IN` with a huge list of values from the application is better replaced by `= ANY($1::bigint[])` — a separate discussion in 10-03.
- A scalar subquery must return exactly one value: return several and it's a production error, not a silent bug (at least it's loud here).

## Takeaways

- A scalar subquery is substituted as one value; return more than one row and it's a runtime error, while **zero** rows is a silent `NULL`, and comparing against it quietly trims rows.
- `IN (subquery)` is "the value is in the set"; `EXISTS (subquery)` is "there's at least one matching row" (values don't matter, you write `SELECT 1`).
- `NOT IN` with a list that contains a `NULL` returns empty for everyone — `NOT (… OR NULL)` collapses to `NULL`.
- For "not among," use `NOT EXISTS` (or `NOT IN` with a guaranteed non-`NULL` subquery).
- `EXISTS`/`NOT EXISTS` is usually friendlier to indexes; a giant `IN` list from the application is a candidate for `= ANY($1::type[])` (10-03).

> [!NOTE]
> **Check yourself.** `SeedPromo` already inserts a "whole menu" promo with `featured_drink_id = NULL`. Imagine you added such a `NULL` to `promo` by hand. What do the two queries from `## Running it` return — `CountNotFeaturedNotIn` (via `NOT IN`) and `CountNotFeaturedNotExists` (via `NOT EXISTS`)? Which one breaks on the `NULL`, and which one counts honestly?

> [!TIP]
> **Answer.** `CountNotFeaturedNotExists` (via `NOT EXISTS`) counts honestly: **4**. A `promo` row with `featured_drink_id = NULL` matches no `d.id` (`NULL` equals nothing), so out of five drinks only espresso `#1` is excluded — 4 remain. But `CountNotFeaturedNotIn` (via `NOT IN`) breaks: the list becomes `{1, NULL}`, and for any drink with `id <> 1` that's `NOT (false OR NULL) = NOT (NULL) = NULL`, so the row fails the filter — and the answer collapses to **0** (though the same 4 drinks aren't on promo). A single `NULL` in the set takes down the whole `NOT IN`; `NOT EXISTS` is immune to it.

Subqueries solve "a question inside a question," but nest them two or three levels deep and the query stops being readable. You can pull them out into named steps with `WITH` and assemble a top-down pipeline — far clearer than nesting. Next up — the **04-06 "CTEs and materialization"** unit: we'll assemble a readable pipeline from steps and unpack when Postgres "materializes" a CTE into an intermediate table versus inlining it into the main query.
