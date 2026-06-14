# 00-06 — Connection lifecycle and pooling

Launch night for Brew's site. Traffic is coming in — and the application logs turn red with a single line: `FATAL: sorry, too many clients already`. The team gathers around one monitor.

> **Danya:** The site won't take orders. Now the till catches the same FATAL — and it did nothing wrong.
>
> **Zoya:** I see it. Backends hit the limit. All of them — yours.
>
> **Marat:** We look from both sides. Zoya — the server. You — the pool.
>
> **You:** Our pool is empty. The site goes around it: the launch code calls pgx.Connect on every HTTP request.
>
> **Zoya:** So every guest's click is a new process on my server. Ceiling. Refusal — for everyone.
>
> **Marat:** That code is older than our pool. First understand, then fix.

Understanding it is this unit's job. It opens the black box of `pg.NewPool`, which we've called in every previous unit: what a connection is to the server, why a pool is needed, how many connections it holds, and how to see your own backends through Postgres's own eyes via `pg_stat_activity`. This is a raw-pgx unit — the lesson is about the pool's API, not about queries, so sqlc has no role here.

## A connection is a server process, and it isn't free

When a client connects to Postgres, the server forks a separate process for it — a **backend**. That process lives for the whole connection and holds its own memory (work_mem, caches, session state). Opening it means a TCP handshake, authentication, session init: milliseconds that add up to noticeable latency under load. And keeping them open costs memory and OS scheduling per backend. So the server has a hard ceiling — `max_connections` (default ~100): not "as many as it can take", but how many backends it will allow at all.

> **You:** A hundred connections — that's just a hundred sockets, right?
>
> **Zoya:** A hundred processes. Memory. The scheduler.
>
> **Marat:** Hence the pool.

