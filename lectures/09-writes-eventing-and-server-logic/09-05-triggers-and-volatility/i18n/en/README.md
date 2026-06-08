# 09-05 — Triggers and function volatility

Brew has two recurring pains. The first: the `updated_at` column. A backend
developer updated an order and forgot to set `updated_at` — and now the row's
"changed at" time lies, and data-freshness analytics are broken. We want this
column filled automatically, without relying on the discipline of everyone who
writes an `UPDATE`. The second: auditing. A regulator asks for a "who changed what
in prices" log: was 480, became 500. Writing a log entry in every place in the
code where a price changes means forgetting it somewhere sooner or later.

Both pains are closed by server-side logic — **triggers**: a function the database
calls itself on every `INSERT`/`UPDATE`/`DELETE`. And talking about functions runs
into the notion of **volatility** — a promise to the database about how
predictable a function is.

## A BEFORE trigger: edits the row before it is written

A `BEFORE` trigger fires *before* the row hits disk and may change it — for that it
returns a modified `NEW`. The classic case is auto-filling `updated_at`:

```sql
CREATE FUNCTION set_updated_at() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at := now();   -- change the row BEFORE the write
    RETURN NEW;                -- a BEFORE trigger must return the row to write
END;
$$;
CREATE TRIGGER touch_lab_bupd
    BEFORE UPDATE ON touch_lab
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
```

Now any `UPDATE` of the row sets `updated_at = now()` itself, and "forgetting" the
column is physically impossible — the filling moved from application code into a
table invariant. Returning `NEW` is mandatory: that is exactly what the database
writes.

## An AFTER trigger: sees OLD and NEW, writes the audit

An `AFTER` trigger fires *after* the write; it is too late to change the row, and
its return value is ignored. But it has access to **both** versions of the row:
`OLD` (as it was) and `NEW` (as it became). That is the material for an audit. By
`TG_OP` (the operation type) the trigger decides what to record in the log:

```sql
IF TG_OP = 'INSERT' THEN ...     -- no OLD (the row didn't exist before)
ELSIF TG_OP = 'UPDATE' THEN ...  -- both OLD and NEW
ELSIF TG_OP = 'DELETE' THEN ...  -- no NEW (nothing will remain)
```

An important asymmetry: `INSERT` has no `OLD` (there was nothing), `DELETE` has no
`NEW` (nothing will remain). In the log this shows as `∅`. An echo of 03-05: there
`RETURNING old/new` handed both versions to the calling query; here the trigger
catches them on the database side and writes them regardless of who fired the
`UPDATE` or from where.

```
What a trigger has by TG_OP:
  INSERT   OLD = ∅            NEW = {new row}      the row didn't exist before
  UPDATE   OLD = {as it was}  NEW = {as it became} both versions present
  DELETE   OLD = {as it was}  NEW = ∅              the row won't remain
```

## Volatility: a promise to the planner

Every function in Postgres carries a **volatility** label — a promise to the
planner about how predictable it is:

| Label | Promise | Examples | What it gives the planner |
|---|---|---|---|
| `IMMUTABLE` | same inputs → always the same output | `lower()`, pure arithmetic | evaluate once and substitute as a constant |
| `STABLE` | doesn't change within a single query | `now()`, reading tables | call it less often within a query |
| `VOLATILE` | may return something different on every call | `random()`, writing to a table | nothing (this is the **default**) |

The label is a promise the planner leans on; lying in it is dangerous (see the
fence). The most visible consequence: only `IMMUTABLE` functions are allowed in an
**index expression**. Logical: an index stores the computed value, and if the
function could return something else for the same data, the index would
immediately go stale.

## What our code shows

This is an escape-hatch psql unit (like 05-02/08-04): the topic is server-side DDL
and PL/pgSQL, sqlc doesn't apply. `demo.sql` has three parts: a BEFORE trigger
bumps `updated_at` itself; an AFTER trigger writes an audit with `OLD`/`NEW` for
INSERT/UPDATE/DELETE; then three functions labeled `IMMUTABLE/STABLE/VOLATILE` —
their classification is read from the `pg_proc` catalog, and the attempt to build
an index on a `VOLATILE` function is caught as an error. The `f_vol_int` function
is deliberately `plpgsql`: a trivial `sql` function would be inlined into the
expression and the label would be lost — `plpgsql` functions are not inlined, so
the label stands.

## Running it

