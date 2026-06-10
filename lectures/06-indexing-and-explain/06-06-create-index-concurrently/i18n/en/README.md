# 06-06 — CREATE INDEX CONCURRENTLY

In the previous five units we chose the right index. One question remains — the one that hits you in production: how do you **add** that index to a table that's being written to right now. Brew decided to drop an index onto the hot orders table during the lunch peak — with a plain `CREATE INDEX`. The command took a `SHARE` lock on the table, and every `INSERT`/`UPDATE`/`DELETE` queued up until the build finished. The register couldn't take orders while the index was building. The build itself was "correct" — but the method turned out to be destructive.

The goal of this unit is `CREATE INDEX CONCURRENTLY`: how to build an index **without blocking writes** to the table for the duration of the build, and at what cost. This is the closing unit of the indexing module — here it's not about "which index" but about "how to ship it safely."

## The locks of a plain CREATE INDEX vs CONCURRENTLY

A plain `CREATE INDEX` takes a `SHARE` lock on the table. It lets readers through but **conflicts with writes**: any `INSERT`/`UPDATE`/`DELETE` waits for the index to finish. On a small table that's milliseconds and nobody notices; on a million-row table the build takes seconds to minutes, and writes stand still the whole time.

`CREATE INDEX CONCURRENTLY` takes a weaker lock — `SHARE UPDATE EXCLUSIVE`, which **doesn't conflict with writes**. Postgres builds the index in two passes over the table, waiting between them for old transactions to finish, and the whole time `INSERT`/`UPDATE`/`DELETE` proceed as usual. This is how you add an index to a live table without stopping the app.

## The cost of CONCURRENTLY

A weak lock isn't free — `CONCURRENTLY` has its own rules:

- **Not allowed inside a transaction.** `CREATE INDEX CONCURRENTLY` manages its own transactions (two passes + waiting), so inside an explicit `BEGIN ... COMMIT` it fails with `SQLSTATE 25001` ("cannot run inside a transaction block"). A plain `CREATE INDEX` is transactional and can sit in a migration alongside other DDL; `CONCURRENTLY` can't.
- **Slower and heavier.** Two passes over the table instead of one — overall slower and more work than a plain build. We trade total time for the absence of a lock.
- **Can leave a broken index.** If a concurrent build fails (a conflict, a cancellation, a uniqueness violation on the second pass), an **invalid** index with `indisvalid = false` remains in the catalog. The planner won't use it and it can't be "finished" — only `DROP INDEX` (preferably `CONCURRENTLY` too) and rebuild.

> ⚠️ You need to be able to find broken indexes. The query `SELECT … FROM pg_index WHERE NOT indisvalid` shows every invalid index — it's the first thing you check if `CONCURRENTLY` failed somewhere. Normally it's zero.

## Who waits for whom: SHARE vs CONCURRENTLY

This is the same lock queue that stalled the register on a hot `ALTER` in 02-06 — only now the queue is created by building an index. The difference between the two commands is exactly whether the lock taken conflicts with writes:

```
Plain CREATE INDEX — takes SHARE (conflicts with writes):

  building index:  [=========================]→ done
  writes:          [····· wait in the queue ····]→ proceed only after the build
                    └─ the register is stuck for the whole build ─┘

CREATE INDEX CONCURRENTLY — takes SHARE UPDATE EXCLUSIVE (no conflict):

  building index:  [ pass 1 ]→[ wait for old tx ]→[ pass 2 ]→ done
  writes:          [=== proceed as usual, no queue ===========]→
                    └─ the register takes orders for the whole build ─┘
```

| | `CREATE INDEX` | `CREATE INDEX CONCURRENTLY` |
|---|---|---|
| Lock | `SHARE` — **writes stall** | `SHARE UPDATE EXCLUSIVE` — writes proceed |
| In a transaction | allowed (transactional) | not allowed → `SQLSTATE 25001` |
| Passes over the table | one | two + waiting for old transactions |
| If it fails | rolled back, no index | leaves an invalid one (`indisvalid = false`) |
| When to use | small/cold table, DDL in a migration | hot table in production |

## What our code shows

`demo.sql` (the `run` target) deterministically checks the rules of `CONCURRENTLY` on a lab table `cic_lab`:

```sql
-- 1) plain CREATE INDEX — allowed inside a transaction
BEGIN; CREATE INDEX cic_lab_plain_idx ON cic_lab (payload); COMMIT;
-- 2) CONCURRENTLY inside a transaction → error 25001
BEGIN; CREATE INDEX CONCURRENTLY cic_lab_conc_idx ON cic_lab (payload); ROLLBACK;
-- 3) CONCURRENTLY outside a transaction → success, indisvalid = t
CREATE INDEX CONCURRENTLY cic_lab_conc_idx ON cic_lab (payload);
-- 4) find broken indexes → 0
SELECT count(*) FROM pg_index WHERE NOT indisvalid;
```

