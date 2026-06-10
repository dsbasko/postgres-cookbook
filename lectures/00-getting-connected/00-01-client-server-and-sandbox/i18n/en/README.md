# 00-01 — Client, server, and the sandbox

First day at Brew. Somewhere there's a Postgres running — with orders, the menu, customers — and your job is to work with it. Not to administer it, not to set up replication, but to write an application that reads and writes data. Before touching types, indexes, and transactions, two boring but mandatory things: understand what is actually on the other end of the connection, and get a local copy you're not afraid to break.

That's the whole plot of this unit. No SQL heroics — connect, ask the server its version, read the menu. Everything that follows rides on this same pipeline.

## Client and server: what's on the other end of the socket

Postgres is a server process. It owns the data files, and nothing except it touches those files. Everything in your hands — `psql`, the `pgx` driver in Go, the Adminer web UI — is a client. A client opens a connection, sends the query text over the network (TCP on port 5432, or a local unix socket), and reads rows back. The protocol between them is fixed — the Postgres wire protocol — but you don't need to know it: the driver speaks it for you.

One connection, two ends:

```
   N clients                                                one server process
   (send SQL, read rows back)                               (does all the work)

   psql ──┐
   pgx ───┼──▶  connection over a socket (TCP :5432 / unix) ──▶  postgres
   Adminer┘     wire protocol                                     • parses SQL, builds a plan
                                                                  • executes, runs MVCC and locks
                                                                  • owns the data files — only it
```

Why this matters in practice, not just in administration theory. Every word that comes up later — "connection", "pool", "timeout", "the connection dropped" — is about this very socket. The server does the work: it parses SQL, builds a plan, executes it, runs MVCC and locking. The client only sends the query and collects the result. One server serves many clients at once — and the whole concurrency story (module 05) grows from exactly this: several connections, one set of data.

The practical takeaway for today is simple: to do anything with data you need a connection. Let's open one and confirm there really is a live Postgres 18 on the other end.

## The sandbox: one Postgres for the whole course

The local stand is one Postgres 18 container plus Adminer as a web client. Bring it up from the repository root:

```sh
docker compose up -d
```

A single database `brew` serves the whole course. Each unit layers its own schema on top of the shared Brew base schema (`schema/brew.sql`) via `make db-reset` — no separate container per unit. The base tables are the coffee-shop's tables: `drinks` (the menu), `customers`, `orders`, `shops`, and others. We fill them with deterministic seed data: fixed `id`s, fixed `created_at`s. That's why the demo output reproduces verbatim — and why it's safe to paste straight into a README.

`make db-reset` is idempotent: under the hood it applies the schema via `IF NOT EXISTS` and the seed via `TRUNCATE ... RESTART IDENTITY` before inserting. Run it as many times as you like — the database always lands in the same state. This isn't cosmetic: reproducibility is what separates "works on my machine" from "works for everyone".

Adminer sits at `http://localhost:8090` (System: PostgreSQL, Server: `postgres`, login/password `brew`/`brew`) — handy for eyeballing the tables, but optional. The working client of this course is Go.

The default connection parameters are tuned for this stand:

```
DATABASE_URL=postgres://brew:brew@localhost:5432/brew?sslmode=disable
```

`internal/pg.NewPool` reads `DATABASE_URL` (or assembles the string from `PG*` variables), so there's no connection string inside the unit's code — there's a single call.

## What our code shows

At the center of the lesson is `query.sql`. It isn't a helper file — it *is* the lesson: we write SQL by hand, and it stays readable SQL instead of dissolving into a query builder.

```sql
-- name: ServerVersion :one
SELECT version();

-- name: ListDrinks :many
SELECT id, sku, name, category, base_price
FROM drinks
ORDER BY id;
```

`make gen` runs `sqlc generate`: sqlc reads `query.sql` together with the schema (the Brew base schema plus the unit's additions) and generates typed Go code into `internal/db/`. From `-- name: ListDrinks :many` you get a method `ListDrinks(ctx) ([]ListDrinksRow, error)`, where `ListDrinksRow` is a struct with fields of exactly the table's types (`base_price BIGINT` → `int64`). We **commit** the generated code: it's part of the repo, gets reviewed in the diff, and doesn't require code generation on someone else's machine.

`main.go` after that is thin. Its whole essence is four lines:

```go
pool, err := pg.NewPool(ctx)      // connection pool to the sandbox
queries := db.New(pool)           // typed wrapper from sqlc
version, err := queries.ServerVersion(ctx)
drinks, err := queries.ListDrinks(ctx)
```

No manual `rows.Scan`, no SQL string literals in the Go code — all of that is generated from `query.sql`. This pipeline — "SQL by hand → `sqlc generate` → typed pgx code" — is the spine of every unit in the course. From here on, only the queries change.

## Running it

Bring up the sandbox (from the repo root) and apply the base schema:

```sh
docker compose up -d
make lecture L=00-getting-connected/00-01-client-server-and-sandbox T=db-reset
```

Run the demo:

```sh
make lecture L=00-getting-connected/00-01-client-server-and-sandbox
```

(`T=run` is the default, so without `T=...` the demo runs straight away. From inside the unit directory it's simply `make db-reset` and `make run`.)

Output:

```
Сервер: PostgreSQL 18.4 on aarch64-unknown-linux-musl, compiled by gcc (Alpine 15.2.0) 15.2.0, 64-bit
Напитков в меню Brew: 5

ID  SKU     НАЗВАНИЕ     КАТЕГОРИЯ  ЦЕНА
1   ESP-01  Эспрессо     coffee     3.00
2   CAP-01  Капучино     coffee     4.50
3   LAT-01  Латте        coffee     4.80
4   CLD-01  Колд брю     cold       5.20
5   TEA-01  Зелёный чай  tea        2.50
```

(The demo prints in Russian; the menu is Brew's seed data.) Your version line may differ in its tail — `aarch64` vs `x86_64`, the gcc version — that depends on the architecture the `postgres:18-alpine` image was built for. What matters is the `PostgreSQL 18.x` at the start: on the other end of the socket is exactly the version this course targets. The menu table reproduces verbatim — ids, prices, and order are fixed by the seed data.

## The fence

- The sandbox is `docker compose` on your laptop. In production a DBA runs and owns the server: the Postgres version and upgrades, `max_connections`, memory, disk, backups — their turf, not yours. You're on the other end of the socket, a client, not the server's operator.
- `sslmode=disable` is for the local stand only. In production connections are encrypted (TLS), and the password comes from a secrets manager — it doesn't sit in a `DATABASE_URL` under version control.
- One shared `brew` database for the whole course is a teaching simplification. In a real project environments live in separate databases and instances, and you never run `make db-reset` (a `TRUNCATE` under the hood) against live data.

## Takeaways

- Postgres is the server; `psql`, `pgx`, Adminer are clients that talk to it over a connection. A client never touches the data — it sends SQL and reads rows.
- The sandbox is one container for the whole course; `make db-reset` idempotently returns the DB to a reference state, so the demo output is reproducible.
- The course pipeline: `query.sql` (by hand) → `make gen` (sqlc) → typed code in `internal/db/` (committed) → a thin `main.go`.

Next up — the **00-02 "psql survival kit"** unit: the same server, but seen through `psql` directly — `\dt`, `\d`, `\x`, `\timing`. It's a working tool you'll reach for in every later unit when you want to peek into the DB by hand, without Go.
