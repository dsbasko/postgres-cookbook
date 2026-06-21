# 05-05 — Retrying on 40001

> **Botyr:** I filed a ticket: "the database is unstable." Overnight transactions started failing on their own — out of nowhere, not our bug. That `SERIALIZABLE` we turned on yesterday.

> **Dmitry:** Show me the query.

Botyr turns his laptop around: the same shift calculation from 05-04, and next to it a serialization error.

> **Dmitry:** The database didn't break. It answered you. It even told you what to do next: retry.

> **You:** Retry it?

> **Dmitry:** Retry it. It's a contract. Not a failure.

Dmitry closes the ticket.

> **Botyr:** So catch this and rerun the transaction — by hand, in every handler?

> **You:** And if there are forty handlers?

> **Dmitry:** Then one loop for all of them. `withRetry` — wrap any transaction in it instead of copy-pasting a `try` all over the code.

Yesterday's incident from 05-04: Alice's transaction (Alice and Boris are the shift baristas, namesakes of the customers Alice Ivanova and Boris Petrov — different people) failed with `40001` (`serialization_failure`) because it committed second and closed a "dangerous pair" of dependencies. The hint in the error read `The transaction might succeed if retried`. The ticket "the database is unstable" is an understandable first reaction; a wrong one.

`40001` is not a glitch. It's the **expected** answer from `SERIALIZABLE`: "I couldn't line your transaction up into a consistent order with the others — start over." The level's contract is two-sided — the database gives you serializability, and you must **retry** transactions on `40001`. You can't use `SERIALIZABLE` without a retry loop. This unit is about that loop: how to write it in Go and why a retry almost always succeeds.

This is a Go-centric escape-hatch (raw-pgx, no sqlc): the lesson is about the application's control logic — the retry loop and parsing the server error code — not about SQL.

## Why the retry works

The key is that a retry runs in a **new** transaction, and therefore with a **new snapshot** (see 05-02). Alice's first attempt read "two on the floor" from a snapshot taken before Boris left. By the time of the retry, Boris has already committed — and the fresh snapshot shows "one on the floor." Alice makes a **different**, correct decision: stay. There's no conflict anymore, and `COMMIT` goes through.

This is the general principle: `40001` means "your decision is based on stale data." The retry re-reads fresh data and either reaches a different outcome or, if the conflict was a coincidence in timing, simply passes. That's why almost all retries finish in 1–2 attempts.

## How to catch exactly 40001

