# 05-04 — Isolation levels in practice

A Brew shift has an unwritten rule: at least one barista must always be on the floor — the room can't go empty. Right now two are on the floor — baristas Alice and Boris (namesakes of the customers Alice Ivanova and Boris Petrov from other modules: different people, shift staff). Both decide to step into the stockroom — **at the same time**. Each glances at the room: "there are two of us, I'll step away, one stays." Each reasons flawlessly. But they leave together — and the room empties. No `UPDATE` "clobbered" another (Alice changed her own row, Boris his own), the row locks from 05-03 are useless here: they touch **different** rows. And yet the invariant is broken.

This is **write-skew** — an anomaly that neither `FOR UPDATE` (the rows differ) nor even the fixed snapshot of `REPEATABLE READ` catches. Only the strictest isolation level catches it — `SERIALIZABLE`. This unit is about the three available isolation levels, and what exactly sets the top one apart.

This is an escape-hatch unit (like 05-02): isolation anomalies are concurrent by nature, and we teach the lesson with psql scripts.

## The three levels worth knowing

The SQL standard describes four levels; Postgres really distinguishes three (its `READ UNCOMMITTED` is identical to `READ COMMITTED` — there is never a "dirty read" in Postgres).

- **READ COMMITTED** — the default. Each *statement* sees a fresh snapshot of committed data. Within one transaction two identical `SELECT`s can return different results if someone committed in between (a non-repeatable read). This level is enough for most web applications.
- **REPEATABLE READ** — the snapshot is fixed for the *whole* transaction (the mechanics were in 05-02). A repeated read is always identical; there are no "phantoms" at this level in Postgres either. But write-skew slips through.
- **SERIALIZABLE** — the strictest: the result of any group of concurrent transactions is guaranteed to match *some* serial order of them. It's implemented via SSI (Serializable Snapshot Isolation): the database tracks read/write dependencies and, on detecting a dangerous pair, aborts one transaction with error **40001** (`serialization_failure`). No extra locking — the price is that you must be ready to **retry** the transaction.

The level is set per transaction: `BEGIN ISOLATION LEVEL SERIALIZABLE;` (or `SET TRANSACTION ...` right after `BEGIN`).

