# 05-06 — Deadlocks and advisory locks

Two Brew processes handle returns. The first locks the order row, then reaches for the item row. The second does the reverse: item first, then order. Each is flawless on its own. But if they start at the same time, the worst happens: the first holds the order and waits for the item, the second holds the item and waits for the order. Each waits for what the other holds. Neither will let go first — they're stuck forever. This is a **deadlock**.

The good news: Postgres won't hang. It detects the lock cycle on its own (on the `deadlock_timeout` timer, 1 second by default) and breaks it — it picks a "victim" and aborts its transaction with error `40P01` (`deadlock_detected`); the other proceeds. This unit is about two things: `40P01` (what it is and where it comes from) and `pg_advisory_lock` — the application-level lock most often used to **prevent** deadlocks.

This is an escape-hatch unit (like 05-02): a deadlock is concurrent by nature, so we teach the lesson with psql scripts.

## The deadlock and how to prevent it

A deadlock needs a "cross": two transactions grabbing the same resources in **opposite** order. Hence the main cure — a **single locking order**. If both return-processes always take the order first, then the item (say, by ascending id), no cycle forms: whoever took the order first will calmly take the item too, and the second waits and follows. Ninety percent of application deadlocks are cured by the discipline of "lock resources in the same order."

```
   Transaction A ──holds──▶ row #1
       │                       ▲
      wants                  wants
       ▼                       │
   row #2 ◀──holds── Transaction B
```

A holds `#1` and wants `#2`; B holds `#2` and wants `#1` — the "wants" arrows point into each other, and that's the deadlock "cross." Each waits for what the other holds, the cycle is closed → `40P01`. Make both transactions take resources in the same order (`#1` first, then `#2`) and the "wants" arrows no longer cross: no cycle.

`40P01` is a transient error, just like `40001` from 05-04/05-05: the victim only needs to **retry** the transaction (the same retry loop). But unlike a serialization failure, a deadlock is almost always a sign of a broken locking order, not an inevitability; the retry saves you here and now, but the fix is the order.

## The advisory lock: a lock for logic, not for data

Sometimes you need to serialize not a row but an **operation**: "let only one worker recompute shop #7's stock at a time," even if it touches dozens of rows. Cobbling together a lock on some single "sentinel" row for this is fragile. Postgres gives you a proper tool — the **advisory lock**: a named latch on an arbitrary 64-bit key whose meaning only the application knows.

```sql
SELECT pg_try_advisory_lock(42);  -- t — acquired, f — busy (no waiting)
-- ... critical section ...
SELECT pg_advisory_unlock(42);    -- released
```

Advisory locks come in two lifetimes: **session-level** (`pg_advisory_lock`/`pg_try_advisory_lock`) lives until an explicit unlock or the end of the connection, and **transaction-scoped** (`pg_advisory_xact_lock`) — released automatically on `COMMIT`/`ROLLBACK`. The second is safer: you can't forget to release it. And a single advisory lock used as a "gate" can also prevent our deadlock: if both transactions first take a shared `pg_advisory_xact_lock(returns_key)`, they line up and never clash on the rows at all.

The three advisory-API functions — by waiting and lifetime:

| Function | Waits if busy? | Lifetime | Released |
|---|---|---|---|
| `pg_advisory_lock(key)` | yes, blocks | session-level | by an explicit `pg_advisory_unlock` or end of connection |
| `pg_try_advisory_lock(key)` | no — returns `t`/`f` at once | session-level | same |
| `pg_advisory_xact_lock(key)` | yes, blocks | transaction | itself, on `COMMIT`/`ROLLBACK` |

## What our code shows

`demo.sql` (the `run` target) is a deterministic tour of the advisory-lock API in a single session: acquire/re-acquire (reentrancy), release the right number of times (an extra unlock → `f` + a WARNING), and the transaction-scoped lock that releases itself on `COMMIT`.

`session-a.sql` / `session-b.sql` are a live deadlock across two terminals: A takes row `#1` then reaches for `#2`, B takes `#2` then reaches for `#1`. The cycle closes — and Postgres aborts one of the transactions with `40P01`.

## Running it

Bring up the sandbox (from the repo root) and apply the Brew base schema:

```sh
docker compose up -d
make lecture L=05-transactions-and-mvcc/05-06-deadlocks-and-advisory-locks T=db-reset
```

