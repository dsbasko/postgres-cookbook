# 08-02 — Ranking and top-N per group

Brew's marketer showed up with two requests, both of which sounded harmless. First: "give me the top-1 drink in each category — what exactly sells best in coffee, in cold drinks, in tea." Second: "lay out the whole menu by sales quartiles — who the leaders are, and who's due to be cut." The chain's analyst sat down to write the query, typed the usual `SELECT category, max(units) FROM drink_sales_lab GROUP BY category` — and froze.

`max(units)` returned a number — `150` for coffee. But the marketer needed the *name of the drink* that made those 150. You can't just add `drink` to the `SELECT`: it's neither under an aggregate nor in `GROUP BY`, and Postgres won't allow that. You can wriggle out with a correlated subquery or a `JOIN` back on `max`, but then on ties — and within coffee two drinks each sold 120 — *both* show up, and it's unclear which one counts as "top-1." And `GROUP BY` can't do quartiles at all: to spread eight rows across four buckets, you need to compare a row against its neighbours in order, not collapse them into a single number.

The incident is that aggregates answer the question "what value," while the marketer was asking "which row" and "what place it's in." Those are questions about rank — and ranking window functions answer them.

## Three ranks and their behaviour on ties

Postgres gives three functions that number rows within a window by `ORDER BY`. From the outside they look identical — all three emit `1, 2, 3, …`. The difference shows up exactly where two rows are equal in sort order, that is, on a tie.