```sh
docker compose up -d
make lecture L=09-writes-eventing-and-server-logic/09-05-triggers-and-volatility T=db-reset
make lecture L=09-writes-eventing-and-server-logic/09-05-triggers-and-volatility
```

`T=run` is the default and can be omitted. From inside the unit directory it is
shorter: `make db-reset`, then `make run`. The raw error text of step 3 goes to
stderr; the stdout below keeps the deterministic SQLSTATE.

```
1) BEFORE-триггер сам проставил updated_at на UPDATE (печатаем факт, не значение):
 id |       name       | updated_at_bumped 
----+------------------+-------------------
  1 | Эспрессо (1 шот) | t


2) AFTER-триггер записал аудит (∅ = значения нет: OLD в INSERT, NEW в DELETE):
   op   | old_name | new_name | old_price | new_price 
--------+----------+----------+-----------+-----------
 INSERT | ∅        | Латте    | ∅         | 480
 UPDATE | Латте    | Латте    | 480       | 500
 DELETE | Латте    | ∅        | 500       | ∅


3) Как Postgres классифицировал наши функции (provolatile из каталога):
 proname | volatility 
---------+------------
 f_imm   | IMMUTABLE
 f_stb   | STABLE
 f_vol   | VOLATILE


   f_imm (IMMUTABLE) в индексном выражении — можно:
          result           
---------------------------
 индекс по f_imm(n) создан

   f_vol_int (VOLATILE) в индексном выражении — нельзя (сырой текст ошибки в stderr):
SQLSTATE = 42P17 (functions in index expression must be marked IMMUTABLE)
```

The BEFORE trigger raised `updated_at` itself (flag `t`). The AFTER trigger wrote
three audit rows, and the asymmetry shows in them: INSERT has an empty old value,
DELETE an empty new one. The functions are classified as `i/s/v`, and the index on
the `IMMUTABLE` one built, while the one on the `VOLATILE` function was rejected
with code `42P17`.

## The fence: when NOT to put logic in the database

- **The logic becomes invisible.** A plain `UPDATE` quietly drags along an audit
  write, an `updated_at` bump, maybe a `NOTIFY` (09-04) — a developer reading the
  application code won't know until they hit an unexpected effect. Triggers are
  hard to test, hard to version alongside the code, and a cascade of "trigger fires
  trigger" is painful to debug.
- **The boundary: invariants in the DB, business logic in the app.** Keep triggers
  for **data invariants** (updated_at, auditing, integrity checks that must hold
  regardless of which service writes). Leave the **business logic** (compute a
  discount, decide whether an order can ship, call a payment gateway) in the
  application: there it is visible, easy to test, and it doesn't block writes to
  the table during execution. A rule of thumb: "mandatory for EVERYONE who touches
  this table, and about integrity" → fine in the DB; "depends on the scenario,
  calls outward, changes often" → in the application.
- **Don't lie in the volatility label.** Mark a function `IMMUTABLE` but read a
  table inside it — the planner caches the first result and keeps handing back a
  stale one; such a bug doesn't reproduce "out of nowhere" and takes days to find.
  The label must be honest: a function that reads data is at most `STABLE`, one
  that writes or is nondeterministic is `VOLATILE`.
- **The depth of PL/pgSQL** (cursors, exceptions, dynamic SQL, the performance of
  server-side functions) is a separate large topic at the seam with your DBA; in a
  course for developers we keep server-side logic at "medium" depth.

## Takeaways

Triggers move an invariant from the level of developer discipline to the level of
the table: `BEFORE` edits the row before the write (auto-filling `updated_at`,
returning `NEW` is mandatory), `AFTER` sees `OLD` and `NEW` and is ideal for
auditing (INSERT has no `OLD`, DELETE has no `NEW`). Volatility is a promise to the
planner: `IMMUTABLE`/`STABLE`/`VOLATILE`, and only `IMMUTABLE` is allowed in an
index expression; you must not lie in the label. And the module's main rule:
server-side logic is for **data invariants**, not for business logic; what depends
on the scenario and calls outward lives in the application, where it is visible and
testable.

That closes module 09: we learned to write in batches (`MERGE`/`COPY`), hand out
work (`SKIP LOCKED`), reliably produce events (`outbox`), push signals (`NOTIFY`),
and move invariants into the DB (triggers). Next is module 10, the capstones: all
of this ties into end-to-end scenarios, including `10-05`, where our `outbox` and
the canon travel via CDC into the sibling `kafka-cookbook`.
