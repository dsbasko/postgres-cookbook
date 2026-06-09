# 08-03 — lag/lead and window frames (ROWS vs RANGE)

A Brew analyst is assembling a "revenue by day" dashboard. Two columns were requested by the owner personally: "day-over-day delta" — how much the till is up or down versus yesterday, and a "3-day smoothed trend" — so a one-off spike doesn't scare anyone. The first is computed from the "value of the previous row", the second from the "average over the three rows back". Across the week of February 1–7 everything looks smooth — until someone notices that for February 6 and 7 the "3-day trend" shows different numbers in two different versions of the report. One version gives 146.67, the other 175.00. The figures diverge right after February 5, and on February 5 the shop was closed: a day off, and the row for that day simply isn't in the table.

A hole in the series is exactly where "three rows back" stops meaning "three days back". This unit is about how Postgres answers the question "which rows fall into the window", and why there are actually two answers.

## lag and lead — neighbours within the window

`lag(cents)` returns the value of `cents` from the previous row of the window, `lead(cents)` from the next one. "Previous" and "next" are defined by the `ORDER BY` inside `OVER (...)`: order by `day` and the neighbours are yesterday and tomorrow. The day-over-day delta is then simply `cents - lag(cents)`.

At the edges of the series there is no neighbour: the very first row has no previous one, so `lag` is `NULL` there; the very last row has no next one — `NULL` for `lead`. That is not an error but an honest "nothing to the left/right". In `query.sql` we cast the result to `text` and use `coalesce` to substitute `'—'` for `NULL`, so that Go receives a clean `string` rather than a nullable type that would have to be unwrapped on every row.

## The window frame — which rows count as "around the current one"

When an aggregate (`avg`, `sum`, `count`) sits inside `OVER (...)`, it is computed not over the whole window but over a *frame* — a subset of rows relative to the current one. The frame can be specified two ways, and that is the whole plot.

`ROWS BETWEEN 2 PRECEDING AND CURRENT ROW` means "the current row and two physical rows before it", exactly three rows in `ORDER BY` order. Rows are counted; what is in their `day` and whether there are gaps between them is irrelevant to the frame.

`RANGE BETWEEN INTERVAL '2 days' PRECEDING AND CURRENT ROW` means "all rows whose date falls in the range [day−2, day]". Here it is not a count of rows but the *value* of `ORDER BY` landing in the window that matters. If some day in that range is missing from the table, it simply isn't in the calculation — the frame for that row just narrows.

On a smooth series with no gaps both variants coincide: three consecutive days are both three rows and a three-day range. The divergence appears exactly where the hole appears.

That very hole on February 5 — in one picture. The till was closed that day, so there's no row for it:

```
   01.02   02.02   03.02   04.02   ·····   06.02   07.02
    100     120      90     150    (none)   200     110
                                     ↑ hole in the series

  Current row — 06.02. The two frames read "three days back" differently:
   ROWS  2 PRECEDING    → 3 rows in a row: 03, 04, 06        → avg(90, 150, 200) = 146.67
   RANGE '2 days' PREC.  → dates in [04.02 … 06.02]: 04, 06    → avg(150, 200)    = 175.00
```

`ROWS` counts rows and steps over the hole without noticing; `RANGE` counts by date and so loses February 5, which isn't there — the window narrows. The same fork as a table:

| | `ROWS` | `RANGE` |
|---|---|---|
| Counts | physical rows | the `ORDER BY` value |
| "2 PRECEDING" means | two rows back | everything within the value range |
| A hole in the series | doesn't notice it, takes the neighbouring row | narrows the window (the missing date isn't there) |
| Type in `ORDER BY` | any | sortable (date/number/timestamp) |
| Cost | cheaper | pricier: bounds are found by value |
| When to use | "last N events" (position) | "over N calendar days" (time) |

## What our code shows

The heart of the lesson is `query.sql`. The first query builds "day-over-day" via `lag`/`lead`:

```sql
-- name: DayOverDay :many
SELECT
    day::text AS day,
    cents,
    coalesce((lag(cents) OVER (ORDER BY day))::text, '—')            AS prev,
    coalesce((cents - lag(cents) OVER (ORDER BY day))::text, '—')    AS delta,
    coalesce((lead(cents) OVER (ORDER BY day))::text, '—')           AS next
FROM daily_revenue_lab
ORDER BY day;
```

The second computes the same "current day and two preceding" moving average with two frames at once, so they can be placed in adjacent columns:

```sql
-- name: MovingAverage :many
SELECT
    day::text AS day,
    cents,
    round((avg(cents) OVER (ORDER BY day ROWS  BETWEEN 2 PRECEDING AND CURRENT ROW))::numeric, 2)::text AS ma_rows,
    round((avg(cents) OVER (ORDER BY day RANGE BETWEEN INTERVAL '2 days' PRECEDING AND CURRENT ROW))::numeric, 2)::text AS ma_range
FROM daily_revenue_lab
ORDER BY day;
```

