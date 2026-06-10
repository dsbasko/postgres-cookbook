# 06-01 — Reading EXPLAIN ANALYZE

The order-status dashboard in Brew's admin panel opened instantly — while the register's event table was small. At a million rows the same page took two or three seconds to load, and under the evening rush the whole section hung. The backend developer stared at the query — `SELECT * FROM events WHERE ref_no = ?` — and couldn't see it: the query is trivial, one row out. The problem wasn't the query but **how** the database ran it: with no index on `ref_no`, it had to read and check every one of a million rows to find the single match. One command makes this visible — `EXPLAIN ANALYZE`.

The goal of this unit is to learn to **read a query plan** and spot exactly that difference: "read the whole million" versus "go straight to the one row we need." This is the first unit of the indexing module, and it's about the tool we'll use through the rest of it. The specifics of indexes (composite, partial, GIN) come later; here is the alphabet: what each plan node means, where to find the number of rows processed, and why in PG18 the buffers show up right under the plan.

## EXPLAIN, EXPLAIN ANALYZE, and plan nodes

`EXPLAIN <query>` shows the **plan** — what the planner *intends* to do, with cost and row estimates, but it doesn't run the query. `EXPLAIN ANALYZE <query>` actually **runs** it and annotates each node with real numbers: how many rows passed, how many times the node ran, how long it took. Estimates can lie; the facts don't — so for "why is it slow" you always reach for `ANALYZE`.

> ⚠️ `EXPLAIN ANALYZE` really **executes** the query. With a `SELECT` that's safe, but `EXPLAIN ANALYZE DELETE ...` will actually delete rows. To inspect the plan of a writing query without consequences, wrap it and roll back: `BEGIN; EXPLAIN ANALYZE UPDATE ...; ROLLBACK;`.

A plan is a tree of nodes, read **inside-out and bottom-up**: lower nodes feed rows to higher ones. Two leaf source nodes matter to us right now:

- **Seq Scan** (sequential scan) — read the whole table, row by row, checking the condition. If under a single-row filter you see a `Seq Scan` on a big table, that's "read the whole million."
- **Index Scan** — descend the index straight to the matching rows. Below it sits `Index Cond` — the condition the index used to pick rows without touching the rest.

## Rows, buffers, and time

On an `EXPLAIN ANALYZE` node line, look at three things:

- **`actual rows`** — how many rows the node actually returned (in PG18 printed with two decimals: `rows=1.00`). Next to it, on scans, you often see **`Rows Removed by Filter`** — how many rows the node read and **threw away** by the filter. A million rows discarded for one match is a precise measure of wasted work.
- **`Buffers`** — how many 8 KB pages the node touched: `shared hit` — found in cache, `shared read` — read from disk. **In PostgreSQL 18, `EXPLAIN ANALYZE` prints `Buffers` by default** — previously you needed an explicit `EXPLAIN (ANALYZE, BUFFERS)`. Buffers are the most honest measure of "how much data we churned": they don't depend on how busy the CPU was at measurement time.
- **`actual time`** — time per node in milliseconds (`first_row..last_row`), plus the overall `Execution Time` at the bottom.

Time and buffers depend on hardware and cache warmth, so in "Running it" below we deliberately mute them and show only **the plan shape and row count** — which reproduce verbatim on any machine. But here is what the full `EXPLAIN (ANALYZE)` output looks like on our machine (your numbers will differ):

```
 Seq Scan on events_lab  (cost=0.00..19853.00 rows=1 width=25) (actual time=15.189..19.830 rows=1.00 loops=1)
   Filter: (ref_no = 762312)
   Rows Removed by Filter: 999999
   Buffers: shared hit=7353
 Planning Time: 0.059 ms
 Execution Time: 19.839 ms
```

7353 pages in cache, a million rows discarded, ~20 ms — versus the index variant:

```
 Index Scan using events_lab_ref_no_idx on events_lab  (...) (actual time=0.035..0.036 rows=1.00 loops=1)
   Index Cond: (ref_no = 762312)
   Index Searches: 1
   Buffers: shared hit=4 read=3
 Execution Time: 0.044 ms
```

7 pages instead of 7353, zero rows discarded, ~0.04 ms. That's the same difference Brew's dashboard felt. (`Index Searches: 1` is also new in PG18: how many times the index had to be searched anew.)

## The plan is a tree: read it inside-out

Plan nodes nest inside one another. A leaf (a table scan) feeds rows to its parent, which feeds its own parent, up to the root. So a plan is read **inside-out, bottom-up**: first where the rows came from, then what was done with them. Here is the shape of a typical plan with a join — not our query (ours is a one-node tree), but the structure, so the reading order is visible:

```
Aggregate                          ← ③ folded rows into groups
  ->  Hash Join                    ← ② joined two sources
        ->  Seq Scan on events_lab ← ① read the whole table       (leaf)
        ->  Index Scan on shops    ← ① fetched rows by index      (leaf)

Read bottom-up: ① leaf scans → ② join → ③ aggregate.
A parent's time and buffers already include its children.
```

Our demo query is the simplest tree — a **single** node: `Seq Scan` (or, after the index, `Index Scan`). The skill is the same: find the leaves, walk up to the root. What each field under a node means:

| Node / field | What it means | What to watch |
|---|---|---|
| `Seq Scan` | reads the whole table, row by row | on a big table under a point filter — wasted work |
| `Index Scan` | descends the index straight to the rows | what you want for a point lookup |
| `Index Cond` | the condition the index used to pick rows | work went into the index, not a scan |
| `Filter` | condition checked after the row is read | rows are read, then thrown away |
| `Rows Removed by Filter` | rows read and discarded by the filter | a direct measure of wasted work |
| `actual rows` | rows the node actually returned | compare against the planner's estimate |
| `Buffers` (`shared hit`/`read`) | 8 KB pages touched (cache / disk) | the honest measure of data churned |
| `Index Searches` | how many times the index was searched anew (PG18) | usually 1 for a point lookup |

## What our code shows

The lesson is in `demo.sql`. It builds a lab table `events_lab` of a million rows (we don't touch the Brew base tables), gathers statistics with `ANALYZE`, and explains the same query twice — before and after `CREATE INDEX`:

```sql
-- 1) without an index
EXPLAIN (ANALYZE, COSTS OFF, TIMING OFF, SUMMARY OFF, BUFFERS OFF)
SELECT * FROM events_lab WHERE ref_no = 762312;     -- → Seq Scan, Rows Removed by Filter: 999999

CREATE INDEX events_lab_ref_no_idx ON events_lab (ref_no);
ANALYZE events_lab;

-- 2) with the index
EXPLAIN (ANALYZE, COSTS OFF, TIMING OFF, SUMMARY OFF, BUFFERS OFF)
SELECT * FROM events_lab WHERE ref_no = 762312;     -- → Index Scan, Index Cond
```

The `(... TIMING OFF, BUFFERS OFF)` options strip everything machine-dependent — leaving the plan shape and actual rows. We turn parallelism off (`max_parallel_workers_per_gather = 0`) so the plan reads as a single column instead of splitting into `Gather` + workers (Postgres can parallelize big scans — but that's not a first-lesson topic).

## Running it

```sh
docker compose up -d
make lecture L=06-indexing-and-explain/06-01-reading-explain-analyze-buffers
```

Output:

```
== 1) БЕЗ индекса: запрос идёт Seq Scan по всему миллиону строк ==
                    QUERY PLAN                     
---------------------------------------------------
 Seq Scan on events_lab (actual rows=1.00 loops=1)
   Filter: (ref_no = 762312)
   Rows Removed by Filter: 999999


== создаём индекс по ref_no и пересобираем статистику ==

== 2) С индексом: тот же запрос — Index Scan точно в одну строку ==
                                   QUERY PLAN                                    
---------------------------------------------------------------------------------
 Index Scan using events_lab_ref_no_idx on events_lab (actual rows=1.00 loops=1)
   Index Cond: (ref_no = 762312)
   Index Searches: 1
```

(The demo prints in Russian.) Without an index — `Seq Scan` and `Rows Removed by Filter: 999999`: the database read a million rows to return one. After `CREATE INDEX` the same query runs as an `Index Scan` with `Index Cond: (ref_no = 762312)` — the index picked the row directly, nothing to discard. Time and buffers are stripped here for reproducibility (the full output with them is in the section above).

## The fence

What we simplified:

- **Perfect selectivity.** We turned parallelism off and showed a query that returns one row out of a million — there the index always wins. In real life selectivity varies: a query returning half the table will be run as a `Seq Scan` by the planner **on purpose** — reading half the table by jumping around the index is more expensive than reading it sequentially. That's the right call, not "the index didn't work."
- **One measurement, not a diagnosis.** The numbers from `ANALYZE` are one run on a specific machine with a specific cache state. A "cold" run (the first after startup) and a "warm" run give different `Buffers`/`time`, so in production you look at a plan several times and compare its **shape**, not individual milliseconds.
- **EXPLAIN is about the query, not the database.** It answers "how did THIS query run," not "is the database healthy overall." System views (`pg_stat_statements` — which queries eat the most time in aggregate), autovacuum, table bloat, the cache hit ratio across the whole database — that's a dashboard your DBA holds.

The course boundary: your job is to **be able to explain your own query and spot wasted work in the plan**; server tuning and cluster monitoring are beyond it.

## Takeaways

- `EXPLAIN` shows the plan (estimates), `EXPLAIN ANALYZE` runs the query and gives **facts**; for "why is it slow" always use `ANALYZE`.
- A plan reads inside-out; `Seq Scan` = "read the whole table," `Index Scan` + `Index Cond` = "went straight to the rows."
- A node's key numbers: `actual rows`, `Rows Removed by Filter` (wasted work), and `Buffers` (pages touched; **printed by default in PG18**).
- Time and buffers are machine-dependent — compare the **plan shape and row count**, not individual milliseconds.
- `EXPLAIN ANALYZE` really executes the query: inspect writing commands inside `BEGIN; ... ROLLBACK;`.

Next up — **06-02 "B-tree and column order in a composite index"**: why an index on `(a, b)` helps a query on `a` and on `a AND b`, but not always on `b` alone — and what PG18 skip-scan changes about that.
