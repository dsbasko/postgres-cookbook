# 05-03 — Row locks and lost updates

Every Brew shop runs a barista app: it shows a drink's stock and decrements it by one on each sale. The code looks innocent: read `on_hand`, subtract 1, write it back. On a single register it works. But at rush hour two baristas at two registers sell the last cold brew **at the same time**: both read stock `10`, both write `9`. Two sold — stock dropped by one. One decrement is **lost**. A week later the inventory count doesn't add up, and no one knows why.

This is a **lost update** — the classic concurrency bug. Its root is the "read into the app, compute, write back" pattern (read-modify-write): between the read and the write, the value goes stale. This unit covers three ways to deal with it, from the simplest to the most general.

This is an escape-hatch unit (like 05-02): the lesson is about two concurrent sessions, and we teach it with psql scripts, not `query.sql` + codegen.

## Fix 1: let the database do the arithmetic

The most common read-modify-write isn't needed at all. Instead of "read `on_hand`, subtract in Go, write," put the arithmetic right into the `UPDATE`:

```sql
UPDATE seat_lab SET on_hand = on_hand - 1 WHERE id = 1;
```

Now the read and the write are **one** command. For the duration of that command Postgres takes a row lock (implicitly, on its own), and a concurrent `UPDATE` of the same row **waits** for it to finish, then computes off the fresh value. There's no stale read — nothing to lose. Ninety percent of "lost updates" in applications are cured by moving the computation out of code into a single atomic `UPDATE`.

## Fix 2: FOR UPDATE, when read-modify-write is unavoidable

Sometimes you genuinely need app logic between the read and the write: check a limit, call a payment gateway, make a decision. Then you lock the row **explicitly** — at read time:

```sql
BEGIN;
SELECT on_hand FROM seat_lab WHERE id = 1 FOR UPDATE;  -- locked the row until COMMIT
-- ... app logic ...
UPDATE seat_lab SET on_hand = on_hand - 1 WHERE id = 1;
COMMIT;                                                 -- lock released
```

`SELECT ... FOR UPDATE` takes the same row lock an `UPDATE` would, but **earlier** — at the read — and holds it until the end of the transaction. A competitor reading the same row with `FOR UPDATE` blocks and waits for your `COMMIT`. When it gets control, it reads the already-updated value. The price: competitors queue up on a hot row.

## SKIP LOCKED: a job queue without a waiting line

Sometimes queueing up is exactly what you don't want. Picture a job table (`job_queue`) and a pool of workers: each wants to grab a **free** job, not wait for the one a neighbor already took. Here you add `SKIP LOCKED` to `FOR UPDATE`:

```sql
SELECT id, payload FROM job_queue
WHERE status = 'pending'
ORDER BY id
FOR UPDATE SKIP LOCKED
LIMIT 1;
```

`SKIP LOCKED` means "don't wait on locked rows — skip them and take the next free one." Ten workers running the same query will pick up ten **different** jobs; nobody waits on anybody and nobody processes someone else's job. This is the idiomatic job queue on plain Postgres (we'll come back to it in 09-02).

The three fixes in one table — from the most common to the most special:

| Fix | How | Competitor on the same row | When to reach for it |
|---|---|---|---|
| **Atomic `UPDATE`** (`SET x = x - 1`) | read and write in one command, implicit row lock | waits for the command, computes off the fresh value | by default: the computation fits in SQL |
| **`SELECT … FOR UPDATE`** | explicit row lock at the read, held until `COMMIT` | waits for your `COMMIT`, reads the updated value | read-modify-write is unavoidable: logic between read and write |
| **`FOR UPDATE SKIP LOCKED`** | skips locked rows, takes the next free one | doesn't wait — takes a different row | job queue: N workers split the work with no duplicates |

## What our code shows

`demo.sql` (the `run` target) reproduces the lost-update arithmetic in a single session: two "workers" capture stock `10` into psql variables **before** any write (simulating simultaneity), both write `9` — and we see the loss. Then the same two sales through an atomic `UPDATE` give the correct stock `8`, and `FOR UPDATE` shows the explicit lock.

`session-a.sql` / `session-b.sql` are a live `SKIP LOCKED` queue across two terminals. A grabs job `#1` and holds it (transaction open); B runs the same claim query — and `SKIP LOCKED` steers it away from the locked `#1` to `#2`, with no wait.

## Running it

Bring up the sandbox (from the repo root) and restore the canon:

