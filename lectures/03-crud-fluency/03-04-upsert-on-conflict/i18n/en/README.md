# 03-04 — upsert via ON CONFLICT

The morning starts with two overnight messages from Pasha.

> **Pasha (in chat, 23:50):** The nightly stock sync failed. duplicate key.
>
> **Pasha (in chat, 23:57):** Restarted it — went through. The file's just a file, what's wrong with it?

"Restarted it — went through" is the worst kind of bug: it doesn't reproduce, so it hasn't gone anywhere. The nightly sync takes the stock export `(shop, drink, on_hand)` from the accounting system; some pairs are already in the database — they need updating, some are new — they need inserting. Naive code first does a `SELECT` for each row and decides: `INSERT` or `UPDATE`. That's three queries where one would do, and — worse — a race: between the `SELECT` and the `INSERT` the second shop sent the same pair, and the `INSERT` failed with that very `duplicate key` — SQLSTATE `23505` from module 02. On the restart the race didn't recur — hence "went through."

The goal of this unit is to do it in one atomic, concurrency-safe command: `INSERT ... ON CONFLICT (...) DO UPDATE`. That's an upsert — "insert, and if such a key already exists, update."

## ON CONFLICT needs an arbiter — a UNIQUE or PK

`ON CONFLICT (shop_code, drink_sku)` means "if uniqueness on these columns is violated." For a conflict to be defined at all, there must be a uniqueness constraint on those columns — a `PRIMARY KEY` or `UNIQUE`. Without it Postgres doesn't know what to catch the conflict on, and the command won't compile. In our table the arbiter is the composite primary key `(shop_code, drink_sku)`: one row per "shop × drink" pair.

## DO UPDATE and the EXCLUDED pseudo-table

On a conflict `DO UPDATE SET ...` runs. There a special pseudo-table `EXCLUDED` is available — it's the row we **tried to insert** (it was "excluded" from the insert due to the conflict). `SET on_hand = EXCLUDED.on_hand` means "take the new stock value from the input." That's how you write a typical upsert counter: the new value overwrites the old. You can also accumulate (`SET on_hand = stock_levels.on_hand + EXCLUDED.on_hand` — add to the current), referring to the old value by the table name and to the new one via `EXCLUDED`.

The alternative is `DO NOTHING`: on a conflict, do nothing. That's the idempotent-insert idiom: "insert if not present, otherwise silently skip." This is exactly how the Brew base schema silences duplicates by `outbox_id` (the `processed_outbox_ids` table) — a redelivered event doesn't break the consumer.

## Insert or update? The xmax trick

`RETURNING` hands back the resulting row, but doesn't say directly whether it was an insert or an update. A well-known trick is the system column `xmax`: a just-inserted row version has `xmax = 0`, an updated one does not. So `(xmax <> 0) AS was_update` is a compact detector. (The system columns `xmin`/`xmax` are MVCC mechanics, covered in **05-02**; here it's enough to know the trick distinguishes an insert from an update.)

## The ON CONFLICT fork: insert or a conflict branch

`INSERT ... ON CONFLICT` is one atomic command with a fork inside. The arbiter (a `UNIQUE`/`PK`) checks whether a row with this key already exists and picks a branch:

```
INSERT (shop_code, drink_sku, on_hand)
        │
        ▼
  key already exists?  ◄── arbiter: PK (shop_code, drink_sku)
   ┌────┴───────────────────┐
   │ no                     │ yes → ON CONFLICT (...)
   ▼                        ▼
INSERT the row       ┌───────┴────────────┐
was_update = false   ▼                    ▼
                 DO UPDATE             DO NOTHING
             SET on_hand =          leave the row alone:
             EXCLUDED.on_hand       0 rows, duplicate silenced
             was_update = true
```

| Branch | On a conflict | Returns | When to use |
|---|---|---|---|
| `DO UPDATE SET …` | overwrites/accumulates the row (`EXCLUDED` = what you tried to insert) | the updated row | syncing reference data, counters |
| `DO NOTHING` | silently skips | 0 rows | idempotent insert (dedup by `outbox_id` in `processed_outbox_ids`) |
| `MERGE` (PG15+) | `WHEN MATCHED / NOT MATCHED` branches | per branch | complex merge logic, but weaker race protection (→ 09-01) |

## What our code shows

Two upserts in `query.sql` — `DO UPDATE` and `DO NOTHING`:

```sql
-- name: UpsertStock :one
INSERT INTO stock_levels (shop_code, drink_sku, on_hand)
VALUES ($1, $2, $3)
ON CONFLICT (shop_code, drink_sku)
DO UPDATE SET on_hand = EXCLUDED.on_hand
RETURNING shop_code, drink_sku, on_hand, (xmax <> 0) AS was_update;

-- name: UpsertIgnore :execrows
INSERT INTO stock_levels (shop_code, drink_sku, on_hand)
VALUES ($1, $2, $3)
ON CONFLICT (shop_code, drink_sku) DO NOTHING;
```

In `main.go` we insert the pair `CENTRAL/ESP-01`, then upsert it again with a new stock value (`DO UPDATE` fires), insert a new pair, then try `DO NOTHING` on an existing key (0 rows, value intact), and print the final state.

## Running it

Bring up the sandbox (from the repo root) and apply the Brew base schema plus the unit's table:

```sh
docker compose up -d
make lecture L=03-crud-fluency/03-04-upsert-on-conflict T=db-reset
make lecture L=03-crud-fluency/03-04-upsert-on-conflict
```

(`T=run` is the default. From inside the unit directory it's `make db-reset`, `make run`.)

Output:

```
1) Первый upsert (CENTRAL/ESP-01, 50): новый ключ → вставка
   on_hand=50, was_update=false

2) Повторный upsert того же ключа (CENTRAL/ESP-01, 80): конфликт → обновление
   on_hand=80, was_update=true  (DO UPDATE SET on_hand = EXCLUDED.on_hand)

3) Upsert нового ключа (NORTH/LAT-01, 30): вставка
   on_hand=30, was_update=false

4) ON CONFLICT DO NOTHING для существующего ключа (CENTRAL/ESP-01, 999):
   строк затронуто: 0 (конфликт проигнорирован, on_hand остался 80)

5) Итоговое состояние stock_levels:
   CENTRAL/ESP-01  on_hand=80
   NORTH/LAT-01  on_hand=30
```

(The demo prints in Russian.) The first upsert of key `CENTRAL/ESP-01` is an insert (`was_update=false`). The second with the same key is an update (`was_update=true`), and `on_hand` became `80`: `EXCLUDED.on_hand` (the new value) overwrote the old. `DO NOTHING` with stock `999` for an already-existing key did nothing — `0` rows, stock stayed `80`. No preliminary `SELECT`s, no race.

## The fence

`ON CONFLICT` solves precisely the race problem: the insert and the conflict check are one atomic operation at the engine level, so two concurrent upserts of the same key won't create a duplicate or fail — one inserts, the other updates. That's its main advantage over `SELECT`-then-`INSERT`. What to keep in mind:

- **In `DO UPDATE SET`, list only what you really want to overwrite** — it's easy to accidentally clobber `created_at` or a counter with a value from `EXCLUDED`.
- **`ON CONFLICT` targets one specific conflict** (one unique index). If a table has several unique constraints and a row can conflict on different ones, the logic gets more complex.
- **`MERGE` (PG15+) is more flexible but weaker under a race.** Multiple `WHEN MATCHED/NOT MATCHED` branches, but `MERGE` is **not** as race-safe as `ON CONFLICT` — we cover that and `COPY` for bulk loading in **09-01**.
- **A nightly sync in production is done in a batch**, not row by row: `COPY` into a temp table → one `INSERT ... SELECT ... ON CONFLICT`.

## Takeaways

- `INSERT ... ON CONFLICT (cols) DO UPDATE` — "insert or update" in one atomic, concurrency-safe command.
- The conflict is caught by a `UNIQUE`/`PRIMARY KEY` on the named columns — without such a constraint `ON CONFLICT` doesn't work.
- `EXCLUDED` is a pseudo-table with the row you tried to insert; `SET col = EXCLUDED.col` takes the new value (you can also accumulate, referring to the old one by the table name).
- `DO NOTHING` is an idempotent insert: insert if absent, otherwise silently skip (like dedup by `outbox_id` in `processed_outbox_ids`).
- The `(xmax <> 0)` trick distinguishes an update from an insert in `RETURNING`.

Next up — the **03-05 "RETURNING old/new"** unit: in PG18 `UPDATE ... RETURNING old.status, new.status` returns both the old and the new value of a row in one command — a ready-made audit of a transition with no separate `SELECT` and no trigger.
