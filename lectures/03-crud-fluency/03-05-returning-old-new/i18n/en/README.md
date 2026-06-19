# 03-05 — RETURNING old/new

A Brew order changes status: `created → paid → shipped`, and every transition sends the guest a push. Today Stas comes down from the marketing floor, phone screen toward you — a support ticket on it.

> **Stas:** The "order status changed" push goes out. What it changed to — the guest sees. What it changed from — nobody sees, and there's nothing to settle the "I paid and you cancelled" dispute with. Get me the "was."

Stas is right: for an audit trail and for notifications the app needs not just the new value but the **previous** one — "was `created`, became `paid`." Classically you reach for it two ways — either a `SELECT` of the status before the `UPDATE` (an extra query plus a race: between the read and the write the status could change), or a trigger that writes `OLD`/`NEW` into a separate table (powerful, but that's logic on the database side — see module 09).

PG18 added a third, direct path: in `RETURNING` you can now refer to the row **before** the change and **after** — via the `old.` and `new.` prefixes. `UPDATE ... RETURNING old.status, new.status` returns both values in one command, in the same transaction, with no second query and no trigger.

## old and new in RETURNING

Before PG18, `RETURNING` handed back the row in its final form (after the change). Now each column has two "versions":

- `new.col` — the value **after** the command (for `UPDATE`/`INSERT`);
- `old.col` — the value **before** the command (for `UPDATE`/`DELETE`).

Without a prefix (just `status`) the old behavior holds: for `UPDATE`/`INSERT` it's `new`, for `DELETE` it's `old`. You can also return expressions over both versions: `(old.paid_at IS NULL)`, `new.amount - old.amount`, and so on.

## The INSERT / UPDATE / DELETE symmetry

The prefixes reveal a neat symmetry of the three commands through "is there a row before and after":

- `INSERT` — there's no "before" row, so `old.*` is `NULL` throughout; `new.*` is what was inserted.
- `UPDATE` — both versions exist: `old.*` (before) and `new.*` (after).
- `DELETE` — there's no "after" row, so `new.*` is `NULL` throughout; `old.*` is what was deleted.

So `old`/`new` is a single language for "what was and what became" over any modifying command.

## The old/new matrix: symmetry of the three commands

Every row has two "versions" — before the command (`old`) and after (`new`). Which one exists is decided by the command itself:

```
            old.* (before)     new.* (after)
          ┌────────────────┬────────────────┐
 INSERT   │    ∅  none     │  the inserted  │  there was no "before" row
          ├────────────────┼────────────────┤
 UPDATE   │  the previous  │  the changed   │  both exist
          ├────────────────┼────────────────┤
 DELETE   │  the deleted   │    ∅  none     │  no "after" row remains
          └────────────────┴────────────────┘
              ▲                           ▲
    no prefix on DELETE → old   no prefix on INSERT/UPDATE → new
```

| Command | `old.*` (before) | `new.*` (after) | Column with no prefix |
|---|---|---|---|
| `INSERT` | `NULL` (no "before" row) | the inserted row | = `new` |
| `UPDATE` | the previous version | the changed version | = `new` |
| `DELETE` | the deleted row | `NULL` (no "after" row) | = `old` |

## Why this unit has no sqlc

The other CRUD units in the module are written with sqlc, but here sqlc gets in the way: its parser (version v1.30.0) doesn't yet know the PG18 `old.`/`new.` syntax and fails with `column "status" does not exist`. And the lesson is precisely about this feature. So the unit is an **escape-hatch**: we write the queries as strings and scan the result by hand via `pgx` (as in 00-04/00-06), with no `query.sql` and no generated `internal/db`. When a lesson needs a database capability the tool doesn't support yet, we choose the capability, not the tool.

## What our code shows

In `main.go` there are three modifying commands, each with `RETURNING old/new`. `UPDATE` (both versions exist):

```go
pool.QueryRow(ctx, `
    UPDATE order_status_lab SET status = 'paid', paid_at = now()
    WHERE id = 1
    RETURNING old.status, new.status,
              (old.paid_at IS NULL)     AS was_unpaid,
              (new.paid_at IS NOT NULL) AS now_paid`,
).Scan(&oldStatus, &newStatus, &wasUnpaid, &nowPaid)
```

And `INSERT` / `DELETE`, where one side is empty:

```sql
INSERT INTO order_status_lab (id, status) VALUES (4, 'created')
RETURNING old.status, new.status;   -- old.status → NULL (no "before" row)

DELETE FROM order_status_lab WHERE id = 2
RETURNING old.status, new.status;   -- new.status → NULL (no "after" row)
```

We print the "empty" side (`NULL`) as `∅`. The demo works on the lab table `order_status_lab`, which is recreated at the start (`CREATE IF NOT EXISTS` + `TRUNCATE` + three orders) — so the output is deterministic.

## Running it

Bring up the sandbox (from the repo root) and apply the Brew base schema:

```sh
docker compose up -d
make lecture L=03-crud-fluency/03-05-returning-old-new T=db-reset
make lecture L=03-crud-fluency/03-05-returning-old-new
```

(`T=run` is the default. From inside the unit directory it's `make db-reset`, `make run`.)

Output:

```
1) Стол order_status_lab засеян: заказы #1, #2, #3 в статусе 'created'.

2) UPDATE #1: created → paid (RETURNING old/new одним запросом):
   old.status=created  new.status=paid   было неоплачено=true  стало оплачено=true

3) INSERT #4 'created' (RETURNING old/new):
   old.status=∅  new.status=created   → на INSERT строки «до» нет, old.* пуст

4) DELETE #2 (RETURNING old/new):
   old.status=created  new.status=∅   → на DELETE строки «после» нет, new.* пуст
```

(The demo prints in Russian.) The `UPDATE` returned both the old status (`created`) and the new (`paid`), plus two predicates over both versions (`was_unpaid`, `now_paid`) — all in one query. The `INSERT` showed an empty `old.*` (there was no before row), the `DELETE` an empty `new.*` (no after row remained). That very before/after symmetry.

## The fence

`RETURNING old/new` gives "before and after" only for the rows **this command** touches, and only within its transaction — it's not a change log. Hence the boundaries:

- **It's not a substitute for an audit trail.** When history must be stored independently of which code did the write (for an `UPDATE` from anywhere, and for direct edits in `psql`), you use an `AFTER` trigger with `OLD`/`NEW` writing into an audit table — that's module **09-05**, where we also discuss when logic belongs in the database and when not.
- **`old`/`new` is a PG18 feature.** Older versions don't have it: the "before value" is obtained with a `SELECT` before the `UPDATE` (with a race risk) or with a trigger.
- **sqlc (v1.30.0) doesn't yet understand it** — so in production with sqlc such a query has to be written "raw" via `pgx` (as here) or wait for tool support.

## Takeaways

- PG18: in `RETURNING` you can refer to the row before the change (`old.col`) and after (`new.col`) — both values in one command.
- `UPDATE` gives both versions; on `INSERT` `old.*` is empty (no "before" row), on `DELETE` `new.*` is empty (no "after" row).
- Without a prefix a column behaves as before: `new` for `UPDATE`/`INSERT`, `old` for `DELETE`.
- This removes the "`SELECT` before `UPDATE`" (extra query + race) for auditing a transition within a single command.
- It's not a substitute for a full audit (trigger + history table, → 09-05) and isn't yet supported by sqlc — hence the raw-pgx in this unit.

Next up — the **03-06 "sober NULL semantics"** unit: the reckoning for the teaser from 01-02 — the `NOT IN` + `NULL` trap that silently returns "nothing," and the `COALESCE`/`NULLIF`/`IS DISTINCT FROM` tools for working with `NULL` safely.