```sh
docker compose up -d
make lecture L=05-transactions-and-mvcc/05-03-row-locks-and-lost-updates T=db-reset
```

The deterministic demo (lost update and the fixes):

```sh
make lecture L=05-transactions-and-mvcc/05-03-row-locks-and-lost-updates
```

```
── Часть 1. Потерянное обновление: два воркера прочитали остаток ДО записи ──
Воркер 1 прочитал остаток = 10 ; воркер 2 прочитал остаток = 10
 остаток после двух продаж 
---------------------------
                         9
(1 row)

→ продали ДВА колд брю, а остаток упал лишь на единицу: один декремент потерян.

── Часть 2. Атомарный UPDATE: арифметику делает БД, потери нет ──
 остаток после двух продаж 
---------------------------
                         8
(1 row)

→ две продажи — остаток 8. Оба декремента на месте.

── Часть 3. FOR UPDATE: явная блокировка строки на время транзакции ──
 id |   name   | on_hand 
----+----------+---------
  1 | Колд брю |       8
(1 row)

 остаток после третьей продажи 
-------------------------------
                             7
(1 row)
```

Now the live queue. In the **first** terminal:

```sh
make lecture L=05-transactions-and-mvcc/05-03-row-locks-and-lost-updates T=session-a
```

A sets up the queue, grabs job `#1`, then waits at the prompt:

```
A1) Забираем задачу claim-запросом FOR UPDATE SKIP LOCKED — строка залочена до COMMIT:
 id |     payload      
----+------------------
  1 | сварить заказ #1
(1 row)

A держит задачу #1. Теперь в другом терминале запусти `make session-b`. ...
```

In the **second** terminal, while A holds `#1`:

```sh
make lecture L=05-transactions-and-mvcc/05-03-row-locks-and-lost-updates T=session-b
```

```
B1) Задача #1 залочена сессией A → SKIP LOCKED её пропускает, берём следующую (#2):
 id |     payload      
----+------------------
  2 | сварить заказ #2
(1 row)
```

B got `#2` **immediately**, without waiting for `#1`. Go back to terminal A and press Enter — A finishes `#1`, and the queue summary shows: jobs `#1` and `#2` done by different workers, `#3` still free.

```
A3) Итог очереди — B взял ДРУГУЮ задачу (#2), не дожидаясь #1. Двойной обработки нет:
 id |     payload      | status  
----+------------------+---------
  1 | сварить заказ #1 | done
  2 | сварить заказ #2 | done
  3 | сварить заказ #3 | pending
```

After the demo, restore the canon: `make ... T=db-reset` (the `job_queue` table can be dropped by hand).

## The fence

The step order in the two-session scenario is held by `\prompt` — without them it would be a race, and who "outran" whom would depend on the scheduler. In a real queue the workers genuinely compete, and `SKIP LOCKED` exists for exactly that. Here's what we simplified:

- **A row lock lives until the end of the transaction.** Holding a transaction open while the app calls an external service is dangerous: the hot row is locked, competitors pile up, and a long transaction also holds back the visibility horizon (bloat — see 05-02). Keep a critical section under `FOR UPDATE` as short as possible.
- **We didn't touch the deadlock here.** Two transactions locking rows in opposing order seize up — that's 05-06.
- **`FOR UPDATE` locks rows, it doesn't prevent every anomaly.** Write-skew (when transactions read and write *different* rows) isn't cured by it — for that you need `SERIALIZABLE` (05-04).

## Takeaways

- A **lost update** is two concurrent read-modify-writes that read the same value: the second write clobbers the first.
- The default fix is an **atomic `UPDATE`** (`SET x = x - 1`): read and write in one command under an implicit row lock.
- When read-modify-write is unavoidable — **`SELECT ... FOR UPDATE`**: explicitly locks the row at the read until `COMMIT`; a competitor waits.
- **`FOR UPDATE SKIP LOCKED`** does the opposite — it *skips* locked rows: the job-queue idiom, where N workers split the work with no waiting and no double-processing.
- A lock lives until the end of the transaction → keep the critical section short, don't call external services while holding it.

Next is **05-04 "isolation levels for developers"**: `FOR UPDATE` locks rows explicitly, but Postgres has a second lever against anomalies — the transaction's isolation level. We'll walk `READ COMMITTED` → `REPEATABLE READ` → `SERIALIZABLE` and the write-skew anomaly that only the last one catches.
