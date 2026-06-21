# 05-02 — The MVCC mental model

Brew is running a revenue report — a long analytical `SELECT` that reads a month of orders for a few seconds. At that exact moment a manager raises the cappuccino price through the admin panel. The question that decides whether you trust your database: what will the report see — the old price, the new one, or, heaven forbid, half the rows the old way and half the new? And why didn't the report grind to a halt, blocked by someone else's `UPDATE`?

The answer to both is one mechanism: **MVCC**, multiversion concurrency control. It isn't a setting or a feature you switch on — it's how Postgres stores rows at all and decides which version to show to whom. The person who needs to understand it isn't the admin, it's you: transactions, isolation, locking, and the whole of module 05 stand on it. Here's the essence, on two observable effects.

This is an escape-hatch unit: we need the system columns (`ctid`, `xmin`, `xmax`) and two live transactions side by side. `sqlc` is no help here — the lesson is taught with psql scripts directly. It's the first such unit in the course, and it sets the convention: **when a lesson needs interactivity, system columns, or concurrent sessions — write `.sql` for psql, not `query.sql` for codegen.**

## A snapshot instead of a lock

The naive model of a row is "a cell you overwrite with a new value." In Postgres that's not how it works. A row, physically, is a **version** (a tuple), and it carries hidden system columns:

- `xmin` — the id of the transaction that *created* this version;
- `xmax` — the id of the transaction that *superseded or deleted* it (0 while the version is live);
- `ctid` — the version's physical address: `(page_number, offset)`.

`UPDATE` does not touch the old version in place. It marks the old one dead (sets its `xmax`) and writes a **new** version of the row — with a new `ctid` and a new `xmin`. The old version lingers on the page as a "dead tuple" for a while, until `VACUUM` removes it.

The answer about the report grows straight out of this. Each transaction works against its own **snapshot**: a fixed set of "which versions are visible to me." Visibility is decided by comparing a version's `xmin`/`xmax` against that snapshot. So a reader never waits on a writer, nor a writer on a reader: they look at *different versions* of the same row. Brew's report sees the price that held at the moment of its snapshot — whole and consistent, with no "half the old way."

> **Botyr:** Hold on. `UPDATE` doesn't overwrite, `DELETE` doesn't delete… so the database just keeps growing?
>
> **Dmitry:** It grows. Dead versions sit there until `VACUUM` clears them.
>
> **Botyr:** And if it doesn't?
>
> **Dmitry:** Then Pavel shows up. Keep transactions short — and he won't.

## What our code shows

The lesson lives in two psql scripts. The first, `demo.sql`, shows the mechanics inside a single transaction — what `UPDATE` physically does to a row version. To keep `ctid`/`xmin` clean (not muddied by past transactions over the base tables), we work on a separate lab table `mvcc_lab`, which we drop at the end:

```sql
INSERT INTO mvcc_lab VALUES (2, 450);

BEGIN;
  CREATE TEMP TABLE _before AS SELECT ctid AS c, xmin AS x FROM mvcc_lab WHERE id = 2;
  UPDATE mvcc_lab SET price = price + 50 WHERE id = 2;   -- a new version, not an overwrite
  CREATE TEMP TABLE _after  AS SELECT ctid AS c, xmin AS x FROM mvcc_lab WHERE id = 2;
  SELECT (b.c <> a.c) AS ctid_changed, (b.x <> a.x) AS xmin_changed FROM _before b, _after a;
COMMIT;
```

`ctid_changed` and `xmin_changed` are both `t` — after the `UPDATE` the row with the same `id=2` sits at a different physical address and belongs to a different (the current) transaction. That's "a new version instead of an overwrite," seen by hand.

The second thread is the snapshot between two transactions. `session-a.sql` (a reader under `REPEATABLE READ`) and `session-b.sql` (a writer) run in two terminals. A takes a snapshot, B changes the price and commits, A reads a second time — within the same transaction:

```sql
-- session-a.sql
BEGIN ISOLATION LEVEL REPEATABLE READ;
SELECT base_price, xmin FROM drinks WHERE id = 2;   -- A1
\prompt '...run session-b, then Enter...' _
SELECT base_price, xmin FROM drinks WHERE id = 2;   -- A2: the same snapshot
COMMIT;
SELECT base_price, xmin FROM drinks WHERE id = 2;   -- A3: a new snapshot
```