And the **live** non-blocking of writes is shown by `session-a.sql` / `session-b.sql`: session A builds a `CONCURRENTLY` index on a 3-million-row table while session B does an `INSERT` — and it goes through immediately, without waiting for the build to finish (with a plain `CREATE INDEX` that same `INSERT` would queue).

## Running it

The deterministic rules demo:

```sh
docker compose up -d
make lecture L=06-indexing-and-explain/06-06-create-index-concurrently
```

Output (`stdout`; the step-2 error text goes to `stderr`):

```
== 1) обычный CREATE INDEX можно внутри транзакции (он транзакционный) ==
                  result                   
-------------------------------------------
 обычный индекс собран внутри BEGIN/COMMIT


== 2) CREATE INDEX CONCURRENTLY ВНУТРИ транзакции запрещён (ошибка в stderr) ==
SQLSTATE = 25001 (cannot run inside a transaction block)

== 3) CREATE INDEX CONCURRENTLY ВНЕ транзакции — успех, индекс валиден ==
      index       | indisvalid 
------------------+------------
 cic_lab_conc_idx | t


== 4) проверка на битые индексы (сорванный CONCURRENTLY оставляет indisvalid=false) ==
 invalid_indexes 
-----------------
               0
```

(The demo prints in Russian.) A plain `CREATE INDEX` ran fine inside `BEGIN/COMMIT`; `CONCURRENTLY` there failed with `25001`; outside a transaction it built and the index is valid (`indisvalid = t`); there are no broken indexes.

The live two-session scenario (interactive):

```sh
# terminal 1:
make lecture L=06-indexing-and-explain/06-06-create-index-concurrently T=session-a
# then QUICKLY in terminal 2, while A builds the index:
make lecture L=06-indexing-and-explain/06-06-create-index-concurrently T=session-b
# at the end:
make lecture L=06-indexing-and-explain/06-06-create-index-concurrently T=db-reset
```

Session B inserts a row and gets `INSERT 0 1` **during** A's index build — the write isn't blocked. (The order depends on timing: on a fast machine the 3-million-row build takes ~a second, so you must switch to terminal 2 right away — a known caveat of two-session demos, as in 05-03.)

## The fence

`CONCURRENTLY` removes the main pain — the write lock — but safely shipping an index in production doesn't end there, and beyond it lies your DBA/release engineer's territory:

- **`CONCURRENTLY` isn't instant.** At the start it still takes a brief lock and **waits for all current transactions** on the table to finish — one hung long transaction will delay the build's start, so migrations run with `lock_timeout` and retries.
- **Not allowed in a transaction → it needs a special migration step.** Since `CONCURRENTLY` can't go in a shared transaction block, migration tools must be able to run such steps separately (many frameworks require an explicit marker).
- **After a failed build, someone has to clean up** the invalid index (`DROP INDEX CONCURRENTLY` + recreate) — an operational procedure.
- **`CONCURRENTLY` has relatives** for other downtime-free operations (`REINDEX CONCURRENTLY` for a bloated index, `DROP INDEX CONCURRENTLY`) — choosing and scheduling them is cluster maintenance.

The course boundary: your job is to **know that an index on a hot table is added via `CONCURRENTLY`, not a plain `CREATE INDEX`**, and to mark such migrations accordingly; orchestrating zero-downtime rollouts is beyond it.

## Takeaways

- A plain `CREATE INDEX` takes a `SHARE` lock and **stops writes** to the table for the build's duration — on a hot table that's downtime.
- `CREATE INDEX CONCURRENTLY` builds the index with a weak lock (`SHARE UPDATE EXCLUSIVE`), not blocking `INSERT`/`UPDATE`/`DELETE`.
- The cost: not allowed inside a transaction (`SQLSTATE 25001`), slower (two passes), a failed build leaves an invalid index (`indisvalid = false`).
- Broken indexes are found with `… WHERE NOT indisvalid`; fixed via `DROP INDEX CONCURRENTLY` + recreate.
- `CONCURRENTLY` isn't instant: it waits for current transactions at the start → in production use `lock_timeout`.

That closes module **06 "Indexing and performance through EXPLAIN"**: reading `EXPLAIN ANALYZE` (with buffers, on by default in PG18), column order in a composite index and skip-scan, non-sargable conditions and the expression index, partial/covering/unique indexes, GIN for jsonb/arrays, and a safe rollout via `CONCURRENTLY`. Next up — module **07 "JSONB, arrays, and search"**: jsonb access and containment, when not to use jsonb, SQL/JSON path, arrays versus a junction table, full-text search, and fuzzy search via `pg_trgm`.
