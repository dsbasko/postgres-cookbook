# 10-04 — Pooling from the app

Brew has grown. There used to be one backend instance holding its own connection
pool straight to Postgres, and everything worked. Now there are dozens of
instances, each with its own pool, and the sum of their connections has hit
`max_connections`. The classic answer is to put **pgbouncer in transaction mode**
in front of the database: it keeps a small set of real backends and hands them
out to applications on demand. Connections to the database drop, everyone is
happy — until the first night under load.

pgbouncer went out at 03:11. A minute later the alerts started: the order screen
is silent, an advisory lock is stuck, a parameterized query fails with an error
about a missing prepared statement. It ran flawlessly for years — and went down
all at once. You're on call, for the first time at night. At 03:14 you message
Pavel.

> **You (in chat, 03:14):** Lock stuck. LISTEN silent. prepared failing. What's the common thread?

The reply is instant, in her signature lowercase.

> **Pavel (in chat):** the pool. transaction mode. there's no session anymore.

There is one cause, and it has to be understood literally. A transaction pool
holds a real backend for you for exactly **one transaction**, not for a "session".
Close the transaction and the backend goes back into the shared pot, and the pool
may hand your next transaction to you on a **different** backend. Anything that
lives at the session level rather than the transaction level silently breaks
across that move.

## What "session" means and why it disappears

When you connect to Postgres directly, you have a **session** — it lives for the
whole connection and holds state: session-level advisory locks you've taken,
`LISTEN` subscriptions, parameters set via `SET` (GUCs), the prepared-statement
cache. All of it is bound to a **specific backend** (the `postgres` process
serving your connection).

A transaction pool destroys that illusion of a session. Between transactions you
may land on a different backend — and it has its own state, not yours. The rule
worth writing on the wall: **a transaction pool guarantees one backend per
transaction, not per session.** What follows are three concrete breakages and
their fixes.

## Breakage 1: the session-level advisory lock

A session-level advisory lock (`pg_advisory_lock`, see 05-06) belongs to the
backend that took it. The lock itself is **global and visible to everyone**:
another backend sees the key is taken. But only the owner can release it. If the
pool took the lock on one backend and you come to release it from another,
`pg_advisory_unlock` returns `false`, the lock stays hanging, and that is a
**leak**.

The fix is a **transaction-scoped lock**, `pg_advisory_xact_lock`. It is held for
exactly one transaction (which the pool honestly keeps on one backend) and
**releases itself at `COMMIT`**. There is nothing to release by hand, so moving
between backends breaks nothing.

## Breakage 2: LISTEN/NOTIFY

`LISTEN` registers a subscription on a **specific backend**. `NOTIFY` only reaches
the backends that have run `LISTEN` on that channel. Under a transaction pool you
ran `LISTEN` in one transaction, the pool returned the backend to the pot, you
came to listen — and you were reseated onto a backend that knows nothing about
your `LISTEN`. The notification never reaches you.

The fix is the same as in 09-04: a **dedicated connection**. You hold one
connection for yourself, run `LISTEN` on it, and wait for notifications on that
same connection — the pool doesn't reseat it because you never release it.
`pg_notify` arrives exactly there.

## Breakage 3: prepared statements

By default pgx uses the extended protocol and **caches prepared statements
per-backend**: the first time it prepares a statement on a backend, then reuses it
by name. Under a transaction pool, a statement prepared on one backend is simply
absent on the next — and the query fails.

The fix is to put the pool into **simple-protocol mode**
(`pgx.QueryExecModeSimpleProtocol`, set via a `pg.Option`): pgx stops caching
prepared statements on the backend, and a reseat breaks nothing. There's a nuance
to the price — the simple protocol is a touch less efficient — but under a
transaction pool it is the only reliable path. A real pgbouncer, by the way, can
do the opposite too: `max_prepared_statements` lets it track prepared statements
itself on top of the pool.

The next morning — an incident retro at the whiteboard.

> **Dmitry:** Three things went down at once. Name them — and why each one.
>
> **You:** Advisory lock, LISTEN, prepared statement. All three — on the session.
>
> **Pavel:** A pool gives you a backend per transaction, not per session. What's
> on the session is no longer yours.
>
> **Dmitry:** So we don't fix the pool — we fix three places in the code. Each
> session-scoped thing to its transaction-scoped equivalent. Below — the reseat
> picture and the replacement table.

## The reseat between backends — and what survives it

All three breakages are the same picture: the pool pins a backend to you for one
transaction, reseats you between transactions, and anything session-scoped is left
behind on the old backend:

```
Transaction pool: the backend is pinned to you per TRANSACTION, not per session

  TX1 ──► pool handed out backend A   LISTEN orders · pg_advisory_lock(42)
  COMMIT ──► backend A went back into the shared pot
  TX2 ──► pool handed out backend B   ← a different backend!
                                        B has none of your LISTEN, none of your lock:
                                        NOTIFY won't arrive, unlock returns false (leak)

  fix — don't release the backend between operations:
    pg_advisory_xact_lock   releases itself at COMMIT (lives inside the TX)
    dedicated connection    you hold backend A yourself, LISTEN on it too
    simple protocol         no prepared statements pinned to a backend
```

The same rule folds into a table: what's risky to keep on the session, and what to
replace it with.

