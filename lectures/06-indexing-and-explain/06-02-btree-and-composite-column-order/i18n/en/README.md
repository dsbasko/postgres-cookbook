# 06-02 — B-tree and column order in a composite index

Brew created one index on the menu-items table — on `(category, price)` — and considered "menu search" handled. The query "cappuccinos under 500" flew. But the report "all items priced at 250 across every category" unexpectedly crawled: same index, the `price` column is right there inside it — yet the query seemed not to see it. The reason: **column order in a composite index isn't cosmetic**. A B-tree stores rows sorted by the first column first, and only within equal values of the first column — by the second. Asking "by the second without knowing the first" is like looking up a word in a dictionary sorted by word length first: knowing the second letter doesn't help until you know the first.

The goal of this unit is the **left-prefix rule**: which queries a composite index speeds up and which it doesn't, and why that depends on column order. And right away — what **PG18 skip-scan** changes about that picture: it can pull the index in even where there used to be only a Seq Scan.

## The left-prefix rule

An index on `(a, b)` is physically sorted by `a`, then by `b`. From that follows which queries it serves:

- **`WHERE a = ... AND b = ...`** — ideal: descend by `a`, then by `b` within it. Both conditions land in `Index Cond`.
- **`WHERE a = ...`** (leading column only, the **left prefix**) — also works: all rows with the desired `a` sit contiguously in the index. `Index Cond` on `a`.
- **`WHERE b = ...`** (second column only) — classically **doesn't work**: rows with the desired `b` are scattered across the whole index (a separate slice inside each `a`). It's cheaper for the planner to read the whole table (`Seq Scan`) than to jump around the index.

Hence the practical rule: **put the column you filter by most often, and by equality, first** (not by range). The index `(category, price)` serves search by category and by "category + price"; for search by "price only" it is, in the classic world, useless — you need a separate index on `(price)` or a different order.

## What PG18 skip-scan changes

Previously, "second column only" meant a guaranteed `Seq Scan`. **PostgreSQL 18 added skip-scan for B-trees**: if the leading column has few distinct values (our case — 4 categories), the planner can iterate through them one by one and, inside each, dive into the index for the desired `b`. The result is a series of small searches instead of reading the whole table.

You can spot skip-scan by the **`Index Searches`** field in `EXPLAIN ANALYZE`: for ordinary index access it equals `1`, and for skip-scan it's greater than one (the index was "searched" once per leading-column value). In our demo, `Index Searches: 9` versus `Index Searches: 1` on the ordinary queries — that's the fingerprint of skip-scan.

> ⚠️ Skip-scan **softens** the left-prefix rule but doesn't repeal it. It pays off only with **low** cardinality of the leading column: iterating 4 categories is cheap, but 100,000 customers is already more expensive than a `Seq Scan`. The planner decides on cost; betting on "PG18 will figure it out" instead of the right column order is a bad bet.

## How a composite index is laid out

The index `(category, price)` is physically sorted by `category` first, and only within the same category — by `price`. That's the whole mechanic of the left prefix:

```
Index (category, price): rows sorted by category (alphabetical),
and within one category — by price.

  bakery │  1, 2, …, 503     ┐
  coffee │  1, 2, …, 503     │  WHERE category=… → one whole block       ✓ left prefix
  cold   │  1, 2, …, 503     │  WHERE price=250  → one row 250 in each
  tea    │  1, 2, …, 503     ┘  block, scattered across the whole index  ✗
```

This is the "dictionary by word length": words are grouped by length (`category`), and only within a length — alphabetically (`price`). Know the length — you open the right block at once; know only the letter (`price`) but not the length — you have to leaf through every block. The three query forms against an index `(a, b)`:

| Query against `(a, b)` | What the index `(category, price)` does | `Index Searches` |
|---|---|---|
| `WHERE a = … AND b = …` | descend by `category`, then by `price`; both in `Index Cond` | 1 |
| `WHERE a = …` (left prefix) | rows with that `category` sit contiguously; `Index Cond` on `category` | 1 |
| `WHERE b = …` (second only) | rows scattered across each `category`; classically `Seq Scan`, in PG18 skip-scan iterates the leader's values | > 1 (9 here) |

## What our code shows

`demo.sql` builds a lab table `menu_lab` (200,000 rows, 4 categories, prices `1..503` — independent of category) with an index on `(category, price)` and explains three queries:

```sql
-- Q1: both columns → Index Cond on both, Index Searches: 1
SELECT * FROM menu_lab WHERE category = 'tea' AND price = 250;
-- Q2: left prefix → Index Cond on category, Index Searches: 1
SELECT * FROM menu_lab WHERE category = 'tea';
-- Q3: second column only → skip-scan, Index Searches: 9
SELECT * FROM menu_lab WHERE price = 250;
```

All three use the same index `menu_lab_cat_price_idx` — but Q3 takes it only thanks to skip-scan, and that shows in `Index Searches`.

## Running it

```sh
docker compose up -d
make lecture L=06-indexing-and-explain/06-02-btree-and-composite-column-order
```

Output:

```
== Q1) фильтр по ОБОИМ столбцам (category=tea AND price=250) — оба в Index Cond ==
                                   QUERY PLAN                                   
--------------------------------------------------------------------------------
 Bitmap Heap Scan on menu_lab (actual rows=100.00 loops=1)
   Recheck Cond: ((category = 'tea'::text) AND (price = 250))
   Heap Blocks: exact=100
   ->  Bitmap Index Scan on menu_lab_cat_price_idx (actual rows=100.00 loops=1)
         Index Cond: ((category = 'tea'::text) AND (price = 250))
         Index Searches: 1


== Q2) левый префикс: только лидирующий столбец (category=tea) — индекс работает ==
                                    QUERY PLAN                                    
----------------------------------------------------------------------------------
 Bitmap Heap Scan on menu_lab (actual rows=50000.00 loops=1)
   Recheck Cond: (category = 'tea'::text)
   Heap Blocks: exact=1471
   ->  Bitmap Index Scan on menu_lab_cat_price_idx (actual rows=50000.00 loops=1)
         Index Cond: (category = 'tea'::text)
         Index Searches: 1


== Q3) только ВТОРОЙ столбец (price=250): до PG18 — Seq Scan, в PG18 — skip-scan ==
                                   QUERY PLAN                                   
--------------------------------------------------------------------------------
 Bitmap Heap Scan on menu_lab (actual rows=398.00 loops=1)
   Recheck Cond: (price = 250)
   Heap Blocks: exact=398
   ->  Bitmap Index Scan on menu_lab_cat_price_idx (actual rows=398.00 loops=1)
         Index Cond: (price = 250)
         Index Searches: 9
```

(The demo prints in Russian.) Q1 and Q2 take the index in a single search (`Index Searches: 1`): "both columns" and "leading column only" are both the left prefix. Q3 filters on the second column `price` without `category` — there would have been a `Seq Scan` before, but PG18 iterated the 4 categories via skip-scan, and `Index Searches: 9` gives it away. Same index, different access cost.

## The fence

What we simplified:

- **`category` first — for the demo.** We made it the leader because skip-scan needs a low-cardinality leader. In production column order is chosen for the real queries: first goes the column you almost always filter by equality; a range column (`price > X`, `created_at BETWEEN ...`) goes last — after a range the index "fans out" and the following columns no longer narrow the search.
- **Skip-scan is a safety net, not a replacement for design.** At high leader cardinality it loses to a `Seq Scan`. You shouldn't rely on it instead of a dedicated index on `(price)`.
- **An index has a write and storage cost.** Every extra index slows down `INSERT`/`UPDATE` and takes disk, so "an index for every case" isn't free (on maintenance cost and `CREATE INDEX CONCURRENTLY` — see 06-06).

Which indexes a cluster actually needs under real load, how to catch unused ones (`pg_stat_user_indexes`) and duplicates — that's your DBA's dashboard; your job as a developer is to **pick the column order for your queries and verify it in `EXPLAIN`**.

## Takeaways

- A composite index `(a, b)` is sorted by `a`, then by `b` — hence the left-prefix rule.
- It serves `WHERE a` and `WHERE a AND b`; `WHERE b` alone is classically a `Seq Scan`.
- Column order: first the one you filter by equality and almost always; the range column last.
- PG18 skip-scan pulls the index in even for the second column alone at **low** leader cardinality — visible by `Index Searches > 1`; it softens the rule but doesn't repeal it.
- Every index costs writes and storage — "an index for everything" isn't free.

Next up — **06-03 "When indexes don't help"**: why an index on `email` stays silent for `WHERE lower(email) = ...`, what a non-sargable condition is, and how an expression index fixes it.
