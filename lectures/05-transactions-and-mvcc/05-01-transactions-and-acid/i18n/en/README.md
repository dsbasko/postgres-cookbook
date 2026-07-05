# 05-01 — Transactions and ACID

The previous module ended on Botyr's question: what happens when two registers reach for the same rows at once? The answer starts with money — with a message Ruslan sent to the chat late in the evening.

> **Ruslan (in chat, 22:47):** Evening reconciliation doesn't add up. Register-1: the debit is there. Register-2: no credit. The money left — and never arrived.

The next morning Dmitry nods at that message over your shoulder.

> **Dmitry:** Money doesn't vanish. "Debit" and "credit" are two separate commands, and something got stuck between them. The fix isn't more care at the register, it's making the database hold both commands as one job. That's exactly what today is about.

Brew is moving the day's takings from one shop's register to another's: debit the first account, credit the second. Two commands. Between them lies a sliver of milliseconds, and that sliver hides the worst-case scenario: the debit goes through, the credit fails (the network blinked, the process died, the disk filled up). Money debited and never credited — it vanished. Or the reverse: the credit lands, the debit doesn't, and money appears out of thin air. In a system that counts other people's money, that isn't a bug — it's a disaster.

The fix isn't "write more carefully," it's a tool the database already gives you: the **transaction**. `BEGIN`, both commands, `COMMIT` — and Postgres guarantees they apply **together or not at all**. This unit is about the four letters behind that guarantee: **ACID**.

## The transaction: BEGIN, COMMIT, ROLLBACK

A transaction is a group of commands the database runs as a single unit. `BEGIN` opens it; one of two endings closes it:

- `COMMIT` — "make it all permanent": the changes become durable and visible to others.
- `ROLLBACK` — "forget all of it": the database returns to exactly the state it was in before `BEGIN`, as if no command had run.

While a transaction is open, its changes aren't visible to other sessions and can be undone at any moment. If an error happens mid-transaction (a `CHECK` is violated, the connection drops), the uncommitted work is rolled back as a whole. That's what turns "debited but not credited" from a possible outcome into an impossible one: either `COMMIT` after both commands, or `ROLLBACK` — there's no third option.

## ACID: what a transaction actually guarantees

- **A — Atomicity.** All or nothing. A two-command transfer either applies in full or not at all. There are no halves.
- **C — Consistency.** A transaction moves the database from one valid state to another without violating invariants (`CHECK`, `FOREIGN KEY`, `UNIQUE`). The total money across accounts doesn't change on a transfer — and the database won't let it.
- **I — Isolation.** Concurrent transactions don't see each other's intermediate, uncommitted states. Almost all of the rest of module 05 is about this (snapshots in 05-02, locks in 05-03, isolation levels in 05-04).
- **D — Durability.** After `COMMIT`, the data survives a server crash — it's already on disk (in the WAL).

In this unit we observe **A** and **C** directly; **I** and **D** come later in the module and at the server level.

The four letters as a map — what each guarantees and where you see it:

| Letter | Guarantees | In our demo | Covered |
|---|---|---|---|
| **A** — atomicity | all or nothing, no halves | step 3: the debit on `#1` rolled back in full | here |
| **C** — consistency | invariants hold (`CHECK`/`FK`/`UNIQUE`) | step 4: account total `150.00` unchanged | here |
| **I** — isolation | concurrent transactions don't see each other's in-between | — | 05-02 → 05-04 |
| **D** — durability | after `COMMIT`, data survives a crash (WAL) | — | server level |

## What our code shows

The queries in `query.sql` are the building blocks of a transfer: debit, credit, and a sum that's the system's invariant.

```sql
-- name: Debit :execrows
UPDATE ledger_accounts SET balance = balance - sqlc.arg(amount) WHERE id = sqlc.arg(id);
-- name: Credit :execrows
UPDATE ledger_accounts SET balance = balance + sqlc.arg(amount) WHERE id = sqlc.arg(id);
```

The debit is guarded by `CHECK (balance >= 0)` from `schema.sql`: you can't go negative — an attempt fails the command (SQLSTATE `23514`) and the whole transaction with it. `main.go` assembles the blocks into a transaction — the `transfer` function:

```go
tx, _ := pool.Begin(ctx)
defer tx.Rollback(ctx)            // safety net: an early return → rollback
qtx := queries.WithTx(tx)         // queries within this transaction
qtx.Debit(ctx, ...)               // debit the sender
n, _ := qtx.Credit(ctx, ...)      // credit the recipient
if n == 0 { return errNoPayee }   // no recipient → return, defer rolls back
tx.Commit(ctx)                    // both commands landed — make it permanent
```

The demo runs two transfers. The first is ordinary, to an existing account: `COMMIT`, the money moved. The second targets a non-existent account `#999`: the debit on `#1` **succeeds** (real work inside the transaction), but `Credit` affects 0 rows — `main.go` notices via `RowsAffected` and returns, `defer tx.Rollback` undoes the whole transfer. The debit on `#1` disappears with it.

## Running it

Bring up the sandbox (from the repo root) and apply the Brew base schema plus this unit's table:

```sh
docker compose up -d
make lecture L=05-transactions-and-mvcc/05-01-transactions-and-acid T=db-reset
make lecture L=05-transactions-and-mvcc/05-01-transactions-and-acid
```

(`T=run` is the default. From inside the unit's directory it's `make db-reset`, `make run`.)

Output:

```
1) Два кассовых счёта засеяны:
   #1 Касса Brew Central   100.00
   #2 Касса Brew North     50.00

2) Перевод 30.00 со счёта #1 на #2 (BEGIN → списать → зачислить → COMMIT):
   COMMIT. Состояние:
   #1 Касса Brew Central   70.00
   #2 Касса Brew North     80.00

3) Перевод 20.00 со счёта #1 на НЕсуществующий #999 — должен откатиться целиком:
   перевод отклонён: счёта-получателя #999 не существует
   ROLLBACK. Состояние (как в шаге 2 — списание #1 откатилось вместе с переводом):
   #1 Касса Brew Central   70.00
   #2 Касса Brew North     80.00

4) Сумма по всем счетам: 150.00 — неизменна с самого начала (ничего не потеряно, ничего не создано).
```

Step 2 is the successful transfer: 30.00 moved from `#1` to `#2`. Step 3 is the failed one: the debit on `#1` inside the transaction went through, but there was no one to credit, and `ROLLBACK` undid **both** commands at once — in step 3 the balance of `#1` is exactly what it was in step 2 (70.00), not 50.00. Step 4 is the point: the total across accounts is **150.00** from start to finish. That's atomicity (the debit didn't survive the transaction) and consistency (the sum invariant held).

## The fence

The demo turns "no such recipient" into an error and rolls back by hand. In production a failure is rarely that polite — and each of the demo's conveniences becomes a production concern:

- **`defer tx.Rollback(ctx)` — right after `Begin`.** A transaction dies on a driver error, a timeout, a dropped connection, and the rollback must happen in every one of those cases. Placed right after `Begin`, the `defer` fires even on a panic or an early `return` — the transaction won't be left hanging open (and an open transaction holds locks and the visibility horizon — see 05-02).
- **A real money transfer isn't two `UPDATE`s.** It's double-entry ledger postings, idempotency keyed on the operation id (so a retried request doesn't debit twice), and an audit trail. Here we only care about the "together or not at all" mechanics.
- **Atomicity holds inside the database only.** If after `COMMIT` you need to publish an event to Kafka or call a payment gateway, that's outside the DB transaction, and consistency there is achieved by other means — the transactional outbox (module 09).

## Takeaways

- A transaction (`BEGIN` … `COMMIT`/`ROLLBACK`) is a group of commands applied **together or not at all**.
- **A**tomicity: partial work (the debit that succeeded) is undone in full on `ROLLBACK` — observable in step 3.
- **C**onsistency: the invariant (account sum, `CHECK`s, `FK`s) is preserved — the total stays 150.00.
- **I**solation and **D**urability — later in module 05 and at the server level (WAL).
- In Go: `pool.Begin` → `queries.WithTx(tx)` → `Commit`/`Rollback`; `defer tx.Rollback(ctx)` right after `Begin` is the safety net against any early exit.
- A `CHECK` invariant in the schema (`balance >= 0`) isn't a nicety — it's the mechanism that fails the transaction and so keeps the data correct.

Next is **05-02 "the MVCC mental model"**: how Postgres lets several transactions read and write at once without blocking each other — through row versions and snapshots. It's the foundation under the **I** in ACID.
