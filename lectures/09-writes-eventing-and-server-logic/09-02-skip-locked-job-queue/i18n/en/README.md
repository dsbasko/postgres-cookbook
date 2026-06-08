# 09-02 — A job queue on FOR UPDATE SKIP LOCKED

Brew has background work: send receipts, recompute stock, push "your order is
ready". All of it lands in a queue table, and several workers drain it in
parallel — the heavier the load, the more workers we spin up. And here the
classic concurrent-queue bug surfaces. A naive worker does "`SELECT` the oldest
job in status `queued`, then `UPDATE` it to `processing`". Two workers manage to
read the *same* row before either claims it — and both take the job. The customer
gets two identical pushes, the receipt goes out twice.

The obvious fix — lock the row on read (`FOR UPDATE`, see 05-03) — breaks
something else: the workers line up *behind each other*. The first takes the row
under a lock; the second, on the same `SELECT ... FOR UPDATE`, stalls and waits
for the first to commit. No parallelism — N workers act as one. We need a way to
say "take the first FREE row, and don't touch the busy ones".

## SKIP LOCKED: "skip the locked ones, don't wait"

That phrase is exactly what `SKIP LOCKED` utters. The worker's query looks like
this:

```sql
SELECT id FROM jobs_lab
WHERE status = 'queued'
ORDER BY id
FOR UPDATE SKIP LOCKED
LIMIT 1;
```

`FOR UPDATE` locks the selected row until the end of the transaction — while the
worker holds it, no one else will change it. `SKIP LOCKED` changes the behavior
on hitting an **already locked** row: instead of waiting for it to free up, the
planner simply **skips** it and takes the next free one. `LIMIT 1` hands out one
job at a time.

Together this gives a queue exactly what it needs: two workers will never take
the same job (the first locked it — the second skipped), and yet they don't block
each other (no one waits for anyone — each instantly gets the next free row). The
worker's transaction is short: claim → "process" (mark `done`) → commit, lock
released.

## The distribution is non-deterministic, and that's correct

Who gets which job depends on which worker fired the `SELECT` at which moment. Run
the demo twice and the "worker → jobs" split will differ. This is not a bug, it
is the whole point of `SKIP LOCKED`: the workers **self-balance** the load.
Whoever frees up takes the next one; a slow worker takes fewer, a fast one more,
nobody idles waiting on a neighbor. So in the output we print not the split (it
"drifts") but the **invariants** that don't depend on the scheduler: how many
jobs were claimed in total, how many were unique, and how many were duplicates.
Those are what we check.

## The race versus SKIP LOCKED, drawn

The whole difference fits in one picture. A naive worker reads and writes in two
steps, and in the gap between them another worker reads the same row:

```
Naive (SELECT, then UPDATE) — race window:
  worker-1  ──SELECT job#1──┐
  worker-2  ──SELECT job#1──┤  both read #1 BEFORE
            UPDATE #1 ◄──────┘  anyone claimed it
            UPDATE #1 ◄───────  → job#1 processed TWICE (two pushes to the client)

FOR UPDATE SKIP LOCKED — "skip what's busy, don't wait":
  worker-1  ─SELECT … SKIP LOCKED─►  #1 (locked) ─processed─ COMMIT
  worker-2  ─SELECT … SKIP LOCKED─►  #1 busy → skipped → takes #2
            nobody waits on a neighbor · nobody takes another's row
```

`FOR UPDATE` claims the row for a worker, `SKIP LOCKED` tells it to steer around
others' locked rows — and both the race and the mutual blocking vanish at once.

## What our code shows

This is a raw `pgx` unit: the lesson is about concurrency (several worker
goroutines, each with its own transaction), not the shape of a query. Each worker
spins a loop until the queue is empty, and on each iteration claims exactly one
job under `SKIP LOCKED`:

```go
err = tx.QueryRow(ctx, `
    SELECT id FROM jobs_lab
    WHERE status = 'queued'
    ORDER BY id
    FOR UPDATE SKIP LOCKED
    LIMIT 1`).Scan(&id)
if errors.Is(err, pgx.ErrNoRows) {
    return processed, nil // queue is empty — the worker finishes
}
```

On `pgx.ErrNoRows` there are no free jobs left and the worker exits. Otherwise it
marks the job `done` (with the worker's name) and commits — the lock is released,
and the row will never show up again (`WHERE status = 'queued'` no longer catches
it). The main function spins up four such workers as goroutines on a shared pool,
waits for them all, and checks the invariants: the claimed ids must form exactly
`{1..12}` with no repeats, and in the database all are `done` and none `queued`.

The pool is opened with `pg.WithMaxConns(numWorkers)`: the workers need
connections simultaneously, otherwise they would queue up for a connection rather
than for a job.

## Running it

```sh
docker compose up -d
make lecture L=09-writes-eventing-and-server-logic/09-02-skip-locked-job-queue T=db-reset
make lecture L=09-writes-eventing-and-server-logic/09-02-skip-locked-job-queue
```

`T=run` is the default and can be omitted. From inside the unit directory it is
shorter: `make db-reset`, then `make run`.

```
1) В очередь jobs_lab поставлено задач: 12. Воркеров: 4.
   Каждый воркер в цикле: BEGIN → SELECT ... FOR UPDATE SKIP LOCKED LIMIT 1 → обработать → COMMIT.

2) Свод по забранным задачам (инварианты, не зависят от планировщика):
   забрано всего      : 12
   уникальных задач   : 12
   дублей (один job двум воркерам): 0

3) Состояние очереди в базе после прогона:
   status='done'   : 12
   status='queued' : 0
```

12 jobs, 4 workers — claimed exactly 12, unique 12, duplicates `0`. No job lost
and none processed twice, even though the workers ran in parallel. The "who took
how many" split is deliberately not shown: it changes from run to run.

## The fence

- **Keep the worker's transaction short.** While it is open the row is locked, and
  a long transaction also holds the visibility horizon (see 05-02) and accumulates
  bloat. Do not do heavy work (a call to an external API, sending an email)
  *inside* the transaction — claim the job, quickly commit the status change, and
  do the work outside it; otherwise one stuck worker stalls version cleanup across
  the whole database.
- **`SKIP LOCKED` sacrifices strict ordering.** By skipping busy rows, the workers
  drain jobs not strictly by `id` but "whoever got there first". If order is
  mandatory (strict FIFO per key) that is no longer a `SKIP LOCKED` job but a
  matter of partitioning the queue by key or one worker per partition.
- **A queue table is a "before a broker" solution.** In Postgres it lives happily
  up to a point (tens to hundreds of thousands of jobs a day — no problem), but it
  is not Kafka or RabbitMQ. When the load outgrows what a single table under
  constant `UPDATE`/`DELETE` can take (already a question of autovacuum and bloat —
  your DBA's territory), it is time to look at a dedicated broker. Where exactly
  that line runs and how a DB queue hands off to a broker — in our universe the
  sibling `kafka-cookbook` course handles it.

## Takeaways

`FOR UPDATE SKIP LOCKED` turns an ordinary table into a concurrent queue:
`FOR UPDATE` claims a row for a worker, `SKIP LOCKED` says to skip others' locked
rows rather than wait on them. Two workers won't take the same job and won't block
each other — each immediately gets the next free one. The per-worker distribution
is non-deterministic by design (they self-balance the load), so what you check are
the invariants: zero duplicates, zero losses. Keep the worker's transaction short,
and remember that a queue table is a great "before a broker" solution, not a
replacement for one.

We can hand out ready work. Next — how to put that work there reliably in the
first place: record a business fact (an order) and an event about it (for
delivery/CDC) so that they either appear together or not at all. In 09-03 the
transactional outbox does this — the order and the event in one transaction, and a
relay drains the events with that same `FOR UPDATE SKIP LOCKED`.
