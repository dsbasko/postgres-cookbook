# 10-02 — Price-and-promo engine

Brew is awash in promotions: the latte goes up in February, the `SUMMER` promo
code runs all summer, and `AUTUMN` takes over in the fall. And here surfaces a
class of bugs that no amount of application data fixes. A manager accidentally
gave a drink **two prices for the same day** — the register doesn't know which to
charge. A marketer launched that same `SUMMER` code with an **overlapping window**
— the discount applies twice or not at all, depending on which row the scheduler
picks. This is not a "bad query" but a missing invariant: "a drink has exactly one
price at any moment" and "the windows of one promo code don't overlap". Such rules
can't live in service code — every writer to the table has to check them, and
sooner or later someone forgets. The invariant must live in the schema.

Postgres can hold "these intervals don't overlap" right inside `CREATE TABLE`.

## Temporal PK (PG18): one price per moment per drink

PG18 added the words **`WITHOUT OVERLAPS`** to the primary key. A drink's prices
live in the `price_periods` table, where the validity interval is a `tstzrange`
column:

```sql
CREATE TABLE price_periods (
    drink_id    bigint    NOT NULL,
    price_cents bigint    NOT NULL CHECK (price_cents > 0),
    valid       tstzrange NOT NULL,
    PRIMARY KEY (drink_id, valid WITHOUT OVERLAPS)
);
```

Read the key as: "for one `drink_id`, the `valid` ranges must not overlap". The
scalar part (`drink_id`) is compared for equality, the range part (`valid`) for
overlap. Two **adjacent** periods (`[01-01, 02-01)` and `[02-01, 03-01)`) are
accepted: `tstzrange` bounds are half-open `[from, to)`, so the shared end
`02-01` doesn't count as an overlap. But a period `[01-15, 02-15)` that covers
both is rejected with an `exclusion_violation` error — SQLSTATE `23P01`. This is
the modern PG18 way to say "no two prices for the same moment" in a single line of
the table definition.

To compare the scalar `drink_id` for equality inside a gist key you need the
`btree_gist` extension — `CREATE EXTENSION IF NOT EXISTS btree_gist`.

## The classic EXCLUDE (pre-PG18): the same ban on promos

`WITHOUT OVERLAPS` is new, but the "no overlap" guarantee itself lived in Postgres
long before version 18, via an **exclusion constraint** `EXCLUDE USING gist` (see
module 02 on constraints and `EXCLUDE`). Promo code windows live in
`promo_windows`:

```sql
CREATE TABLE promo_windows (
    code text      NOT NULL,
    span tstzrange NOT NULL,
    EXCLUDE USING gist (code WITH =, span WITH &&)
);
```

It reads symmetrically to the temporal key: two rows are forbidden where `code`
are equal (`WITH =`) **and** `span` overlap (`WITH &&`). The same `SUMMER` with an
overlapping window is rejected with the same `23P01`. But a **different** code
`AUTUMN` with the exact same window passes — the constraint keys on the pair "code
+ window", not on the window alone. This is the pre-PG18 form of the same
invariant; `EXCLUDE` also relies on `btree_gist` (equality on `code`). It's worth
knowing both forms: on an existing table without a temporal key, you'll fix
overlaps precisely through `EXCLUDE`.

## RETURNING old/new (PG18): price audit with no separate SELECT

When a period's price changes, you want to log "before → after". Before PG18 that
was two passes: `SELECT` the old price, then `UPDATE`, or `UPDATE ... RETURNING`
only the new one plus a guess about the old. PG18 gives `RETURNING` the
pseudo-tables **`old` and `new`** — a single `UPDATE` returns both versions of the
row at once (we already saw this trick in 03-05):

```sql
UPDATE price_periods
   SET price_cents = $1
 WHERE drink_id = 1 AND valid = tstzrange('2025-02-01', '2025-03-01')
RETURNING old.price_cents, new.price_cents;
```

A single statement returns both the old and the new price — and that's what fills
the `price_audit` row, with no separate `SELECT` before and after.

## Half-open intervals and the two forms of the ban

