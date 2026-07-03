# 02-06 — ALTER TABLE: a migration mindset

Brew shipped a migration in the middle of the business day. The line looked harmless: `ALTER TABLE orders ALTER COLUMN amount TYPE numeric(12,2)`. On a table of tens of millions of rows, that command took an `ACCESS EXCLUSIVE` lock and set about **rewriting the entire table** from scratch — for about eight minutes. The whole time, not a single order could be read or written: the app hit timeouts, the register stalled. Meanwhile a neighboring migration in the same release — `ADD COLUMN ... DEFAULT 'active'` — finished instantly and went unnoticed. The difference isn't in the syntax: some `ALTER`s touch only metadata, others rewrite the whole table under a lock.

The goal of this unit isn't "memorize the list of safe ALTERs" but a **reflex**: before a migration, ask "is this instant, or is it a rewrite / a long lock?". We'll observe the cost via `relfilenode` — the identifier of the table's physical file. It changes **only** on a full rewrite; if an `ALTER` only adjusted metadata, the file is the same. We'll also cover the two-phase constraint add (`NOT VALID` → `VALIDATE`), which separates a fast metadata change from a long scan.

## Instant vs rewrite

Some `ALTER`s are edits to the system catalog, without touching on-disk data. The classic — `ADD COLUMN` with a **constant** `DEFAULT`: since PG11 the new value is stored as table metadata, existing rows aren't touched (`relfilenode` unchanged). Instant, regardless of table size.

Others require rewriting every row, because the physical representation of the data changes. `ALTER COLUMN ... TYPE` that changes the type (`int` → `bigint`, `text` → `numeric`) is almost always a rewrite: Postgres creates a new file, pumps all rows into it in the new format, and swaps the `relfilenode`. On a large table that's a long `ACCESS EXCLUSIVE` lock — exactly what took down Brew's register.

> ⚠️ Important caveat: even an "instant" `ADD COLUMN` briefly takes `ACCESS EXCLUSIVE`. If a long query is running over the table at that moment, the migration queues behind it — and everyone else queues behind the migration. That's why in production migrations run with `lock_timeout` (see "The fence").

## A two-phase constraint: NOT VALID → VALIDATE

Adding a `CHECK` (or `FOREIGN KEY`) the ordinary way means scanning the whole table, checking old rows, under a strong lock. PG can split this into two phases:

1. `ADD CONSTRAINT ... NOT VALID` — instant: the constraint is created and applied **to new** rows, but old ones aren't scanned (`convalidated = f`). A brief lock.
2. `VALIDATE CONSTRAINT` — a separate command to scan the old rows. It takes only `SHARE UPDATE EXCLUSIVE` — it **doesn't block** reads and writes, running in the background.

So "add a rule" stops being an emergency brake: the hot phase is instant, and the long check doesn't hold the app.

## The lock queue and the cost of ALTER

A migration's main trap isn't the "rewrite" but the **lock queue**: even an instant `ALTER` briefly takes `ACCESS EXCLUSIVE`, and if a long transaction hangs ahead of it — it stalls, and all traffic stalls behind it:

```
time →
T1  long SELECT over orders    ╞════════ holds ACCESS SHARE ════════╡
T2  migration: ALTER ADD COLUMN     ╞···· waits for ACCESS EXCLUSIVE ····╡══╡
T3  ordinary INSERT into orders           ╞······ stuck behind the migration ······╡
                                          ▲
                      even the "instant" ALTER queued behind T1 —
                      and the whole write stream (T3) queued behind ALTER
```

And here's how the operations themselves compare by cost:

| `ALTER` | What happens | Cost |
|---|---|---|
| `ADD COLUMN ... DEFAULT <constant>` | metadata edit (since PG11) | instant |
| `ADD CONSTRAINT ... NOT VALID` | metadata; old rows not scanned | instant (brief lock) |
| `VALIDATE CONSTRAINT` | scans the old rows | background, `SHARE UPDATE EXCLUSIVE` (doesn't block writes) |
| `ALTER COLUMN ... TYPE` (representation change) | rewrites every row | long `ACCESS EXCLUSIVE` |
| `ADD COLUMN ... DEFAULT now()` (volatile) | rewrites the table | long `ACCESS EXCLUSIVE` |
| `ALTER COLUMN ... SET NOT NULL` (existing column) | scans the table to verify no NULLs | lock for the duration of the scan (PG12+ skips it with a valid `CHECK (col IS NOT NULL)`) |

## What our code shows

The lesson is in `demo.sql`, on a 1000-row lab table (we don't touch the base tables). Before each `ALTER` we capture `relfilenode`, after — compare:

```sql
SELECT pg_relation_filenode('alter_lab') AS fn \gset before1_
ALTER TABLE alter_lab ADD COLUMN status TEXT NOT NULL DEFAULT 'active';   -- metadata
SELECT pg_relation_filenode('alter_lab') AS fn \gset after1_
SELECT (:before1_fn = :after1_fn) AS filenode_unchanged;                  -- t

ALTER TABLE alter_lab ALTER COLUMN n TYPE bigint;                         -- rewrite
-- ... comparison → f

ALTER TABLE alter_lab ADD CONSTRAINT n_positive CHECK (n > 0) NOT VALID;  -- convalidated = f
ALTER TABLE alter_lab VALIDATE CONSTRAINT n_positive;                     -- convalidated = t
```

`filenode_unchanged = t` means "same file, table not rewritten"; `f` means "new file, there was a rewrite." `convalidated` shows the constraint's phase.

## Running it

```sh
docker compose up -d
make lecture L=02-schema-and-constraints/02-06-alter-table-migration-mindset T=db-reset
make lecture L=02-schema-and-constraints/02-06-alter-table-migration-mindset
```

Output:

```
== 1) ADD COLUMN с константным DEFAULT — мгновенно (только метаданные) ==
 filenode_unchanged 
--------------------
 t


== 2) ALTER COLUMN ... TYPE int -> bigint — таблица ПЕРЕПИСана (новый relfilenode) ==
 filenode_unchanged 
--------------------
 f


== 3) ADD CONSTRAINT CHECK ... NOT VALID — мгновенно (старые строки не сканируются) ==
 validated_after_not_valid 
---------------------------
 f


== 4) VALIDATE CONSTRAINT — отдельный шаг (не блокирует запись) ==
 validated_after_validate 
--------------------------
 t
```

(The demo prints in Russian.) `ADD COLUMN ... DEFAULT 'active'` left `relfilenode` unchanged (`t`) — an instant metadata change. `ALTER COLUMN n TYPE bigint` changed the file (`f`) — the table was rewritten whole (in production that's the very lock). `CHECK ... NOT VALID` was created unvalidated (`convalidated = f`), and `VALIDATE` as a separate step scanned and marked it validated (`t`) — the hot part is instant, the long part doesn't hold writes.

## The fence

What we simplified: `relfilenode` is a good "rewrote / didn't" indicator, but not the whole picture, and beyond it lies territory your DBA holds:

- **The lock matters more than the rewrite.** Even an instant `ADD COLUMN` takes `ACCESS EXCLUSIVE`, and if a long transaction hangs ahead of it, the lock queue will stall writes out of nowhere — so migrations run with a short `lock_timeout` and retries, not "dry."
- **A big rewrite isn't done head-on.** `ALTER TYPE` on a hot table is split into steps in production: you add a new column, backfill data in batches in the background, then swap it atomically — or use online tools (`pg_repack`, orchestrators like Reshape).
- **Version nuances.** `ADD COLUMN` with a **volatile** default (`now()`, a function) already rewrites the table. And `NOT NULL` depends on how it appeared: `ADD COLUMN ... NOT NULL` with a constant default is instant (the new column has no `NULL`s anyway — that's what our demo shows), while `ALTER COLUMN ... SET NOT NULL` on an existing column scans the table to check the old rows (PG12+ skips the scan if a valid `CHECK (col IS NOT NULL)` exists).

The course boundary: orchestrating zero-downtime migrations leans toward DBA/DevOps; your job is to **recognize a dangerous `ALTER` in a migration review** and not ship a hot-table rewrite during business hours.

## Takeaways

- `relfilenode` changes only on a full table rewrite — an observable indicator of "instant vs rewrite."
- `ADD COLUMN` with a constant `DEFAULT` — instant (metadata); `ALTER COLUMN ... TYPE` with a representation change — a rewrite under `ACCESS EXCLUSIVE`.
- `ADD CONSTRAINT ... NOT VALID` (instant) + `VALIDATE CONSTRAINT` (background, `SHARE UPDATE EXCLUSIVE`) — a two-phase rule add with no emergency brake.
- Even an instant `ALTER` takes `ACCESS EXCLUSIVE` briefly → production needs `lock_timeout`; a big rewrite is done via new-column + batched backfill + swap.
- The reflex before a migration: "does this edit metadata, or rewrite the table / hold a long lock?".

That closes module **02 "Schema, DDL, and constraints"**: `IDENTITY`/`DEFAULT`, `NOT NULL`/`PK` and key choice, foreign keys, `UNIQUE`/`CHECK`, generated columns/domains, and the migration mindset. Next up — module **03 "CRUD fluency"**: confident `INSERT ... RETURNING`, `SELECT` with pagination, safe `UPDATE`/`DELETE`, `upsert`, and sober `NULL` semantics.
