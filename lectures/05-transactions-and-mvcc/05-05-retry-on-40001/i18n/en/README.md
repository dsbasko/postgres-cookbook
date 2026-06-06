# 05-05 — Retrying on 40001

In the previous unit `SERIALIZABLE` saved Brew's invariant at the cost of an error: Alice's transaction failed with `40001` (`serialization_failure`) because it committed second and closed a "dangerous pair" of dependencies. The hint in the error read `The transaction might succeed if retried`. This is where many make the fatal mistake: they show the user a "500 Internal Error" and go off to debug a "random database glitch."

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

Bring up the sandbox (from the repo root) and apply the canon:

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

Our loop is simplified to its essence. In production you add: a **retry cap** (we have one — `maxAttempts`, otherwise an endless loop on a persistent conflict) and **backoff with jitter** between attempts (without a pause, N retrying transactions will hammer each other in lockstep — a "thundering herd"). What you retry matters too: `txFn` must be replayable from scratch — no side effects outside the DB (a sent email, a charge in a payment gateway) inside a retrying transaction, or the second attempt does them twice. Network calls and external services go outside the loop. Besides `40001`, the same scheme is often used to retry `40P01` (deadlock — see 05-06): also a transient error with the same cure. And remember: retrying is a "cheap" path to correctness only while conflicts are rare; if a transaction fails with `40001` constantly, the problem isn't the loop — it's that too many transactions fight over the same data, and the fix is the schema/logic, not a bigger attempt count.

## Takeaways

- `40001` (`serialization_failure`) is the **expected** answer from `SERIALIZABLE`, not a glitch; the level's contract requires you to **retry** the transaction.
- The retry works because it runs in a new transaction with a **new snapshot**: fresh data → a different (correct) decision. Usually 1–2 attempts are enough.
- Catch exactly code `40001`: in pgx, `errors.As` to `*pgconn.PgError`, the `Code` field. Non-transient errors must not be retried.
- A retried transaction must be **idempotent/replayable**: no external side effects inside it.
- Put a retry cap and backoff in the loop; the same loop usually handles `40P01` (deadlock) too.

Next is **05-06 "deadlocks and advisory locks"**: we'll look at `40P01` — the mutual lock that Postgres detects and breaks on its own — and `pg_advisory_lock`, the application-level lock used to prevent deadlocks.