The three levels and what each catches (there's no dirty read at any of them in Postgres):

| Level | Snapshot | Non-repeatable read | Write-skew | Cost |
|---|---|---|---|---|
| **READ COMMITTED** (default) | per statement | slips through | slips through | cheap |
| **REPEATABLE READ** | whole transaction | caught | slips through | cheap |
| **SERIALIZABLE** | transaction + SSI | caught | **caught** (`40001`) | retries under load |

## Why write-skew is sneaky

Each of the two transactions is, on its own, **correct**: it read "two on the floor," cleared one flag, left one — invariant satisfied. The trouble is that both read a state that was *stale by commit time*: by the time Alice commits, Boris has already left, but Alice's snapshot doesn't see it. `REPEATABLE READ` honestly gives each a stable snapshot — and that's exactly why it misses the problem. `SERIALIZABLE` notices: it sees that Alice read a row Boris modified, and vice versa — a "dangerous structure" with no compatible serial order, so one transaction has to be rejected.

## What our code shows

`demo.sql` (the `run` target) deterministically shows the available levels (`SHOW transaction_isolation` → `read committed`; `BEGIN ISOLATION LEVEL ...` → the chosen one) and the **logic** of write-skew in a single session: both baristas "see 2," both clear a flag, 0 are left on the floor.

`session-a.sql` / `session-b.sql` show the **live** conflict under `SERIALIZABLE` across two terminals:

```sql
-- both sessions:
BEGIN ISOLATION LEVEL SERIALIZABLE;
SELECT count(*) FROM shift_lab WHERE on_floor;     -- both see 2
-- A: UPDATE ... WHERE id = 1;   B: UPDATE ... WHERE id = 2;   -- different rows!
COMMIT;                                            -- whoever commits second catches 40001
```

Boris (B) commits first — successfully. Alice (A) commits second — her `COMMIT` fails with 40001, the transaction is aborted in full, and Alice stays on the floor. The invariant is saved at the cost of one rejected transaction.

## Running it

Bring up the sandbox (from the repo root) and restore the Brew base schema:

```sh
docker compose up -d
make lecture L=05-transactions-and-mvcc/05-04-isolation-levels-for-devs T=db-reset
```

The deterministic demo (levels + write-skew logic):

```sh
make lecture L=05-transactions-and-mvcc/05-04-isolation-levels-for-devs
```

```
── Уровень изоляции по умолчанию (дефолт Postgres) ──
 transaction_isolation 
-----------------------
 read committed
(1 row)


── Уровень задаётся на транзакцию через BEGIN ISOLATION LEVEL ... ──
 внутри BEGIN REPEATABLE READ 
------------------------------
 repeatable read
(1 row)

 внутри BEGIN SERIALIZABLE 
---------------------------
 serializable
(1 row)


── Write-skew: правило «на полу всегда ≥1 бариста». На полу сейчас:
 на полу 
---------
       2
(1 row)


Алиса смотрит «сколько на полу» (видит 2 ≥ 1 → решает уйти):
 Алиса видит на полу 
---------------------
                   2
(1 row)

Борис смотрит ОДНОВРЕМЕННО, по своему снимку (тоже видит 2 → тоже решает уйти):
 Борис видит на полу 
---------------------
                   2
(1 row)


Итог — на полу не осталось никого, хотя каждый «оставлял одного»:
 на полу 
---------
       0
(1 row)

→ инвариант сломан. READ COMMITTED и REPEATABLE READ это пропускают;
  ловит только SERIALIZABLE — он завершит одну из транзакций ошибкой 40001 (см. сессии).
```

Now the live conflict. In the **first** terminal run `session-a`: Alice opens a `SERIALIZABLE` transaction, reads "2 on the floor," clears her flag, and stops at the prompt. **At that moment** in the **second** terminal run `session-b` in full — Boris reads "2 on the floor," clears his flag, and commits **successfully**. Return to the first terminal, press Enter — and Alice's `COMMIT` fails:

```
A3) Алиса коммитит ВТОРОЙ. SERIALIZABLE видит: A и B прочитали одно множество,
    а сняли РАЗНЫЕ флаги — вместе они нарушили бы «≥1 на полу». COMMIT падает 40001:
ERROR:  could not serialize access due to read/write dependencies among transactions
DETAIL:  Reason code: Canceled on identification as a pivot, during commit attempt.
HINT:  The transaction might succeed if retried.

A4) Транзакция A отменена целиком — её UPDATE не применён. На полу всё ещё есть Алиса:
 id | name  | on_floor 
----+-------+----------
  1 | Алиса | t
  2 | Борис | f
(2 rows)
```

`HINT: ... might succeed if retried` is exactly the `SERIALIZABLE` contract: catch 40001 and **retry**. On retry Alice reads the now-fresh state (one on the floor) and doesn't leave. (The exact `DETAIL`/reason-code text may vary from run to run — what matters is the code `40001`.)

## The fence

The commit order in the sessions is held by `\prompt` — in a real race "the second" could be either transaction, and the 40001 would land on an unpredictable one. That's why `SERIALIZABLE` can't be used without a **retry loop**: code that runs under it must be able to retry the transaction on 40001 (that's the next unit, 05-05). Here's what else to keep in mind:

- **`SERIALIZABLE` is not a free "correctness mode."** Under load it produces more serialization failures, and thus retries and wasted work. You usually don't enable it across the whole database — it's applied selectively, to transactions with genuine write-skew risk: bookings, limits, on-call schedules.
- **The same anomaly can be closed on `READ COMMITTED` — by hand.** Materialize the conflict with `SELECT … FOR UPDATE` on a "control" row, add a `CHECK`/unique index, or take an explicit lock. Which to pick depends on how often the conflict actually happens.

## Takeaways

- The isolation level is set **per transaction** (`BEGIN ISOLATION LEVEL ...`); the Postgres default is `READ COMMITTED`.
- `READ COMMITTED` — snapshot per statement; `REPEATABLE READ` — snapshot per transaction; `SERIALIZABLE` — as if transactions ran one at a time.
- **Write-skew**: two transactions read a shared set, write to *different* rows, each correct on its own — yet together they break an invariant.
- Neither `FOR UPDATE` (different rows) nor `REPEATABLE READ` (stable snapshot) catches write-skew — only `SERIALIZABLE` does, at the cost of error **40001** on one of the transactions.
- `SERIALIZABLE` requires a **retry loop** on 40001 (`serialization_failure`) — you can't use it without one.

Next is **05-05 "retrying on 40001"**: we'll write, in Go, exactly that loop that retries the transaction on a serialization failure, and see how the second attempt — on a fresh snapshot — makes the right decision.
