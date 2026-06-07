# 09-01 — MERGE and COPY: bulk load and reconcile in one pass

Every night the supplier sends Brew a stock file: one row per drink —
`SKU;quantity`. We need to do two things with it. First, load it fast: thousands
of rows, and inserting them one `INSERT` at a time is thousands of round-trips —
the nightly window is not elastic. Then, reconcile the file against our stock:
where more arrived, update; what we don't carry yet, add; and what the supplier
sent with a zero (discontinued) — remove. The backend used to grind through this
in a loop: for each file row a `SELECT` to check whether the item exists, then an
`INSERT` or `UPDATE`, and deletions in a separate pass entirely. Long-winded —
and between "read" and "write" yawns a window where the data could shift.

Postgres closes both tasks with two tools: `COPY` for loading and `MERGE` for
reconciling.

## COPY FROM STDIN: loading without a "query per row"

`COPY` is not an `INSERT` but a separate streaming load protocol. Instead of
"parse query → execute → reply" for every row, the driver opens a single stream
and pushes the whole batch into it; the server writes the rows in bulk. At scale
this is many times faster than a chain of `INSERT`s: no per-row query parse, no
per-row round-trip.

`COPY FROM STDIN` is precisely a protocol, so there is no place for it in our
"SQL by hand → sqlc" rule: sqlc generates query functions, and a streaming
`COPY` is not one of them. In `pgx` it lives as a separate method — `pool.CopyFrom`,
which takes a table name, a column list, and a row source. That is why this whole
unit is raw `pgx` (an escape hatch ahead of sqlc), like 00-03 and 05-05.

## MERGE: INSERT, UPDATE, and DELETE in one command

`MERGE` takes a source (our loaded feed `s`) and a target (the stock `t`),
matches them on the `ON` condition, and for each row picks a branch:

- `WHEN MATCHED AND s.on_hand = 0 THEN DELETE` — the item exists for us but
  arrived with a zero → discontinue it;
- `WHEN MATCHED THEN UPDATE` — the item exists, a new quantity arrived → update;
- `WHEN NOT MATCHED THEN INSERT` — we don't carry the item → add it.

Branches are checked top to bottom, the first matching one fires — which is why
the `AND s.on_hand = 0` condition comes before the general `MATCHED`. That whole
"read — decide — write" loop from the application collapses into a single
declarative command: you describe *what the state should become* for each
outcome, and the database does the row walk and branch selection.

## merge_action() and RETURNING: a report of what happened

After a `MERGE` you want to know exactly what it did to each row. For that
`RETURNING` offers the `merge_action()` function — it returns `'INSERT'`,
`'UPDATE'`, or `'DELETE'` for the row just processed. In the `DELETE` branch the
target columns `t.*` are the values of the **deleted** row (as they were before
deletion). So one command both changes the data and immediately reports what
became of it, with no second query.

The order of `RETURNING` rows from a `MERGE` is undefined, so in the demo we
collect them into a slice and sort by `SKU` in Go — otherwise the output would
drift between runs.

## What our code shows

This is a raw `pgx` unit; the heart is two operations in `main.go`. First,
`COPY FROM STDIN` loads the feed into a staging table in a single call:

```go
copied, err := pool.CopyFrom(ctx,
    pgx.Identifier{"supplier_feed_lab"},
    []string{"drink_sku", "on_hand"},
    pgx.CopyFromRows(feed),
)
```

Then one `MERGE` reconciles the staging table against the stock and reports each
row via `merge_action()`:

```sql
MERGE INTO stock_lab t
USING supplier_feed_lab s ON t.drink_sku = s.drink_sku
WHEN MATCHED AND s.on_hand = 0 THEN DELETE
WHEN MATCHED THEN UPDATE SET on_hand = s.on_hand
WHEN NOT MATCHED THEN INSERT (drink_sku, on_hand) VALUES (s.drink_sku, s.on_hand)
RETURNING merge_action() AS action, t.drink_sku, t.on_hand;
```

The feed is deliberately arranged to trigger all three branches: `ESP-01`/`LAT-01`
exist for us (UPDATE), `CAP-01` arrived with a zero (DELETE), `CLD-01`/`TEA-01`
are not on the shelf yet (INSERT).

## Running it

```sh
docker compose up -d
make lecture L=09-writes-eventing-and-server-logic/09-01-merge-and-copy T=db-reset
make lecture L=09-writes-eventing-and-server-logic/09-01-merge-and-copy
```

`T=run` is the default and can be omitted. From inside the unit directory it is
shorter: `make db-reset`, then `make run`.

```
1) COPY FROM STDIN: загружено строк поставки = 5
   Наш склад ДО сверки (stock_lab):
   SKU     остаток
   CAP-01  40
   ESP-01  50
   LAT-01  30

2) MERGE поставки в склад — один проход, три исхода:
SKU     merge_action()  остаток
CAP-01  DELETE          40
CLD-01  INSERT          25
ESP-01  UPDATE          60
LAT-01  UPDATE          35
TEA-01  INSERT          15

3) Наш склад ПОСЛЕ сверки (stock_lab):
   SKU     остаток
   CLD-01  25
   ESP-01  60
   LAT-01  35
   TEA-01  15
```

`CAP-01` arrived with a zero — `merge_action()` reported `DELETE`, and it is gone
from the final stock (the `40` in the DELETE row is its value *before* deletion).
`CLD-01` and `TEA-01` were added (INSERT), `ESP-01`/`LAT-01` were updated to the
new quantities — all in a single command.

## The fence

The main trap: **`MERGE` is not race-safe** as an upsert. Under concurrency two
parallel `MERGE` commands can both fail to find a row (`NOT MATCHED`), both go
down the `INSERT` branch — and one fails on a uniqueness violation, or, with no
key, you get duplicates. `MERGE` does not do for you what `INSERT ... ON CONFLICT`
(03-04) does: that atomically catches a conflict on a unique index and decides
"insert or update" itself. So the rule: for a **concurrent upsert by key** reach
for `ON CONFLICT`, and use `MERGE` for **batch reconciliation** of two sets (our
typical case: nightly syncing a staging table into the stock), where you control
that there are no parallel merges into the same table.

About `COPY`: it is fast, but it barely validates data on the fly and loads it
"as is". So the production pattern is to `COPY` into a **staging** table (here
`supplier_feed_lab`) and from there move into the live table with checks and a
`MERGE`/`INSERT ... SELECT` reconciliation. `COPY`ing straight into a table laden
with constraints and triggers loses both speed and predictability. Fine-tuning
`COPY` (formats, `FREEZE`, dropping indexes during a massive load) is work at the
seam with your DBA, and we don't touch it here.

## Takeaways

`COPY FROM STDIN` (`pool.CopyFrom` in pgx) loads a batch in a single stream,
bypassing "parse query and round-trip per row" — it is the bulk-load tool, and it
is absent from sqlc because it is a protocol, not a query. `MERGE` reconciles a
source with a target and does `INSERT`/`UPDATE`/`DELETE` in one command, while
`merge_action()` in `RETURNING` reports what happened to each row. But `MERGE` is
about **batch reconciliation**, not concurrent upsert: for "insert or update"
under load, `INSERT ... ON CONFLICT` from 03-04 remains the tool.

Next — when writes are not a once-a-night batch but a stream from many workers at
once, and no task may be processed twice. In 09-02 a job queue on
`FOR UPDATE SKIP LOCKED` hands work to N workers with no duplicates and no
blocking on each other.