The takeaway: a connection is an expensive and limited resource. Opening one per request is an anti-pattern (Brew's very mistake). The right way is to open a pool once at application startup and reuse connections.

## The pool: open once, reuse many times

The pool (`pgxpool`) holds a set of already-open connections. The logic is simple:

- **Acquire** — take a connection from the pool. If a free one exists, it's handed over instantly. If none is free but the `MaxConns` limit isn't reached, the pool opens a new one. If the limit is reached, Acquire **waits** until someone returns theirs.
- **Release** — return a connection to the pool. Importantly, Release does **not** close the backend — it leaves it open and marks it free for the next Acquire. That's the whole point: the handshake is paid once, after which the connection lives on and gets reused.

The pool is **lazy**: after `NewPool` there are no connections yet (`MinConns=0` by default), the first one opens on the first Acquire. And the pool has built-in stats — `pool.Stat()`: how many connections are open now (`TotalConns`), how many are handed out (`AcquiredConns`), how many sit idle and ready (`IdleConns`).

The key correspondence: **one connection in the pool = one backend on the server**. The application's pool size is exactly the number of processes it occupies on Postgres. That's why the pool's `MaxConns` and the server's `max_connections` are two sides of the same arithmetic.

The same idea as a picture:

```
   app pool (MaxConns=4)            server (postgres)
   ┌────────────────────┐
   │ slot 1  ▣ acquired │──▶ backend 1  ─┐
   │ slot 2  ▣ acquired │──▶ backend 2   │  one pool slot =
   │ slot 3  ▢ idle     │──▶ backend 3   │  one backend on the server
   │ slot 4  ▢ idle     │──▶ backend 4  ─┘
   └────────────────────┘
     Acquire: ▢ idle → ▣ acquired   (none free, under limit → open; at limit → wait)
     Release: ▣ acquired → ▢ idle   (the backend is NOT closed — it sits idle, ready for Acquire)
```

## What our code shows

The demo creates a small pool (`MaxConns=4`) and traces the connection lifecycle, cross-checking against what the server itself sees. The connections are tagged with `application_name` so we can filter exactly our own backends in `pg_stat_activity`:

```go
pool, err := pg.NewPool(ctx,
	pg.WithMaxConns(4),
	func(c *pgxpool.Config) {                                   // a custom Option (escape hatch)
		c.ConnConfig.RuntimeParams["application_name"] = "brew-pool-demo"
	},
)
```

`pg.WithMaxConns` is the standard option from `internal/pg`. Next to it is a `func(*pgxpool.Config)` literal: that's the same `pg.Option` (an escape hatch for fine-tuning the pool for a lesson), and it sets `application_name` in each connection's startup packet. The demo then acquires all 4 connections without returning them, asks the server for the backend count, and returns everything:

```go
conns := make([]*pgxpool.Conn, 0, 4)
for i := 0; i < 4; i++ {
	c, _ := pool.Acquire(ctx)        // the pool must open a real backend
	conns = append(conns, c)
}
// the pool is exhausted — run count on an already-acquired conn, or pool.Query would block
conns[0].QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity WHERE application_name = $1", appName).Scan(&backends)
for _, c := range conns {
	c.Release()                      // doesn't close the backend — leaves it idle
}
```

`pool.Stat()` before the acquire, after the acquire, and after the return — three snapshots that show the whole cycle:

| Moment | `TotalConns` (total) | `AcquiredConns` (in use) | `IdleConns` (idle) |
|---|---|---|---|
| after `NewPool` — pool is lazy | 0 | 0 | 0 |
| 4×`Acquire` — pool opened backends | 4 | 4 | 0 |
| 4×`Release` — returned, not closed | 4 | 0 | 4 |

The last row is the whole point of the pool: in use is 0, yet total is 4. The connections didn't vanish — they sit idle, waiting for the next `Acquire`.

## Running it

Bring up the sandbox (from the repo root) and apply the base schema:

```sh
docker compose up -d
make lecture L=00-getting-connected/00-06-connection-lifecycle-and-pooling T=db-reset
```

Run the demo:

```sh
make lecture L=00-getting-connected/00-06-connection-lifecycle-and-pooling
```

(`T=run` is the default. From inside the unit directory it's simply `make db-reset` and `make run`.)

Output:

```
Пул создан: MaxConns=4, application_name="brew-pool-demo".

1) Сразу после NewPool пул ленив — соединений ещё нет:
   всего=0  занято=0  простаивают=0  (макс=4)

2) Захватили 4 соединения (pool.Acquire) — пул открыл столько реальных бэкендов:
   всего=4  занято=4  простаивают=0  (макс=4)

3) Сколько бэкендов с application_name="brew-pool-demo" видит Postgres (pg_stat_activity):  4

4) Вернули все 4 в пул (conn.Release) — соединения не закрылись, а простаивают:
   всего=4  занято=0  простаивают=4  (макс=4)
```

(The demo prints in Russian.) Step 1 — the pool is empty (lazy). Step 2 — the acquire opened exactly 4 backends, and the server confirms them at step 3: the `pg_stat_activity` count matched the number of acquired connections. Step 4 — `Release` returned them to the pool without closing: `acquired=0`, but `total=4` — four backends are still alive, ready for reuse.

> [!NOTE]
> **Check yourself.** The demo acquires 4 connections, then returns all 4. After
> the return, `pool.Stat()` shows `acquired` (`AcquiredConns`) and `total`
> (`TotalConns`) — what are those numbers? Which one is surprising, and why?

> [!TIP]
> **Answer.** `acquired=0` and `total=4` — as in step 4 of the output above. The
> surprising one is `total=4`: it may seem returning a connection means closing it,
> but `Release` only marks the backend free, without closing it. Four processes stay
> alive on the server, ready for the next `Acquire` — that's the whole point of the
> pool. They'd close only on `pool.Close()` when the application exits.

## The fence

- "More connections = faster" is a myth. Each backend costs the server memory and a slot in the OS scheduler, and beyond the number of cores extra connections only add contention, not throughput. Pool size is a trade-off, not "the more the better"; in production it's tuned to the load and to `max_connections`, not left at the default.
- When there are many application instances and their pools collectively hit the server limit, an external pooler (PgBouncer) goes between them and Postgres; it has its own pitfalls (transaction mode breaks session-level things like advisory locks and `LISTEN/NOTIFY`) — that's the subject of capstone 10-04.
- Every `Acquire` must have a matching `Release` (usually a `defer`), or the connection "leaks" from the pool forever — a slow-motion version of Brew's very mistake.
- Don't hold an acquired connection across long external I/O: you're blocking a scarce resource while waiting on someone else's service.

## Common mistakes in module 00

The morning after the war-room the team sits down for a short postmortem — and since this is the last unit of the module, the first-contact rakes get collected into one table. What they share: the demo doesn't show them — some silently return the wrong thing (injection, a swapped `Scan`), others pile up and take the app down only in production under load (a connection per request, a pool leak). Pin this table above your code review.

| trap | unit | the right way |
|---|---|---|
| gluing SQL from strings with input: `' OR 1=1 --` closes the quote and bypasses the filter — "works" in the demo, leaks the whole table at review | 00-04 | values only as parameters (`$1`, `$2`, …): SQL text and data travel to the server separately, the value past the parser, and it cannot become code |
| manual `rows.Scan` with a swapped order of same-type columns: the compiler stays silent, `name` lands in the `category` field | 00-04 → 00-05 | write `query.sql` and generate the mapping via sqlc — a type mismatch is caught by the compiler after `make gen`, not by a user at runtime |
| a new `pgx.Connect` connection per request: under load it hits `max_connections` and `FATAL: too many clients` for everyone, including healthy requests | 00-06 | open the pool once at application startup and reuse connections |
| `Acquire` without a matching `Release`: the connection "leaks" from the pool forever — a slow-motion version of the same mistake | 00-06 | close every `Acquire` with a `Release` (usually a `defer`) |

## Takeaways

- A Postgres connection is a backend process on the server: expensive to open, holding memory, bounded by the server's `max_connections`. Opening one per request is an anti-pattern.
- The pool opens connections once and reuses them: `Acquire` takes one (or opens, or waits at the limit), `Release` returns it **without closing**. The pool is lazy — connections appear on the first Acquire.
- One connection in the pool = one backend on the server; `pool.Stat()` and `pg_stat_activity` show this from both sides and agree.
- Pool size is tuned to the load and `max_connections`; every `Acquire` must have a `Release`.

Late in the evening, once the site is taking orders again, a message pops up in the team chat from Viktor — the founder you know so far only from the framed receipt of order #1.

> **Viktor (in chat):** The site is alive, guests are ordering. In the morning I want our first report: how much we made today. Down to the kopek.

That closes module **00 "Getting connected"**: you have a sandbox, psql at hand, a working pipeline — "SQL by hand → sqlc → typed pgx code" — and an understanding of what the pool does with connections. Next up is module **01 "Data types"**: which type to pick and why, starting with Viktor's report — with money, where `numeric` vs `float` decides whether Brew's till balances down to the kopek.