The whole mechanism rests on one property of `tstzrange` — the bounds `[from, to)`
are half-open, the end is not in the interval. On a time axis you see it at once
(price periods of drink #1):

```
Time axis.  Bounds [from, to): the end is NOT included.

  [01-01 ──────── 02-01)                   3.00  ✓ accepted
                  [02-01 ──────── 03-01)    3.20  ✓ accepted
  adjacent: the point 02-01 belongs only to the right period → no overlap

       [01-15 ──────────── 02-15)           9.99  ✗ 23P01
       covers both → exclusion_violation
```

Two adjacent periods share the boundary `02-01` but don't conflict: the left one
gave that point up, the right one took it. The third period physically covers both —
and `23P01` rejects it. The same ban exists in Postgres in two forms:

| | temporal PK `WITHOUT OVERLAPS` | `EXCLUDE USING gist` |
|---|---|---|
| Version | PG18 and newer | long before PG18 |
| Where it lives | inside the `PRIMARY KEY` | a separate table constraint |
| Spelling | `PRIMARY KEY (drink_id, valid WITHOUT OVERLAPS)` | `EXCLUDE USING gist (code WITH =, span WITH &&)` |
| Condition | scalar on `=`, range non-overlapping (implicit) | you spell it: `WITH =` and `WITH &&` |
| Relies on | `btree_gist` | `btree_gist` |
| On overlap | `23P01` | `23P01` |
| When to use | new table, "one row per moment" invariant right in the key | existing table or several range conditions |

## What our code shows

`cmd/demo/main.go` builds three lab tables — `price_periods`, `promo_windows`,
`price_audit` (the canon is untouched) — and runs three scenes. First it lands two
adjacent price periods (accepted) and a third, overlapping one — the temporal PK
rejects it with `23P01`. Then the same on promos: `SUMMER` with an overlap is
rejected, `AUTUMN` with the same window accepted. Finally it raises the second
period's price with a single `UPDATE ... RETURNING old.price_cents,
new.price_cents` and puts "before → after" into `price_audit`. The `outcome`
helper turns an insert error into a short label — `OK` or `SQLSTATE 23P01`.

The unit is an **escape-hatch on raw pgx** (there's a `go.mod`, but no sqlc): sqlc
v1.30.0 doesn't parse DDL with `WITHOUT OVERLAPS` and doesn't understand
`RETURNING old/new` (the same reason as 03-05) — we pick the feature over the
tool.

## Running it

```sh
docker compose up -d
make lecture L=10-use-cases/10-02-price-and-promo-engine T=db-reset
make lecture L=10-use-cases/10-02-price-and-promo-engine
```

`T=run` is the default and can be omitted. From inside the unit directory it is
shorter: `make db-reset`, then `make run`. And `make test` runs the asserted
integration test — this is a capstone, and its check is green only when all three
scenes produce exactly the output below.

```
1) Temporal PK (PG18): у одного напитка не пересекаются периоды цены.
   период цены напитка #1    цена  результат
   [2025-01-01, 2025-02-01)  3.00  OK (принято)
   [2025-02-01, 2025-03-01)  3.20  OK (принято)
   [2025-01-15, 2025-02-15)  9.99  отбито, SQLSTATE 23P01

2) Классический EXCLUDE (до PG18): то же для окон промо-кода.
   промо-код  окно                      результат
   SUMMER     [2025-06-01, 2025-09-01)  OK (принято)
   SUMMER     [2025-08-01, 2025-10-01)  отбито, SQLSTATE 23P01
   AUTUMN     [2025-08-01, 2025-10-01)  OK (принято)

3) RETURNING old/new (PG18): меняем цену и пишем аудит без отдельного SELECT.
   период [2025-02-01, 2025-03-01): цена 3.20 → 3.40 (одним UPDATE ... RETURNING old/new)
   аудит: 1 запись, было 3.20 → стало 3.40
```

Scene one: two adjacent periods land, the overlapping one is rejected with `23P01`
— the temporal PK denied the drink two prices on the same day. Scene two: the same
`SUMMER` with an overlap is rejected with the same `23P01`, while `AUTUMN` with an
identical window is accepted — the key looks at the pair "code + window". Scene
three: a single `UPDATE` raised the price `3.20 → 3.40` and immediately returned
both versions via `old/new`, filling the audit with no separate `SELECT`.

## The fence

- **Both bans rely on `btree_gist`.** On the sandbox the extension installs in one
  line, but in production know **which extensions you depend on**: you have to have
  them in every environment and when migrating a cluster. `EXCLUDE`/gist costs more
  on writes than a plain btree — every insert makes the index check for overlaps, and
  on huge hot tables that is a noticeable price. Keep such constraints where you
  actually need to catch overlaps, not everywhere.
- **The half-openness of the range is not a detail but the point.** `tstzrange`
  defaults to `[from, to)`, the end excluded, so `[.., 02-01)` and `[02-01, ..)` do
  **not** overlap and both fit side by side. Were the bounds closed `[from, to]`,
  adjacent periods would share the point `02-01` and the second would be rejected —
  check the bound form when entering periods.
- **`RETURNING old/new` is not a real audit trail.** It's convenient for "before →
  after" in the moment, but here the application **itself** decided to write a row
  into `price_audit`; if someone changes the price bypassing this code, the log stays
  silent. A genuine, unbypassable audit is a trigger + history table on the DB side,
  the territory of 09-05, not `RETURNING`.

## Takeaways

Keep invariants like "these intervals don't overlap" **in the schema**, not in
service code: every writer checks them and the check can't be forgotten. PG18
gives you the temporal key for this — `PRIMARY KEY (drink_id, valid WITHOUT
OVERLAPS)` on a `tstzrange` column; before PG18 the same ban was done with the
exclusion constraint `EXCLUDE USING gist (code WITH =, span WITH &&)`. Both forms
give `23P01` on an overlap and both rely on `btree_gist`; adjacent half-open ranges
don't conflict. And `RETURNING old.* / new.*` (PG18) returns "before → after" in a
single `UPDATE` — convenient, but not a substitute for a real audit.

Next — the final capstone 10-03: an app anti-patterns clinic. We'll gather in one
place the typical rakes the code steps on against Postgres — from N+1 and `OFFSET`
pagination to implicit type casts that kill an index — and work through how to spot
and straighten each one.