The `\prompt` holds A's transaction open until you come back, so the order of steps is fixed, with no race. `REPEATABLE READ` pins the snapshot for the whole transaction: A2 must show the same as A1.

## Running it

Bring up the sandbox and apply the Brew base schema:

```sh
docker compose up -d
make lecture L=05-transactions-and-mvcc/05-02-mvcc-mental-model T=db-reset
```

The mechanics inside one transaction (`make run` — this unit's main demo):

```sh
make lecture L=05-transactions-and-mvcc/05-02-mvcc-mental-model
```

```
── Свежая строка: одна версия, ctid = физический адрес ───────────────
 id | price | ctid  | superseded 
----+-------+-------+------------
  2 |   450 | (0,1) | f
(1 row)


── UPDATE написал НОВУЮ версию строки (внутри той же транзакции) ──────
 ctid_changed | xmin_changed 
--------------+--------------
 t            | t
(1 row)


── Итог: одна логическая строка id=2 — но уже вторая физическая версия 
 id | price | ctid  | superseded 
----+-------+-------+------------
  2 |   500 | (0,2) | f
(1 row)
```

(The script prints its headers in Russian; `superseded` is `xmax <> 0`, `f` = the visible version is live.) Now the two sessions. In the **first** terminal start the reader — it will stop at the prompt:

```sh
make lecture L=05-transactions-and-mvcc/05-02-mvcc-mental-model T=session-a
```

In the **second** terminal, the writer; it runs to completion:

```sh
make lecture L=05-transactions-and-mvcc/05-02-mvcc-mental-model T=session-b
```

Go back to the first terminal and press Enter. Folded together, the steps give this picture (`xmin` is a transaction id — your numbers will differ; what matters is their *relationships*):

```
Terminal A (reader, REPEATABLE READ)          Terminal B (writer)
─────────────────────────────────────────     ──────────────────────────────
A1  base_price = 450,  xmin = 856
                                               B1  UPDATE → 500,  xmin = 863
                                               B2  COMMIT
A2  base_price = 450,  xmin = 856   ← A's snapshot unchanged: B's commit invisible
    COMMIT
A3  base_price = 500,  xmin = 863   ← a new snapshot: B's version now visible
```

A2, inside the open transaction, reads the old price with the old `xmin` even though B has already committed — A's snapshot was taken before that commit. After `COMMIT` A itself takes a new snapshot, and A3 shows both the new price and the new `xmin` (that's the version created by transaction B). After the demo, restore the menu: `make db-reset`.

## The fence: the simplification and where it bites

> **Pavel — in review, one line:** Your forgotten BEGIN is holding VACUUM across my whole database.

We showed `xmin`/`xmax`/`ctid` directly and said "the old version lingers and gets cleaned up by `VACUUM`." In production a whole class of problems sits behind that — problems your DBA handles, but the code provokes:

- Dead versions are **bloat**: tables and indexes swell until `autovacuum` clears the dead tuples. Frequent `UPDATE`s on the same row breed versions faster than you'd think.
- `VACUUM` can remove a version only once nobody can still see it. A **long open transaction** (the infamous `idle in transaction`, or a `BEGIN` forgotten in code) holds the visibility horizon and blocks cleanup across the whole database — even for tables it never touched.

The practical takeaway: transactions must be short, and a `BEGIN` without a prompt `COMMIT`/`ROLLBACK` is a bug, not a style. You don't read system columns by hand in an app; you need to know about them to understand *why* things behave this way.

## Takeaways

- In Postgres a row is a version with system columns `xmin`/`xmax`/`ctid`; `UPDATE` writes a new version instead of overwriting the old one.
- Each transaction sees its own snapshot; visibility is decided by `xmin`/`xmax`. So readers don't block writers and vice versa — they look at different versions.
- `REPEATABLE READ` pins the snapshot for the whole transaction: a commit by someone else that arrives after the snapshot stays invisible inside the transaction.
- The price of the model is dead versions and bloat; keep transactions short, or `VACUUM` can't clean up.
- A course convention: when you need interactivity, system columns, or two sessions — it's an escape-hatch unit on psql, not `query.sql` + sqlc.

The neighbouring unit **05-01 "Transactions and ACID"** lays the foundation beneath this: `BEGIN`/`COMMIT`/`ROLLBACK` and atomicity on a balance-transfer example. And **05-03** goes further into concurrency — row locks and lost updates (`FOR UPDATE`, `SKIP LOCKED`), also on two sessions. The snapshot you saw here is the shared vocabulary for the whole module.
