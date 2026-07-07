# 08-01 — Window functions: the basics (PARTITION BY, ORDER BY, running total)

Morning in the openspace. Evgeny brings a printout — a report titled "how much
each customer spent in total": Alice — 1270, Boris — 730, Karina — 780, one row
per guest. Behind him, rarely seen on the dev floor, comes Emil himself — with
the framed receipt for order #1, where it all began.

> **Evgeny:** Done — a total per customer. Pretty, isn't it?
>
> **Emil:** I don't want to know just the total. I want to see HOW it grew. Did
> Alice spend 1270 over three visits or over thirty? After which purchase did she
> cross a thousand? When should I have sent her a coupon?
>
> **You:** We could run two queries — one for the total, one for the purchases —
> and stitch them together in Go.
>
> **Dmitry:** We could. But there's a way to keep every purchase in place and glue
> the total beside it — in one query. That's today's topic.
>
> **Emil:** And show me only the ones who crossed a thousand. The rest later.

The report holds one row per guest, and the purchases themselves are gone. That's
not Evgeny's fault: that's what `GROUP BY` does — it takes a group of rows and
collapses it into one. To get a per-customer total, we sacrificed the purchases.
Emil needs both at once — every purchase in place AND the total right beside it.
That is exactly what window functions are for.

## An aggregate collapses, a window does not

A window function is the same aggregate you already know: `sum`, `avg`, `count`.
The difference is one extra clause — `OVER (...)`. And that clause changes
everything.

An aggregate with `GROUP BY` works like this: collect rows into groups, compute
one value per group, return one row per group. Brew's seven purchases turn into
three rows — one per customer. The original rows are destroyed; there's no going
back to them.

A window function (`sum(cents) OVER (...)`) does the opposite. It also computes
over a set of rows — that set is called a "window" — but it does NOT collapse it.
Each of the seven purchases stays in its place, and the computed result is glued
onto it as an extra column. Seven rows in, seven rows out, just with a new column.
That's precisely what the owner wanted: purchases visible, total alongside.

## PARTITION BY slices the table into windows

Since a window is a set of rows, for each row we have to decide which neighbors
belong to it. That's the job of `PARTITION BY`.

`sum(cents) OVER (PARTITION BY customer)` says: for each row, the window is all
rows with the same `customer`. For any of Alice's purchases the sum is computed
over all of Alice's purchases and equals 1270; the same number appears in every
one of her rows. Boris has his own window — 730; Karina has hers — 780.
`PARTITION BY` cut the table into non-overlapping windows by customer, and within
each window `sum` was computed independently.

And if we drop `PARTITION BY` entirely? Then `OVER ()` — empty parentheses —
means "one window over the whole table". `sum(cents) OVER ()` adds up all seven
purchases and yields the chain's grand total — 2780 — in every row. That's handy
to keep alongside: the marketer instantly sees what share of total revenue a given
customer drives, without running a second query.

## ORDER BY inside the window turns sum into a running total

So far the per-customer total was identical in all of a customer's rows — 1270 for
Alice here, here, and here. That's a static total. To see HOW it grew (the owner's
question), add `ORDER BY` inside the window.

`sum(cents) OVER (PARTITION BY customer ORDER BY day, id)` reads like this: inside
the customer's window, order the rows by day, and for each row add up `cents` from
the start of the window to the current row inclusive. That's a running total — a
cumulative sum. For Alice it grows 300 → 750 → 1270: you can see she crossed a
thousand on her third purchase. `PARTITION BY customer` resets the accumulation at
the customer boundary — Boris starts his 250 from zero, not picking up Alice's
1270.

Why the second key — `id` — in `ORDER BY`? It's a deterministic tie-break. If a
customer has two purchases on the same day, `ORDER BY day` alone doesn't decide
which comes first — and the accumulation across those two rows could fall any way.
`id` (which is `GENERATED ALWAYS AS IDENTITY`, i.e. insertion order) breaks the
tie unambiguously, and the output reproduces verbatim on every run.

The same difference as a picture — `GROUP BY` collapses, the window keeps:

```
GROUP BY customer                  OVER (PARTITION BY customer)
7 purchases → 3 rows               7 purchases → 7 rows + a total column

Alice  300 ┐                       Alice  300 ┐
Alice  450 ├→ Alice  1270          Alice  450 ├ total 1270 in each of Alice's rows
Alice  520 ┘                       Alice  520 ┘
Boris  250 ┐→ Boris   730          Boris  250 ┐ total 730
Boris  480 ┘                       Boris  480 ┘
Karina 480 ┐→ Karina  780          Karina 480 ┐ total 780
Karina 300 ┘                       Karina 300 ┘
                                   rows in place, total glued on as a column
```

On the left the original purchases are destroyed, three totals remain; on the right all seven rows are intact, with the total written beside them. The window itself is set by the `OVER (...)` clause, which has three forms:

| Form | Window for each row | What's in the column |
|---|---|---|
| `OVER ()` | the whole table | the chain's grand total (2780) beside every row |
| `OVER (PARTITION BY customer)` | rows of the same customer | the customer's static total (1270 in all of Alice's rows) |
| `OVER (PARTITION BY customer ORDER BY day, id)` | from the window start to the current row | a running total (Alice 300 → 750 → 1270) |

## What our code shows

The heart of the lesson is `query.sql`. Three queries show one idea from three
angles. `CustomerTotals` — a plain aggregate for contrast (it collapses).
`WindowTotals` — the same `sum`, but as a window: the per-customer total and the
grand total beside each purchase. `RunningTotal` — add `ORDER BY` inside the window
and get the accumulation.