The deterministic advisory-lock API demo:

```sh
make lecture L=05-transactions-and-mvcc/05-06-deadlocks-and-advisory-locks
```

```
── Берём session-level advisory-лок по ключу 42 ──
 got_42 
--------
 t
(1 row)


── Та же сессия берёт ключ 42 повторно (реентрабельно) → счётчик = 2 ──
 got_42_again 
--------------
 t
(1 row)


── Отпускаем дважды (t, t); третий unlock → f (лок уже не наш; +WARNING в stderr) ──
 unlock_1 
----------
 t
(1 row)

 unlock_2 
----------
 t
(1 row)

 unlock_3 
----------
 f
(1 row)


── Транзакционный лок по ключу 7: живёт до COMMIT, освобождается сам ──
 pg_advisory_xact_lock 
-----------------------
 
(1 row)

 held_now 
----------
        1
(1 row)

 held_after_commit 
-------------------
                 0
(1 row)

→ held_now=1 (внутри tx), held_after_commit=0 (COMMIT снял лок автоматически).
```

Now the live deadlock. In the **first** terminal run `session-a`: A takes row `#1` and stops at the prompt. In the **second** terminal run `session-b` up to its prompt — B takes row `#2`. Return to A, press Enter — A reaches for `#2` and **hangs** (B holds it). Return to B, press Enter — B reaches for `#1`, the cycle closes, and Postgres breaks the deadlock:

```
A2) Тянемся за строкой #2 (её держит B) → A ЗАВИСАЕТ в ожидании.
    Как только B потянется за строкой #1, цикл замкнётся, и Postgres разорвёт дедлок:
ERROR:  deadlock detected
DETAIL:  Process 1863 waits for ShareLock on transaction 832; blocked by process 1864.
Process 1864 waits for ShareLock on transaction 831; blocked by process 1863.
HINT:  See server log for query details.
CONTEXT:  while updating tuple (0,2) in relation "lock_lab"
```

Postgres picked the victim on its own (here, A) and rolled back its transaction; B proceeded and committed. (The process/transaction numbers in `DETAIL` will be your own, and the victim may turn out to be B — that's the scheduler's choice.) After the demo, re-apply the Brew base schema: `make ... T=db-reset`.

## The fence

The step order in the sessions is held by `\prompt` — in a real race everything happens in milliseconds, and Postgres decides which transaction to make the victim (usually the one that's "cheaper" to roll back). Here's what to keep in mind:

- **A deadlock isn't limited to rows.** Unindexed foreign keys, escalation at the `ALTER TABLE` level, even advisory locks taken crosswise — anywhere there are locks.
- **Fight it with prevention, not the retry.** A single acquisition order, short transactions, indexes under FKs (see 06). The `40P01` retry is insurance, not a strategy.
- **`40001` and `40P01` are different in nature.** `40001` (05-04/05-05) is about *logical* serialization conflicts and can't be prevented by lock order. `40P01` is about a *physical* wait cycle, and that one is exactly what lock order cures.
- **Advisory locks aren't free either.** A session-level lock easily "leaks" (you forgot to unlock, or the connection returned to the pool with a lock still held) — so applications almost always take `pg_advisory_xact_lock`, which releases itself.

## Takeaways

- A **deadlock** is two transactions waiting on resources each holds, grabbed in opposite order; neither can proceed.
- Postgres detects the deadlock on its own (via `deadlock_timeout`) and breaks it: the victim gets `40P01` (`deadlock_detected`), the other proceeds.
- The preventive cure is a **single locking order** (e.g. by ascending id); the `40P01` retry is insurance, not a substitute for discipline.
- An **advisory lock** (`pg_advisory_lock` / `pg_advisory_xact_lock`) is an application-level lock on a numeric key, not tied to rows: it serializes an *operation*. The transaction-scoped variant releases itself on `COMMIT` — prefer it.
- `40P01` (a physical wait cycle) is cured by lock order; `40001` (a logical serialization conflict) is not. Both are transient and both are retried.

This is the last unit of module 05. Next is module **06 "Indexing and EXPLAIN"**: with locks and snapshots covered, it's time to understand how Postgres *finds* rows and to learn to read a query plan (`EXPLAIN ANALYZE` with buffers — on by default in PG18).