| What lives on the session | Session-scoped (breaks under the pool) | Transaction-safe (survives) |
|---|---|---|
| Advisory lock | `pg_advisory_lock` (leaks on another backend) | `pg_advisory_xact_lock` (releases at COMMIT) |
| LISTEN/NOTIFY | `LISTEN` through the pool (silent) | dedicated connection |
| Prepared statements | per-backend cache (fails) | simple protocol / `max_prepared_statements` |
| GUC parameters | `SET` for the session | `SET LOCAL` inside the transaction |

## What our code shows

There is no real pgbouncer here. We reproduce a transaction pool on **plain
Postgres**, deliberately spreading operations across several pool backends
(`pool.Acquire` hands out distinct connections = distinct backends) — exactly what
a transaction pool does between transactions. It is an honest simulation: the
behaviour of session state doesn't change because of it.

The unit is written in **raw pgx** (escape-hatch, a `go.mod` without sqlc): the
lesson is about connection management — `Acquire`/`Release`, a dedicated
connection, the protocol mode. That is the pool API, not SQL, so sqlc would be out
of place here.

`cmd/demo/main.go` shows three breakages and three fixes. In the first part
`connA` and `connB` are different backends (their `pg_backend_pid` is printed): A
takes `pg_advisory_lock(42)`, B sees it (`pg_try_advisory_lock` → `false`) but
can't release it (`pg_advisory_unlock` → `false`, the lock leaks), while
`pg_advisory_xact_lock(99)` is held inside a transaction and releases at `COMMIT`.
In the second, a dedicated connection catches the `NOTIFY` while a connection
without `LISTEN` stays silent until timeout. In the third, a separate pool in
simple-protocol mode runs a parameterized `SELECT name FROM drinks WHERE id = $1`
and calmly returns the result.

One subtlety about the output: the failed `pg_advisory_unlock` on someone else's
lock prints a `WARNING` to the log. That is expected, and it goes to **stderr** —
stdout stays clean, exactly the text pasted below.

## Running it

```sh
docker compose up -d
make lecture L=10-use-cases/10-04-pooling-from-the-app T=db-reset
make lecture L=10-use-cases/10-04-pooling-from-the-app
```

`T=run` is the default and can be omitted. From inside the unit directory it is
shorter: `make db-reset`, then `make run`. Since this is a capstone, the unit also
has `make test` — it runs an integration test with assertions against a live
database.

```
1) Session advisory-лок привязан к бэкенду (транзакционный пул его ломает)
   A и B — разные бэкенды: true
   A: pg_advisory_lock(42) — взял
   B: pg_try_advisory_lock(42) → false (лок виден всем, держит A)
   B: pg_advisory_unlock(42) → false (не его лок — снять нельзя, лок течёт)
   фикс — pg_advisory_xact_lock: держится в транзакции true, после COMMIT false (снялся сам)

2) LISTEN/NOTIFY живёт на бэкенде — нужен выделенный коннект
   выделенный коннект (сам делал LISTEN): получил "order #1"
   коннект без LISTEN (как при пересадке пулом): услышал что-то false (таймаут — ничего)

3) Prepared statements под пулингом → режим простого протокола
   simple protocol: SELECT с параметром вернул "Эспрессо" — без кэша prepared-запросов на бэкенде
   (по умолчанию pgx кэширует prepared statements per-backend — под транзакционным пулом это ломается)
```

Block 1: A and B really do sit on different backends. A took the lock, B sees it
but can't release a lock it doesn't own — in production that lock would hang
forever. The transaction-scoped lock is held inside the transaction (`true`) and
releases itself at `COMMIT` (`false`). Block 2: the notification reached only the
connection that ran `LISTEN` itself; the connection without a subscription
honestly sat out the timeout and heard nothing. Block 3: the parameterized query
in simple-protocol mode returned `Эспрессо` with no per-backend cache whatsoever.

## The fence

- **This is a simulation, not a real pgbouncer.** In production a real **pgbouncer in
  transaction mode** would sit in front of Postgres; we imitated its behaviour by
  spreading operations across several pool backends. The simulation is honest in
  exactly one way: it breaks the same session state a real pool would break. But it is
  not a substitute for pgbouncer, nor for its config.
- **A transaction pool guarantees one backend per transaction, not per session.** Keep
  it in mind at all times. Anything session-scoped is the hazard — session-level
  advisory locks, `LISTEN`, session GUCs via `SET`, plain prepared statements; their
  transaction-scoped equivalents are safe (see the table above). If you need a
  different pool mode — session pooling gives you the whole session back, but then the
  very connection savings you put the pool in for disappear.
- **Sizing the pool against `max_connections` is operational work, not ours.** It's
  real and important, but it's your DBA's territory. This unit is about writing code
  that survives a transaction pool, not about configuring one.

## Takeaways

A transaction pool (pgbouncer transaction mode) holds a real backend for you for
exactly **one transaction**, not for a session — between transactions it can
reseat you onto another backend. So anything session-scoped silently breaks, and
the cure is moving to transaction-scoped equivalents: `pg_advisory_xact_lock`
instead of session locks, a dedicated connection for `LISTEN`/`NOTIFY`,
simple-protocol mode instead of a per-backend prepared-statement cache. We
introduced the connection pool back in 00-06, covered advisory locks in 05-06,
and the dedicated connection for `LISTEN`/`NOTIFY` in 09-04; here all of it met
the reality of pooling.

Next — the finale. Capstone 10-05 closes the course: the Brew base tables (`orders`,
`outbox`, the CDC sources with `REPLICA IDENTITY FULL`) goes into logical
replication, and the CDC seam hands the baton to `kafka-cookbook` — Debezium reads
our tables without rewriting the schema. The two books meet on a single data
model.
