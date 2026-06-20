# 03-03 — UPDATE/DELETE safely

On your screen — an `UPDATE` on orders. Botyr spins his chair around before you can hit Enter.

> **Botyr:** Friday, evening, an UPDATE on orders. My most expensive incident started exactly like that.
>
> **You:** The one? A single line?
>
> **Botyr:** A single line. `UPDATE orders SET status = 'cancelled'` — the `WHERE` stayed in my head. Zero errors — and every Brew order cancelled. Every one. Before your time: calls from the shop, a backup restore until midnight.
>
> **Dmitry:** The query was syntactically flawless — that's the horror. The database did exactly what it was told.
>
> **You:** And how do you see the blast radius before it's forever?
>
> **Dmitry:** Between the command and "forever" there's a window. Today we learn how to open it.
>
> **Botyr:** Ever since, my fingers hit BEGIN before my head finishes the thought. An expensive habit — as in, it cost a lot.

This unit builds the habits that turn this mistake from a catastrophe into a harmless typo: know the **blast radius** of a change (how many rows are hit and which ones) and run risky writes inside a **transaction** you can roll back until you've confirmed the right rows are hit.

## Blast radius: RETURNING and RowsAffected

`UPDATE` and `DELETE`, like `INSERT`, support `RETURNING` (see 03-01). On a write it's especially valuable: `UPDATE ... RETURNING` hands back **exactly the rows that changed** — not "some number," but the concrete list. If you expected three coffee items to be hit and 300 rows came back, something went wrong, and you see it immediately.

When you don't need the rows themselves, their count is enough. The driver returns a `CommandTag` with the number of affected rows; in sqlc a query with the `:execrows` suffix hands that number back directly as an `int64`. `RowsAffected` is the "blast radius" of a command: 1 — you fixed one row; a number the size of the whole table — you forgot the `WHERE`.

## Safety: a transaction as a safety net

Knowing the blast radius is little use if the change is already on disk. So a risky write is wrapped in a transaction: `BEGIN`, the command, a **check** (`RowsAffected`/`RETURNING`), and only then `COMMIT` — or `ROLLBACK` if the number is wrong. Until `COMMIT` the changes aren't visible to other sessions and aren't committed; `ROLLBACK` puts everything back as it was, as if the command never happened.

In Go that's `pool.Begin(ctx)` → `tx`; the sqlc-generated methods bind to the transaction via `queries.WithTx(tx)`, so all queries inside go in one transaction. `tx.Rollback(ctx)` rolls back. A handy trick is `defer tx.Rollback(ctx)` right after `Begin`: if the function exits early (an error, a panic), the transaction is guaranteed to roll back; the explicit `Commit`/`Rollback` below simply decides its fate.

## The undo window: a transaction timeline

`BEGIN` opens a "draft": the command runs, but until `COMMIT` no one sees it and it isn't on disk. Between the command and `COMMIT` there's a window where you can check the blast radius and reconsider:

```
BEGIN  ── "draft": changes aren't on disk and aren't visible to other sessions
  │
  ▼
UPDATE price_lab SET price = price + 50      ← no WHERE: hit the WHOLE table
  │
  ▼
check: RowsAffected = 5                       ← expected 3, got 5 — something's wrong
  │
  ├──────────────┬──────────────────
  ▼              ▼
ROLLBACK       COMMIT
as if no        changes committed,
rows changed    visible to all
(our choice)
```

Until `COMMIT` other sessions see the previous data; `ROLLBACK` puts everything back as it was, as if the command never happened. That window is the safety net: a forgotten `WHERE` in it is an observation, not a catastrophe.

## What our code shows

The queries in `query.sql` are two "blast radii" and one catastrophe. A targeted `UPDATE` with `RETURNING`:

```sql
-- name: RaiseCategory :many
UPDATE price_lab SET price = price + sqlc.arg(delta)
WHERE category = sqlc.arg(category)
RETURNING id, name, price;
```

And the "forgotten `WHERE`" — an `UPDATE` with no condition and a `DELETE`, both in the `:execrows` form (returning a row count):

```sql
-- name: RaiseAll :execrows
UPDATE price_lab SET price = price + sqlc.arg(delta);     -- no WHERE → the whole table
-- name: DeleteCategory :execrows
DELETE FROM price_lab WHERE category = sqlc.arg(category);
```