`row_number()` assigns a strictly unique number: `1, 2, 3, 4`. Two rows with identical sales still get different numbers — which one gets which is decided by the order in which they happen to come up (and if `ORDER BY` has no unique tie-break, it's decided by chance). `rank()` gives ties *one* number and then *skips* the following ones: after two second places comes fourth immediately — `1, 2, 2, 4`. This is the "sport" rank: two silvers, and there's no bronze, straight to fourth place. `dense_rank()` also gives ties one number, but does *not* skip: `1, 2, 2, 3`. This is the "dense" rank — after two seconds comes third, no gap in the numbering.

An important subtlety: what counts as a tie in the first place? For `rank()` and `dense_rank()` two rows are peers (equal in rank) if and only if they're equal across *all* columns of the window's `ORDER BY`. Add one more column to `ORDER BY` on which the rows differ, and the tie falls apart — `rank`/`dense_rank` degenerate into `row_number`. That's not a bug, it's the definition: rank is computed over what's written in `ORDER BY`, no more and no less.

## What our code shows

The heart of the lesson is `query.sql`. The first query puts the three ranks side by side within a single category, `coffee`, and this is where that tie subtlety becomes visible — which is why it uses *two different windows*:

```sql
SELECT
    drink,
    units,
    row_number() OVER wu AS rn,
    rank()       OVER wt AS rnk,
    dense_rank() OVER wt AS dns
FROM drink_sales_lab
WHERE category = 'coffee'
WINDOW wu AS (ORDER BY units DESC, drink),
       wt AS (ORDER BY units DESC)
ORDER BY units DESC, drink;
```

Window `wu` sorts by `units DESC, drink` — the extra `drink` provides a unique tie-break, so `row_number()` gets strict numbering `1, 2, 3, 4` with no randomness. Window `wt` sorts by `units DESC` *only* — no tie-break, so Cappuccino and Espresso at 120 stay genuine peers, on which `rank()` and `dense_rank()` will show their signature behaviour. Computing all three over one window would be a mistake: a shared tie-break on `drink` would make 120 and 120 distinguishable, and then `rank`/`dense_rank` would emit `2, 3` instead of `2, 2` — that very degeneration into `row_number`.

The second query solves the marketer's top-1 task. The technique is classic: number rows within each category and keep only the first.

```sql
WITH ranked AS (
    SELECT category, drink, units,
           row_number() OVER (PARTITION BY category ORDER BY units DESC, drink) AS rn
    FROM drink_sales_lab
)
SELECT category, drink, units
FROM ranked
WHERE rn = 1
ORDER BY category;
```

`PARTITION BY category` restarts the numbering within each category, so `rn = 1` is the leader of its group, not of the whole table. Why does the numbering hide in a CTE rather than getting filtered directly with `WHERE rn = 1` on the outside? Because window functions are computed *after* `WHERE` — at the filtering stage `rn` doesn't exist yet. So we first compute the rank in a subquery, and apply the condition to its result.

The third query answers the quartiles request — `ntile(4)` spreads all eight drinks across four equal buckets by sales, two in each. Bucket `1` is the leaders, `4` the laggards; the exact position within a bucket doesn't matter, the bucket itself does.

`cmd/demo/main.go` is thin: it opens a pool, calls three typed methods (`RankFunctions`, `TopPerCategory`, `Quartiles`, generated by sqlc from `query.sql`), and prints the result via `tabwriter`. The `drink_sales_lab` data is baked in as a fixed seed in `schema.sql`, so the output is deterministic.

## Running it

```sh
docker compose up -d
make lecture L=08-analytical-and-lateral/08-02-ranking-and-top-n-per-group T=db-reset
make lecture L=08-analytical-and-lateral/08-02-ranking-and-top-n-per-group
```

`T=run` is the default target and can be omitted. From inside the unit directory it's just `make db-reset` and `make run`.

(The demo prints in Russian.)

```
1) Три ранга в категории coffee (ORDER BY units DESC, drink):
НАПИТОК   продано  row_number  rank  dense_rank
Латте     150      1           1     1
Капучино  120      2           2     2
Эспрессо  120      3           2     2
Раф       90       4           4     3
   → row_number уникален (2,3); rank ставит ничьим 2,2 и прыгает на 4; dense_rank идёт 2,2,3.

2) Лидер продаж в каждой категории (row_number() = 1 в CTE):
КАТЕГОРИЯ  напиток   продано
coffee     Латте     150
cold       Колд брю  70
tea        Сенча     50

3) ntile(4) — квартили продаж (корзина 1 — лидеры, 4 — аутсайдеры):
НАПИТОК   продано  квартиль
Латте     150      1
Капучино  120      1
Эспрессо  120      2
Раф       90       2
Колд брю  70       3
Сенча     50       3
Фраппе    40       4
Матча     30       4
```

Cappuccino and Espresso in the first block are both at 120. `row_number` split them into `2` and `3` (tie-break on `drink`), while `rank` and `dense_rank` left both at `2`. Then Raf at 90: `rank` jumps to `4` (it skipped `3`, because two places were taken by the twos), while `dense_rank` goes exactly `3` — no gap.

## The fence

`row_number()` is non-deterministic without a *full* `ORDER BY`. If two rows have nothing to tell them apart, the engine is free to number them in any order — and on the next run the order can change. That's why `wu` has `drink` as the second key: it guarantees that `1, 2, 3, 4` land the same way every time. In production your analyst always adds such a tie-break when rank is used to select rows, otherwise "top-1" can quietly pick different rows between runs.

With `rank()` and `dense_rank()` it's exactly the opposite: an extra tie-break *breaks* the tie. Add a unique column to their `ORDER BY` and 120/120 stop being peers, and the functions degenerate into `row_number`. That's precisely why our query gives them a separate window `wt` *without* `drink`. When deciding "rank by what," think about what should count as a tie, and put exactly those columns in `ORDER BY` — not one extra.

`ntile()` on a number of rows that doesn't divide evenly doesn't fail — it scatters the remainder into the *first* buckets: eight rows over four buckets divide evenly, but nine would give buckets of `3, 2, 2, 2`. Keep this in mind when building deciles on data whose count isn't a multiple of the bucket count — the first buckets end up slightly fuller.

"Top-N per group" via `row_number() = 1` in a CTE is a working classic, but not the only path. On huge partitions `LATERAL` (which we'll reach in 08-05) is sometimes faster, or an index on `(category, units DESC)`, off which the top-1 of each group comes almost for free. In production your DBA will look at the plan: if `row_number` is driving a sort of the whole table just for one row per category, an index or `LATERAL` will cut the work by an order of magnitude.

And finally — the choice between `rank` and `dense_rank` is not about performance, it's about the meaning of the report: whether you want "gaps" in the numbering after ties. Want "after two silvers, straight to fourth" — `rank`. Want "levels with no skips, second-third-fourth" — `dense_rank`. Decide by how people will read the report.

## What to take away

The three ranking functions number rows within a window by `ORDER BY` and diverge only on ties: `row_number()` is strictly unique (`1, 2, 3, 4`), `rank()` gives ties one number and skips the following ones (`1, 2, 2, 4`), `dense_rank()` gives one number with no skip (`1, 2, 2, 3`). A tie here means equality across *all* columns of the window's `ORDER BY`, and the order of keys in `ORDER BY` decides everything: for strict numbering add a unique tie-break, for honest peers don't. "Top-N per group" is built from `row_number()` with `PARTITION BY` in a CTE and `WHERE rn = 1` on the outside, because the window is computed after `WHERE`. And `ntile(n)` lays rows out into buckets when you need not an exact rank but a group — quartiles, deciles, percentiles.

This is a direct development of 08-01: there we first placed `OVER (...)` and computed aggregates over a window without collapsing rows. Ranking functions are the same window functions, only instead of "sum over the window" they answer "what place in the window." The next unit, 08-03, goes further along the same axis: `lag`/`lead` look at neighbouring rows of the window (yesterday's sales next to today's), and explicit frames define exactly which slice of the window goes into the calculation — a rolling weekly average, a running total, the difference with the previous day. After rank, which answers "where the row stands," it's natural to ask "what's next to the row" — and that's what we'll do.
