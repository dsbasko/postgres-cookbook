# 08-06 — GROUPING SETS, ROLLUP, and CUBE: subtotals in one pass

The Brew sales dashboard needs to show three numbers on one screen: revenue per shop, revenue per drink category, and the grand total across the whole chain. The analyst sits down with SQL and writes three separate `SELECT`s — one with `GROUP BY shop`, one with `GROUP BY category`, and one with no grouping at all — then glues them together with `UNION ALL`. And that's where the pain starts: the queries have different column counts, you have to pad with `NULL` placeholders to line up the `SELECT` lists, it's easy to swap the order and end up with a "category total" landing in the shop column. Three passes over the table, three places to get it wrong, and a report nobody wants to touch.

Postgres can compute all of those slices — leaves, subtotals, and the grand total — in a single pass of a single query. The `GROUP BY` extensions: `ROLLUP`, `CUBE`, and `GROUPING SETS`.

## Plain GROUP BY gives only the leaves

`GROUP BY (shop, category)` groups by each pair of values and returns one row per combination: `(Central, coffee)`, `(Central, tea)`, `(North, coffee)`, `(North, tea)`. Those are the "leaves" — the most detailed cells. Plain `GROUP BY` produces no "total for Central" or "total for the chain": to get them you need another query with coarser grouping. That's exactly where those three `UNION ALL`ed `SELECT`s come from.

## ROLLUP adds hierarchical subtotals

`GROUP BY ROLLUP (shop, category)` computes the leaves — and on top of them adds subtotals over the prefixes of the column list, working right to left. For `(shop, category)` that means: each shop gets an extra row where `category` is rolled up (subtotal per shop), plus one row where both columns are rolled up (grand total). The hierarchy reads as a tree: shop → its categories, and at the root the total over everything. A rolled-up column in a subtotal row arrives as `NULL`; in the demo we label such levels with `coalesce(shop, '— все —')`.

## CUBE adds every combination of rollup

`ROLLUP` only rolls up along the "left to right" hierarchy: it won't give you a subtotal per category on its own (where `shop` is rolled up but `category` is not). `GROUP BY CUBE (shop, category)` covers that: it computes **every** combination of which columns are rolled up and which are not. For two columns that's four variants: both expanded (leaves), `category` rolled up (subtotal per shop), `shop` rolled up (subtotal per category — "how much coffee across the whole chain"), both rolled up (grand total). `CUBE` is `ROLLUP` plus exactly those cross-cut category slices.

## GROUPING SETS — exactly the slices you need

`CUBE` is generous: it gives you everything, including the leaves the dashboard doesn't need. When the slices are known in advance, you list them by hand: `GROUP BY GROUPING SETS ((shop), (category), ())`. Each parenthesized element is a separate grouping: `(shop)` — totals per shop, `(category)` — totals per category, `()` — the empty grouping, i.e. the grand total. No leaves, no extra rows — exactly the three slices the report asks for. In fact `ROLLUP` and `CUBE` are sugar over `GROUPING SETS`: they expand into specific sets of groupings, and `GROUPING SETS` lets you write that set directly.

## grouping() tells a subtotal from a real NULL

A rolled-up column in a subtotal row arrives as `NULL`. But `NULL` also occurs in real data — and then you can't tell a total row from a row with a genuine missing value. The `grouping(col)` function settles this: it returns `1` if the column is rolled up in this row (it's a `NULL` subtotal) and `0` if it's a real value. It's also handy for sorting totals to the end of each group: `ORDER BY grouping(shop), shop, grouping(category), category` — first the rows with a real `shop`, then the per-shop subtotal. In the demo we add both flags into a `level` column: `level = grouping(shop) + grouping(category)`: `0` is data/leaf, `1` is a subtotal with one column rolled up, `2` is the grand total (both rolled up).

Two columns → four rollup variants, a 2×2 square. Each `GROUP BY` extension paints its own set of cells:

```
               category expanded         category ROLLED UP
            ┌──────────────────────────┬──────────────────────────┐
 shop       │ LEAVES           level 0 │ subtotal per shop   l. 1 │
 expanded   │ (Central, coffee) 1000   │ (Central, — все —) 1300   │
            ├──────────────────────────┼──────────────────────────┤
 shop       │ subtotal per cat. l. 1   │ GRAND TOTAL       level 2 │
 ROLLED UP  │ (— все —, coffee) 1700   │ (— все —, — все —) 2200   │
            └──────────────────────────┴──────────────────────────┘

  GROUP BY        → top-left only (leaves)
  ROLLUP          → top-left + top-right + bottom-right (no cross-cut bottom-left)
  CUBE            → all four cells
  GROUPING SETS   → exactly the cells you list
```

The same fork as a table:

| Extension | Which slices | Cells of the 2×2 | When to use |
|---|---|---|---|
| `GROUP BY (a, b)` | leaves only | 1 (top-left) | only the detailed cells are needed |
| `ROLLUP (a, b)` | leaves + prefix subtotals + grand total | 3 (no cross-cut) | hierarchy "shop → its categories → total" |
| `CUBE (a, b)` | every rollup combination | 4 (all) | slices across all dimensions needed |
| `GROUPING SETS (…)` | exactly the listed ones | as many as you name | dashboard with a fixed set |

## What our code shows

`query.sql` has three queries over one small fact table `sales_fact_lab` (two shops × two categories, fixed numbers). The heart of the lesson is `RollupByShop`: `ROLLUP` plus `grouping()` to label and sort the totals.

```sql
SELECT
    coalesce(shop, '— все —')                  AS shop,
    coalesce(category, '— все —')              AS category,
    (sum(cents))::bigint                       AS cents,
    (grouping(shop) + grouping(category))::int AS level
FROM sales_fact_lab
GROUP BY ROLLUP (shop, category)
ORDER BY grouping(shop), shop, grouping(category), category;
```

`CubeAllAngles` repeats this with `GROUP BY CUBE (shop, category)`, and `GroupingSetsExplicit` with `GROUP BY GROUPING SETS ((shop), (category), ())`; everything else is identical across all three. `cmd/demo/main.go` is a thin layer: `pgxpool` → `db.New` → three typed calls (`RollupByShop`, `CubeAllAngles`, `GroupingSetsExplicit`) → a `tabwriter` printing shop, category, revenue, and `level`. The `coalesce(..., '— все —')` in SQL labels the rolled-up levels right in the output.

## Running it

Bring up the sandbox (from the repo root) and apply the Brew base schema:

```sh
docker compose up -d
make lecture L=08-analytical-and-lateral/08-06-grouping-sets-rollup-cube T=db-reset
make lecture L=08-analytical-and-lateral/08-06-grouping-sets-rollup-cube
```