```sql
-- name: WindowTotals :many
-- The same sum, but as a WINDOW function: each of the 7 purchases stays in place.
SELECT customer,
       day::text AS day,
       cents,
       (sum(cents) OVER (PARTITION BY customer))::bigint AS customer_total,
       (sum(cents) OVER ())::bigint                      AS grand_total
FROM purchases_lab
ORDER BY customer, day, id;

-- name: RunningTotal :many
-- ORDER BY INSIDE the window → cumulative total from window start to current row.
SELECT customer,
       day::text AS day,
       cents,
       (sum(cents) OVER (PARTITION BY customer ORDER BY day, id))::bigint AS running
FROM purchases_lab
ORDER BY customer, day, id;
```

Note: the `ORDER BY` at the end of the query (after `FROM`) and the `ORDER BY`
inside `OVER (...)` are two DIFFERENT orderings. The first sorts the final output
for readability; the second sets the accumulation order inside the window. They
are independent.

`cmd/demo/main.go` is a thin wrapper: it brings up the pool via `pg.NewPool`,
calls the three typed methods from the generated `internal/db/`, and prints the
result through `tabwriter`. All the SQL logic lives in `query.sql`; Go merely lays
the rows out into columns.

## Running it

(The demo prints in Russian.)

```sh
docker compose up -d
make lecture L=08-analytical-and-lateral/08-01-window-basics-partition-order T=db-reset
make lecture L=08-analytical-and-lateral/08-01-window-basics-partition-order
```

`T=run` is the default, so the second command needs no target. From inside the
unit directory this is simply `make db-reset` and `make run`.

```
1) Агрегат GROUP BY — покупки схлопнуты в одну строку на клиента:
КЛИЕНТ  покупок  сумма
Алиса   3        1270
Борис   2        730
Карина  2        780

2) Оконная sum OVER (...) — строки на месте, итоги доклеены колонкой:
КЛИЕНТ  день        сумма  итог клиента  общий итог
Алиса   2025-02-01  300    1270          2780
Алиса   2025-02-03  450    1270          2780
Алиса   2025-02-05  520    1270          2780
Борис   2025-02-02  250    730           2780
Борис   2025-02-04  480    730           2780
Карина  2025-02-01  480    780           2780
Карина  2025-02-06  300    780           2780

3) sum OVER (PARTITION BY customer ORDER BY day) — running total на клиента:
КЛИЕНТ  день        сумма  накоплено
Алиса   2025-02-01  300    300
Алиса   2025-02-03  450    750
Алиса   2025-02-05  520    1270
Борис   2025-02-02  250    250
Борис   2025-02-04  480    730
Карина  2025-02-01  480    480
Карина  2025-02-06  300    780
```

Check the per-customer arithmetic: Alice 300+450+520=1270, Boris 250+480=730,
Karina 480+300=780, grand total 2780. In block 1 those are the totals after the
collapse; in block 3 they are the last row of each window. The numbers match — the
window computed the same thing as `GROUP BY`, but without losing the purchases
themselves.

## The fence

- A window function is computed at a very late stage of the query — AFTER `WHERE`,
  `GROUP BY`, and `HAVING`. The practical consequence is annoying: you cannot filter
  rows by a window function's value directly in `WHERE` — at the moment `WHERE` is
  checked the window isn't computed yet. "Keep only the purchases where the running
  total crossed 1000" can't be written in a single level; for such top-N tasks the
  window result is wrapped in a CTE (`WITH ...`) and filtered from the outside —
  that's the next unit's job.
- For a running total the `ORDER BY` inside the window must be COMPLETE. Leave only
  `ORDER BY day` and a customer has two purchases on the same day, and the
  accumulation across those ties falls non-deterministically — it may jump from run
  to run. We closed that with the second key `id`; in production any column that
  guarantees a unique order works (the primary key itself, or a timestamp with
  sufficient precision).
- When a window has `ORDER BY` but no explicit frame, Postgres supplies the default
  `RANGE BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW`. For our series with a unique
  order it gives exactly what we expect, but on ties `RANGE` behaves differently from
  `ROWS`. That's already frame territory — we'll cover it separately.
- On large data a window often requires a sort, and a sort that doesn't fit in memory
  spills to a temporary file on disk (`Sort Method: external merge` in the plan,
  module 06). How much memory to grant the sort (`work_mem`) and whether to back the
  window with an index — that's your DBA's concern, not a line of SQL.

## What to take away

- A window function is the same aggregate (`sum`/`avg`/`count`) but with
  `OVER (...)`; unlike `GROUP BY` it does NOT collapse rows — it glues the result
  on as a column.
- `PARTITION BY` slices the table into windows (here — by customer); `OVER ()`
  with no partition is one window over all rows, i.e. the grand total.
- `ORDER BY` INSIDE the window turns `sum` into a running total — accumulation
  from the window start to the current row; the tie-break (`id`) must be complete,
  otherwise the accumulation is non-deterministic on ties.
- You cannot filter by a window function's value directly in `WHERE` — the window
  is computed later; for that you wrap the window result in a CTE.

That last limitation is the bridge to the next unit. A window can compute a sum
but can't, on its own, pick out "the top-3 customers by revenue" or "each
customer's first purchase": for that you first assign a row number and rank, then
filter by it from the outside. In unit 08-02 we'll take `row_number`, `rank`, and
`dense_rank`, wrap them in a CTE, and finally solve that very top-N problem we
stumbled over here.