`schema.sql` creates a lab table `daily_revenue_lab` (date + cents) with February 5 deliberately missing — the Brew canon with its three orders is no good for a smooth time series, so the table is its own and the canon stays untouched. `cmd/demo/main.go` is thin: it opens the pool, calls `DayOverDay` and `MovingAverage`, and prints both tables via `tabwriter`. The average is wrapped in `round(..., 2)`, so the text is deterministic and matches what is pasted into `## Running it` below.

## Running it

```sh
docker compose up -d
make lecture L=08-analytical-and-lateral/08-03-lag-lead-and-frames T=db-reset
make lecture L=08-analytical-and-lateral/08-03-lag-lead-and-frames
```

`T=run` is the default target, so the second call runs it. From inside the unit directory the same steps are shorter: `make db-reset`, then `make run`.

(The demo prints in Russian.)

```
1) lag/lead — день-к-дню (prev/next = '—', где соседа нет):
ДЕНЬ        выручка  вчера  дельта  завтра
2025-02-01  100      —      —       120
2025-02-02  120      100    20      90
2025-02-03  90       120    -30     150
2025-02-04  150      90     60      200
2025-02-06  200      150    50      110
2025-02-07  110      200    -90     —

2) Скользящее среднее за 3 дня — ROWS vs RANGE (расходятся после пропуска 05.02):
ДЕНЬ        выручка  ma_rows  ma_range
2025-02-01  100      100.00   100.00
2025-02-02  120      110.00   110.00
2025-02-03  90       103.33   103.33
2025-02-04  150      120.00   120.00
2025-02-06  200      146.67   175.00
2025-02-07  110      153.33   155.00
   → 06 и 07 февраля: ROWS берёт 3 строки подряд, RANGE — только даты в окне 2 дней (05 нет).
```

In the first table you can see the edges at work: for February 1, `вчера` (yesterday) and `дельта` (delta) are `'—'` (no previous row), and for February 7 the `'—'` sits in `завтра` (tomorrow). Otherwise `delta = revenue − yesterday` exactly as the owner ordered: +20, −30, +60, +50, −90.

The second table is the fork in the road. Up to and including February 4, `ma_rows` and `ma_range` coincide: the series is smooth, no hole yet. But after February 5 goes missing they diverge. On February 6, `ma_rows = avg(90, 150, 200) = 146.67` — three consecutive physical rows (Feb 03, 04, 06), `ROWS` does not think about dates. And `ma_range = avg(150, 200) = 175.00` — only the dates that fall into the window [04, 06]: February 4 and 6, because February 5 is not in the table. February 7 is the same story: `ma_rows = avg(150, 200, 110) = 153.33` (three rows: 04, 06, 07), while `ma_range = avg(200, 110) = 155.00` — the window is [05, 07], but February 5 is missing, so only the 6th and 7th made it into the calculation. The analyst computed a "3-day average", and `ROWS` gave them a "3-row average" — in a week with a day off those are different things.

## The fence

- `ROWS` is blind to the gaps: "two rows back" is two rows back, even if there's a week-long void between them. `RANGE` counts by the `ORDER BY` value and is therefore correct on a gappy series. Practical rule: a "moving average over N calendar days" in finance and analytics is almost always wanted as `RANGE` over a date — so weekends and gaps don't inflate the window with extra rows; `ROWS` fits where position matters (the last 3 events, whatever their dates).
- `RANGE` pays for its correctness: it needs a sortable type in `ORDER BY` (date, number, timestamp — not just anything), and it costs more, because for each row it finds the window bounds by value rather than by a row counter.
- A beginner's surprise: the default frame. `ORDER BY` in `OVER (...)` with no explicit frame is `RANGE BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW`. That's why `sum(...) OVER (ORDER BY ...)` back in 08-01 already produced a running total even though we said nothing about frames then. Get into the habit of checking: if an aggregate over `ORDER BY` behaves cumulatively when you didn't intend it to, the default `RANGE UNBOUNDED PRECEDING` kicked in.
- Your DBA looks at the frame as a cost: on large series a window aggregate is a sort by `ORDER BY` plus a row buffer for the frame, and that cost shows up in the plan (`EXPLAIN`, module 06) as a separate `WindowAgg` node. The memory for that buffer, and whether to keep it in `work_mem` or spill it to disk, is their concern, not the query's.

## What to take away

`lag`/`lead` are window neighbours: the previous and next row in `ORDER BY` order, and at the edges of the series there is no neighbour and `NULL` arrives (we substituted `'—'` for it). The window frame decides which rows fall into the aggregate around the current one: `ROWS` counts physical rows, `RANGE` counts by the `ORDER BY` value. On a smooth series they give the same thing, but on holes they diverge — and a "3-day average" is almost always wanted as `RANGE`. And remember the default: `ORDER BY` with no explicit frame is `RANGE UNBOUNDED PRECEDING`, that is, an invisible running total.

So far we have navigated rows that lie side by side in one flat series. But data is sometimes nested: a tree of categories, a chain "order → refund → re-issue", a hierarchy of employees. To walk such a structure in depth, windows are no longer enough — you need a query that references itself. The next unit, 08-04, is about exactly that: recursive CTEs, how `WITH RECURSIVE` traverses a tree level by level, and where such a traversal gets its safety catch against an infinite loop.