(`T=run` is the default. From inside the unit directory it's `make db-reset`, `make run`.)

Output (the demo prints in Russian):

```
1) ROLLUP (shop, category) — листья, подытог по магазину, общий итог:
МАГАЗИН  категория  выручка  уровень
Central  coffee     1000     0
Central  tea        300      0
Central  — все —    1300     1
North    coffee     700      0
North    tea        200      0
North    — все —    900      1
— все —  — все —    2200     2

2) CUBE (shop, category) — плюс подытоги по категории по всей сети:
МАГАЗИН  категория  выручка  уровень
Central  coffee     1000     0
Central  tea        300      0
Central  — все —    1300     1
North    coffee     700      0
North    tea        200      0
North    — все —    900      1
— все —  coffee     1700     1
— все —  tea        500      1
— все —  — все —    2200     2

3) GROUPING SETS ((shop),(category),()) — только нужные срезы:
МАГАЗИН  категория  выручка  уровень
Central  — все —    1300     1
North    — все —    900      1
— все —  coffee     1700     1
— все —  tea        500      1
— все —  — все —    2200     2
```

Compare the three blocks. `ROLLUP` produced the leaves (`Central/coffee 1000` and so on), a subtotal per shop (`Central/— все — 1300`, `North/— все — 900`), and the grand total (`— все —/— все — 2200`). `CUBE` repeated all of that and added exactly two rows — `— все —/coffee 1700` and `— все —/tea 500`: those are the per-category subtotals cross-cut over shops ("how much coffee and how much tea across the whole chain"), which `ROLLUP` does not give. And `GROUPING SETS` dropped the leaves and kept exactly the three listed slices — totals per shop, totals per category, and the grand total: the five rows the dashboard needs, with not one extra.

## The fence

- The main trap is that a `NULL` in a subtotal row is **indistinguishable** from a real `NULL` in the data if the column allows them: without `grouping()` you can't tell whether `NULL` here means "total across all categories" or "this row genuinely had no category." The moment real `NULL`s are possible in a grouped column, the `grouping()` flag (and/or `coalesce` for a label) stops being decoration and becomes mandatory.
- `CUBE` grows as `2^N` combinations in the number of columns: two dimensions is four groupings, five is already thirty-two. Over many dimensions `CUBE` is expensive — for a specific dashboard you're better off with `GROUPING SETS` listing exactly the slices you want.
- This is a one-off "on the fly" summary over a small fact table, not a replacement for a real OLAP cube or for materialized aggregates over large data. When there are many slices, the data is heavy, and the report is hit often, building and regularly refreshing such a report (precomputation, materialized views, a separate analytical database) is the job of an analytics platform and your DBA, not of a single `SELECT` in production OLTP.

## Takeaways

- `GROUP BY (a, b)` gives only the leaves — combinations of values; plain `GROUP BY` computes no subtotals or grand total.
- `ROLLUP (a, b)` adds hierarchical subtotals: per `a` (with `b` rolled up) plus the grand total (both columns rolled up).
- `CUBE (a, b)` adds **every** combination of rollup — including the cross-cut subtotals per `b` across all `a`, which `ROLLUP` lacks.
- `GROUPING SETS ((a), (b), ())` lists exactly the slices you need — no leaves, no extra rows; `ROLLUP` and `CUBE` are sugar over it.
- `grouping(col)` = `1` if the column is rolled up in the row (a `NULL` subtotal) and `0` if it's a real value — that's how you tell a total from a real `NULL` and sort totals to the end.

That wraps up module 08, "Analytics and LATERAL." You've worked through Postgres's whole analytical toolkit: window functions that compute a summary without collapsing rows (`OVER (...)`); ranking (`row_number`, `rank`, `dense_rank`) for "top-N per group"; `lag`/`lead` and window frames for comparing a row with its neighbors and for running totals; recursive CTEs (`WITH RECURSIVE`) for walking trees and graphs; `LATERAL`, where the right-hand source of a `JOIN` sees the current row of the left; and now `GROUPING SETS`/`ROLLUP`/`CUBE` — subtotals and the grand total in a single pass. All of it is about how to **read** data for analytics.

Two weeks later the dashboard is built — the very one Emil ordered at the start of the module. You bring it up on the big screen: every guest's purchases stand in a row, the running total beside each; a separate tile for what sells most; a revenue line by day that doesn't drop to zero on the days a shop wasn't open; and subtotals per shop and category, with the grand total for the chain underneath. Emil comes down to sign off in person; he won't let go of the frame with the receipt for order #1.

> **Emil:** Before, I only knew the total — how much a guest spent. Now I can see HOW he got there. At this cup Alice crossed a thousand — the coupon should have gone here, not half a year later.
>
> **You:** Every purchase in place, the total right beside it.
>
> **Emil:** And the whole chain? It all used to drown in one number.
>
> **You:** Shops, categories, the grand total — here, on one screen.
>
> **Emil:** This shop is growing, that one is quietly sinking. That's exactly what I needed to see.
>
> **Dmitry:** One pass instead of a stack of glued reports. Keep it that way — it won't start lying on the gaps.
>
> **Emil:** Good work. And right away, the next thing: let me load the supplier price list myself — I'm tired of waiting until night for someone else to do it.

The dashboard is signed off — and Emil's last request already turns the team toward the next task.

Next comes module **09, "Writing, events, and server-side logic,"** and the focus shifts from reading to writing. There it's advanced writes: `MERGE` for upsert logic in one statement and `COPY` for bulk loading; a work queue on top of a table via `FOR UPDATE SKIP LOCKED`, so several workers can pull jobs without blocking each other; the transactional outbox pattern, which atomically commits business data and an event for the outside world (that very bridge to Kafka); `LISTEN`/`NOTIFY` for lightweight cross-session notifications; and triggers — server-side logic the database runs itself on insert, update, and delete.
