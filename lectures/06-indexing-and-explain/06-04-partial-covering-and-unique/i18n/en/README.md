# 06-04 — Partial, covering, and unique indexes

A Brew worker asked once a second: "give me the next unprocessed orders": `WHERE status = 'pending' ORDER BY id LIMIT 100` — and it crawled. You bring a migration to review: an ordinary index on `status`, so the database finds `pending` by the index rather than by a scan. Pavel looks at the diff and answers in one line:

> **Pavel (in review):** a million done rows in the index. why.

There are always few orders in `pending` — dozens out of a million, the rest long since `done`. And an index on `status` would hold **all** rows, including the useless million `done` ones: it would be huge, eat memory, and be slow to update on every status change. Alongside, a second bill: a customer dashboard pulled `SELECT customer_id, total ... WHERE customer_id = ?`, and for every row the index also sent the database into the table itself for the `total` field, even though all it really needed was two numbers.

Both scenarios are about an ordinary index taking **too much**: extra rows or extra trips into the table. The goal of this unit is three indexes that take exactly as much as needed: **partial** (only the rows that matter), **covering** (carries extra columns inside itself → Index-Only Scan, no trip to the table), and **unique** (which also guarantees uniqueness).

## Partial index: index only the rows you need

`CREATE INDEX ... WHERE <condition>` builds an index **only over the rows** satisfying the condition. For a "pending queue" that's ideal: an index on `(id) WHERE status = 'pending'` contains only the pending rows — it's tens of times smaller than a full index, updates faster (`done` rows never touch it), and serves the hot query directly.

A query uses a partial index if its `WHERE` **implies** the index's condition. `WHERE status = 'pending' ORDER BY id` goes through our index; `WHERE status = 'done'` does not (different rows, simply not in the index). Partial indexes shine exactly where a query always looks at a narrow subset: active records, unpublished, unpaid.

## Covering index: Index-Only Scan with no trip to the table

An ordinary `Index Scan` works in two steps: find rows in the index, then visit the table (heap) for the remaining columns. If **all** the needed columns are already in the index, the second step is unnecessary — that's an **Index-Only Scan**: the answer is assembled straight from the index.

`INCLUDE` adds "payload" to the index — columns you don't search by but want to read:

```sql
CREATE INDEX orders_lab_cust_cover_idx ON orders_lab (customer_id) INCLUDE (total);
```

The key is `customer_id` (we search and sort by it), `total` rides along. The query `SELECT customer_id, total WHERE customer_id = ?` is fully **covered** by the index → Index-Only Scan, `Heap Fetches: 0` (not a single trip to the table). `Heap Fetches` is precisely the counter for "how many times we had to go to the heap after all."

> ⚠️ `Heap Fetches: 0` is achieved not by index magic but by the **visibility map**: an Index-Only Scan can skip the heap only for pages marked "all-visible," and `VACUUM` is what marks them. On a freshly written table, before `VACUUM`, the same plan will show `Heap Fetches > 0`. So in the demo we call `VACUUM` before the measurement — in production autovacuum does this.

> **Botyr:** Why hold back — let's shove every column into `INCLUDE`, and never touch the table again.
>
> **Dmitry:** You just bloated the index back.

## Unique index

A unique index (`CREATE UNIQUE INDEX`, or what Postgres builds under a `UNIQUE` constraint and `PRIMARY KEY`) serves two goals at once: it **guarantees uniqueness** (inserting a duplicate fails with `23505`) and at the same time **speeds up equality search** — it's an ordinary B-tree that an `Index Scan` runs over. So `UNIQUE (email)` isn't only an integrity rule from module 02, but also a ready-made index for `WHERE email = ?`; a separate index on `email` alongside it would be a duplicate.

## One step or two: where the index goes

A regular `Index Scan` works in two steps; a covering index removes the second:

```
A regular Index Scan — two steps:
  [ index (key) ] —find rows→ [ table (heap) ] —read the remaining columns→ answer

Index-Only Scan — all needed columns are already in the index, the second step is gone:
  [ index: key + INCLUDE ] —answer assembled right here→ ✓   Heap Fetches: 0
  we do NOT go to the heap, if the page is all-visible by the visibility map (VACUUM sets it)
```

Three index forms — each takes exactly as much as needed:

| Index | What it does | When to use | Plan signal |
|---|---|---|---|
| **Partial** `… WHERE condition` | indexes only the rows implying the condition | the query always looks at a narrow subset (pending queue, active) | `Index Scan`, index many times smaller |
| **Covering** `… INCLUDE (cols)` | carries the readable columns in the index itself | `SELECT` takes just a couple of columns by key | `Index Only Scan`, `Heap Fetches: 0` |
| **Unique** `UNIQUE` / `PK` | guarantees uniqueness + speeds up equality search | the column must be unique (`email`, `sku`) | `Index Scan`; inserting a dup → `23505` |

## What our code shows

`demo.sql` builds `orders_lab` (200,000 orders, 1% in `pending`) and shows:

```sql
-- A) a partial index on pending — tiny, and serves the queue
CREATE INDEX orders_lab_pending_idx ON orders_lab (id) WHERE status = 'pending';
-- size: 64 kB versus 4408 kB for the full index (the PK over all rows)
SELECT id, total FROM orders_lab WHERE status = 'pending' ORDER BY id LIMIT 5;   -- Index Scan

-- B) a covering index → Index-Only Scan
CREATE INDEX orders_lab_cust_cover_idx ON orders_lab (customer_id) INCLUDE (total);
VACUUM (ANALYZE) orders_lab;
SELECT customer_id, total FROM orders_lab WHERE customer_id = 777;   -- Index Only Scan, Heap Fetches: 0
```

## Running it

```sh
docker compose up -d
make lecture L=06-indexing-and-explain/06-04-partial-covering-and-unique
```

Output:

```
== A1) частичный индекс много меньше полного (PK по всем строкам) ==
 full_pk_idx | partial_idx | partial_is_smaller 
-------------+-------------+--------------------
 4408 kB     | 64 kB       | t


== A2) частичный индекс обслуживает "разгрести pending по порядку id" ==
                                       QUERY PLAN                                       
----------------------------------------------------------------------------------------
 Limit (actual rows=5.00 loops=1)
   ->  Index Scan using orders_lab_pending_idx on orders_lab (actual rows=5.00 loops=1)
         Index Searches: 1


== B) покрывающий индекс INCLUDE → Index Only Scan, Heap Fetches: 0 ==
                                         QUERY PLAN                                         
--------------------------------------------------------------------------------------------
 Index Only Scan using orders_lab_cust_cover_idx on orders_lab (actual rows=200.00 loops=1)
   Index Cond: (customer_id = 777)
   Heap Fetches: 0
   Index Searches: 1
```

(The demo prints in Russian.) The partial index on `pending` — 64 kB versus 4408 kB for the full index on `id`: it holds only 1% of the rows (`partial_is_smaller = t`) and serves the queue query with an ordinary `Index Scan`. The covering index with `INCLUDE (total)` returns `customer_id` and `total` straight from the index: `Index Only Scan`, `Heap Fetches: 0` — it never visited the table.

## The fence

What we simplified:

- **`Heap Fetches: 0` rests on a fresh `VACUUM`.** In a live table, between vacuums some pages are "dirty," and an Index-Only Scan still dips into the heap — the win is real but not absolute, and it depends on the autovacuum frequency your DBA tunes.
- **`INCLUDE` bloats the index** (carries extra columns) — that trades disk and write speed for the speed of one query; you shouldn't dump everything into `INCLUDE`.
- **A partial index pays off only if queries really look into its subset.** If the index condition and the query condition diverge even slightly, the index won't be used; that's worth checking in `EXPLAIN`.
- **"Which index earns its keep under load," catching unused ones (`pg_stat_user_indexes`), the "extra index vs write speed" balance** in a large system — that's maintenance your DBA runs.

Your job is to **match the index shape to the query shape**: a narrow subset → partial, "I only need a couple of columns" → covering, "it must be unique" → unique.

## Takeaways

- A partial index (`... WHERE condition`) indexes only the rows that matter — small, fast to update, ideal for "active"/"queue"; the query must imply the index's condition.
- A covering index (`... INCLUDE (cols)`) carries the readable columns inside it → **Index-Only Scan** with no trip to the table (`Heap Fetches: 0`).
- `Heap Fetches: 0` is provided by the visibility map, which `VACUUM` sets — before vacuuming there will be heap trips.
- A unique index (`UNIQUE`/`PK`) both guarantees uniqueness and speeds up equality search — a separate index alongside would be a duplicate.
- `INCLUDE` bloats the index, partial saves space: both trade space/writes for query speed.

Next up — **06-05 "GIN for jsonb and arrays"**: why a B-tree is no good for `@>`/searching inside jsonb and arrays, and how the GIN index takes that on.