Not every error is worth retrying — retrying a syntax error or a `CHECK` violation is pointless (the result won't change) and dangerous. You need exactly code `40001`. In pgx the server error is extracted via `errors.As` to `*pgconn.PgError`, whose `Code` field is the SQLSTATE:

```go
func isSerializationFailure(err error) bool {
    var pgErr *pgconn.PgError
    return errors.As(err, &pgErr) && pgErr.Code == "40001"
}
```

Not every error is worth retrying — a short decision table:

| SQLSTATE | What it means | Retry? |
|---|---|---|
| `40001` (`serialization_failure`) | serialization conflict under `SERIALIZABLE`/`REPEATABLE READ` | **yes** — new transaction, fresh snapshot |
| `40P01` (`deadlock_detected`) | mutual lock, the victim was rolled back (05-06) | **yes** — the same loop |
| `23505` (`unique_violation`) | duplicate key | no — the result won't change on retry |
| `23514` (`check_violation`) | a `CHECK` was violated | no — the data won't pass on retry either |
| `42601` / `42P01` … | syntax or missing object — a query bug | no — retrying won't help |

## What our code shows

`cmd/demo/main.go` is the retry loop `withRetry` plus Alice's transaction as a closure. The loop is simple:

```go
for attempt := 1; attempt <= maxAttempts; attempt++ {
    tx, _ := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
    err := txFn(ctx, tx, attempt)          // the transaction's work
    if err == nil { err = tx.Commit(ctx) } // 40001 often surfaces at COMMIT
    if err == nil { return attempt, nil }  // success
    tx.Rollback(ctx)
    if isSerializationFailure(err) { continue }  // ↻ retry: new tx, new snapshot
    return attempt, err                          // another error — propagate
}
```

To make the conflict **deterministic** (rather than dependent on a race), the demo on the first attempt synchronously injects Boris's departure via a separate transaction — in production another app instance would do this at the same instant. After that everything is honest: Alice's first attempt catches `40001`, the loop retries, and the second attempt on a fresh snapshot decides to stay and commits.

## Running it

Bring up the sandbox (from the repo root) and apply the Brew base schema:

```sh
docker compose up -d
make lecture L=05-transactions-and-mvcc/05-05-retry-on-40001 T=db-reset
make lecture L=05-transactions-and-mvcc/05-05-retry-on-40001
```

(`T=run` is the default. From inside the unit's directory it's `make db-reset`, `make run`.)

Output:

```
1) shift_lab: на полу 2 бариста (Алиса #1, Борис #2). Правило: на полу всегда ≥1.

2) Алиса решает, может ли уйти — транзакция SERIALIZABLE с ретраями:
   (параллельно: Борис ушёл с пола и закоммитил — конфликт назревает)
   попытка 1: на полу 2 (на момент чтения) → можно уйти, снимаю свой флаг
   ↻ транзакция упала: 40001 (serialization_failure) — повторяю на свежем снимке
   попытка 2: на полу 1 → уходить нельзя (на полу ≤1), остаюсь
   ✓ COMMIT успешен (заняло попыток: 2)

3) Итог: на полу 1 бариста — инвариант сохранён.
   Ретрай прочитал свежий снимок и принял верное решение (Алиса осталась):
   #1 Алиса  на полу: да
   #2 Борис  на полу: нет
```

Attempt 1: Alice sees "2 on the floor," decides to leave, and clears her flag — but Boris has already committed his departure, and Postgres fails the transaction with `40001` (here, right at the `UPDATE`; it could also be at `COMMIT`, as in 05-04). The loop retries. Attempt 2 takes a fresh snapshot: "1 on the floor" — leaving isn't allowed, Alice stays, `COMMIT` passes. The invariant is preserved, and note — the retry made a **substantively different** decision, because it saw fresh data.

## The fence

Our loop is simplified to its essence. In production you add:

- **A retry cap.** We have one (`maxAttempts`) — otherwise an endless loop on a persistent conflict.
- **Backoff with jitter between attempts.** Without a pause, N retrying transactions will hammer each other in lockstep — a "thundering herd."
- **A replayable transaction.** `txFn` must run from scratch: no side effects outside the DB (a sent email, a charge in a payment gateway) inside a retrying transaction, or the second attempt does them twice. Network calls go outside the loop.
- **The same trick for `40P01`.** A deadlock (see 05-06) is transient too and is cured by the same loop.
- **A retry is no substitute for a sound schema.** If a transaction fails with `40001` constantly, the problem isn't the loop — it's that too many transactions fight over the same data; the fix is the schema/logic, not a bigger attempt count.

## Takeaways

- `40001` (`serialization_failure`) is the **expected** answer from `SERIALIZABLE`, not a glitch; the level's contract requires you to **retry** the transaction.
- The retry works because it runs in a new transaction with a **new snapshot**: fresh data → a different (correct) decision. Usually 1–2 attempts are enough.
- Catch exactly code `40001`: in pgx, `errors.As` to `*pgconn.PgError`, the `Code` field. Non-transient errors must not be retried.
- A retried transaction must be **idempotent/replayable**: no external side effects inside it.
- Put a retry cap and backoff in the loop; the same loop usually handles `40P01` (deadlock) too.

Next is **05-06 "deadlocks and advisory locks"**: we'll look at `40P01` — the mutual lock that Postgres detects and breaks on its own — and `pg_advisory_lock`, the application-level lock used to prevent deadlocks.