In `main.go` we seed the lab table `price_lab` (5 rows), do a safe targeted `UPDATE` (seeing 3 changed rows via `RETURNING`), and then inside a transaction run the "forgotten `WHERE`" and a `DELETE`, print their `RowsAffected` — and `ROLLBACK`. After the rollback the table's state is exactly as it was before the catastrophe.

## Running it

Bring up the sandbox (from the repo root) and apply the Brew base schema plus the unit's table:

```sh
docker compose up -d
make lecture L=03-crud-fluency/03-03-update-delete-safely T=db-reset
make lecture L=03-crud-fluency/03-03-update-delete-safely
```

(`T=run` is the default. From inside the unit directory it's `make db-reset`, `make run`.)

Output:

```
1) price_lab засеян (5 строк):
   #1 Эспрессо coffee 3.00
   #2 Капучино coffee 4.50
   #3 Латте coffee 4.80
   #4 Колд брю cold 5.20
   #5 Зелёный чай tea 2.50

2) Целевой UPDATE ... WHERE category='coffee' SET price+=50, RETURNING изменённое:
   #1 Эспрессо 3.50
   #2 Капучино 5.00
   #3 Латте 5.30
   (RETURNING показал ровно 3 затронутые строки)

3) «Забыл WHERE» внутри транзакции — смотрим масштаб и откатываем:
   UPDATE без WHERE затронул бы строк: 5 (вся таблица!)
   DELETE WHERE category='coffee' затронул бы строк: 3
   → ROLLBACK: ни одно изменение не применено.

4) Состояние после ROLLBACK — как в шаге 2 (5 строк, кофе +50, остальное нетронуто):
   #1 Эспрессо coffee 3.50
   #2 Капучино coffee 5.00
   #3 Латте coffee 5.30
   #4 Колд брю cold 5.20
   #5 Зелёный чай tea 2.50
```

(The demo prints in Russian.) The targeted `UPDATE` raised the price of three coffees and returned exactly those three rows. The "forgotten `WHERE`" inside the transaction showed its blast radius — 5 rows under `UPDATE`, 3 under `DELETE` — but `ROLLBACK` undid everything: step 4 shows only the three coffees from step 2 are affected, the rest intact. The transaction turned an incident into an observation.

## The fence

Here we roll back the catastrophe **ourselves**, because we know in advance it will happen. In production you won't always notice a forgotten `WHERE` — so people rely not on vigilance but on barriers:

- **Barriers instead of attentiveness.** Review of migrations and write scripts, a run on staging, and for interactive `psql` a non-auto-commit mode (`\set AUTOCOMMIT off`, and then every `UPDATE`/`DELETE` waits for an explicit `COMMIT`).
- **Our blast radius is a toy.** `price_lab` is tiny, and a whole-table `UPDATE`/`DELETE` is instant; on a large table a mass write is also a long row lock (other transactions wait) and bloat (an `UPDATE` in MVCC creates new row versions), covered in module 05 and `VACUUM`.
- **`RETURNING` on a mass write is traffic.** It drags all changed rows into the app; on a million rows you use `:execrows` (just the count) or work in batches.
- **Dangerous `DELETE`s are often replaced with "soft delete"** (a `deleted_at` flag) so data can be brought back.

## Takeaways

- `UPDATE`/`DELETE` without a `WHERE` hit the **whole** table — and do it without errors; the database executes exactly what it was asked.
- Always know the blast radius: `RETURNING` shows which rows are affected; `:execrows` (RowsAffected) shows how many.
- Run risky writes inside a transaction: `BEGIN` → command → check → `COMMIT`/`ROLLBACK`. A forgotten `WHERE` then rolls back.
- In Go: `pool.Begin` → `queries.WithTx(tx)` → `tx.Commit`/`tx.Rollback`; `defer tx.Rollback(ctx)` right after `Begin` is insurance against an early exit.
- `RETURNING` on a mass write drags all rows into the app — for a count use `:execrows`.

Next up — the **03-04 "upsert via ON CONFLICT"** unit: we'll learn to "insert or update" in one command — the idiom for syncing reference data and counters, where rows now appear, now change.
