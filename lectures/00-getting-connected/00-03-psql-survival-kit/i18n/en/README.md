# 00-03 — psql survival kit

The morning after the sandbox setup — and the first live ticket. Danya, without turning from his monitor, flips it over to you.

> **Danya:** All yours. "Cold brew disappeared from the menu on the site." Congratulations — this is what a trial by fire looks like around here.
>
> **You:** Check the storefront code?

You're already reaching for the repository when Marat, passing by with his mug, nods at the terminal.

> **Marat:** The code can wait. What did the database say? Three commands and you'll know.

Marat has a point: before digging into code, it's worth a thirty-second peek into the database itself — is that drink even there, what's its `stock`, did the table structure break? You could open Adminer and click around, but that's slow and doesn't fit a terminal workflow. The working tool for this is `psql`: the official Postgres console client, installed alongside the server (`brew install libpq` on macOS).

The goal of this unit is narrow: not to learn all of psql, but to assemble a "first-aid kit" of a handful of commands that covers 90% of "I need a quick look in the DB" cases. This is an escape-hatch unit — there's no Go and no sqlc here, because the lesson is about the client itself.

> [!NOTE]
> Builds on 00-02: the sandbox is up (`docker compose up -d`), and the "client
> sends SQL — server executes" boundary is already familiar. psql is just another
> client to the same server — an interactive one, for hands-on work.

## Meta-commands: what sets psql apart from "just SQL"

psql takes two kinds of input. Plain SQL (`SELECT ...;`) goes to the server and is executed there. Commands starting with a backslash (`\dt`, `\d`, `\x`) are **meta-commands**: psql processes them on the client, before and instead of sending anything to the server. They aren't part of SQL and don't work from the driver in your application — they're a tool for interactive, hands-on work.

```
   you type…                         where it is handled
   ─────────────────────────────────────────────────────────────────────
   \dt   \d   \x   \timing    ──▶  psql (CLIENT) runs it itself, never hits the server
   SELECT … ;   INSERT … ;    ──▶  psql forwards it ──▶ postgres (SERVER) runs it
```

That's exactly why this is an escape-hatch lesson: you can't "write a meta-command into `query.sql` and generate it with sqlc." Yet for a single character they give you what would otherwise take a clunky query against the system catalogs (`information_schema`, `pg_catalog`).

Connect to the sandbox:

```sh
psql 'postgres://brew:brew@localhost:5432/brew?sslmode=disable'
```

Inside you get a `brew=#` prompt. Everything from here is interactive.

## Look around: `\l`, `\dt`, `\d`

Three commands answer "what's even here" at three levels of nesting.

`\l` (list) — the databases on the server. It shows `brew` plus the system `postgres`, `template0`, `template1`. Useful when you're not sure you connected to the right database.

`\dt` (describe tables) — the tables of the current database. In our `brew` that's the 9 base tables. Nearby live `\dv` (views), `\di` (indexes), `\ds` (sequences), `\dn` (schemas), `\df` (functions) — the letter after `\d` narrows the type.

`\d <name>` — the structure of one object: columns, types, nullability, defaults, indexes, and — especially valuable — foreign keys in both directions (what the table references and what references it). `\d drinks` immediately shows that `base_price` is a `bigint` (price in cents, not a float) and that `order_items` and `inventory` reference `drinks`. That's a map of relationships without a single JOIN to `pg_catalog`.

## Make the output readable: `\x` and `\timing`

`\x` (expanded) flips the table "into columns": instead of a wide row that doesn't fit the terminal and wraps into mush, each field prints on its own line as `name | value`. Indispensable for tables with a dozen columns or a long `body`/`description`. `\x auto` is the smart mode: wide results in columns, narrow ones as a normal table.

`\timing` turns on per-query timing (`Time: 2.255 ms`). It's a first, rough "fast/slow" signal — not profiling (that's `EXPLAIN ANALYZE` in module 06), but enough to notice a query suddenly taking seconds. `\timing` output depends on the machine and load, so it isn't in the demo below — try it yourself in `make db-shell`.

## Don't keep it all in your head: `\i`, `\?`, `\h`

`\i <file>` executes a SQL file — exactly how our `make db-reset` applies `schema/brew.sql` and `seed.sql`. Handy for repeatable scripts: instead of pasting a query into the prompt, keep it in a file under version control.

`\?` — help for all meta-commands (there are dozens). `\h <command>` — help on SQL syntax: `\h INSERT` recalls the `INSERT` grammar with all its options without leaving the terminal. Two commands that make memorizing the rest optional.

The whole kit in one table:

| Command | What it does | Where it runs |
|---|---|---|
| `\l` | databases on the server | psql (client) |
| `\dt` | tables of the current database (`\dv`/`\di`/`\ds`/`\dn`/`\df` — by object type) | psql (client) |
| `\d <name>` | object structure: columns, types, PK, indexes, FK both ways | psql (client) |
| `\x` | "into columns" output (`\x auto` — by result width) | psql (client) |
| `\timing` | per-query timing (a rough signal, not profiling) | psql (client) |
| `\i <file>` | run SQL from a file | psql (client) |
| `\?` / `\h` | help: on meta-commands / on SQL syntax | psql (client) |
| `SELECT … ;` | plain SQL — goes to the server and is executed there | postgres (server) |

The last row is for contrast: everything with `\` stays on the client, plain SQL goes to the server (the same boundary as in the diagram above).

## What our code shows

The "code" of this unit is the psql script `demo.sql`: a small tour that runs three key meta-commands over the Brew base schema without changing any data.

```sql
\dt                 -- which tables exist in the database
\d drinks           -- the structure of one table: columns, types, PK, FK
\x on               -- expanded output (a row as a column)
SELECT id, sku, name, category, base_price, stock FROM drinks WHERE sku = 'CLD-01';
\x off
```

`make run` runs it via `psql -f demo.sql` — the same as typing the commands by hand in `make db-shell`, only all at once and reproducibly. That's the kit: look around (`\dt`), break down one table (`\d`), read a row comfortably (`\x`).

## Running it

Bring up the sandbox (from the repo root) and apply the base schema:

```sh
docker compose up -d
make lecture L=00-getting-connected/00-03-psql-survival-kit T=db-reset
```

Run the tour:

```sh
make lecture L=00-getting-connected/00-03-psql-survival-kit
```

(`T=run` is the default. From inside the unit directory it's simply `make db-reset` and `make run`.)

Output:

```
== \dt — какие таблицы есть в базе brew (схема Brew: 9 таблиц) ==========
                List of tables
 Schema |         Name         | Type  | Owner 
--------+----------------------+-------+-------
 public | articles             | table | brew
 public | customers            | table | brew
 public | drinks               | table | brew
 public | inventory            | table | brew
 public | order_items          | table | brew
 public | orders               | table | brew
 public | outbox               | table | brew
 public | processed_outbox_ids | table | brew
 public | shops                | table | brew
(9 rows)


== \d drinks — структура: колонки, типы, PK и кто на таблицу ссылается ==
                          Table "public.drinks"
   Column    |           Type           | Collation | Nullable | Default 
-------------+--------------------------+-----------+----------+---------
 id          | bigint                   |           | not null | 
 sku         | text                     |           | not null | 
 name        | text                     |           | not null | 
 description | text                     |           | not null | 
 category    | text                     |           | not null | 
 base_price  | bigint                   |           | not null | 
 stock       | integer                  |           | not null | 0
 created_at  | timestamp with time zone |           | not null | now()
 updated_at  | timestamp with time zone |           | not null | now()
Indexes:
    "drinks_pkey" PRIMARY KEY, btree (id)
Referenced by:
    TABLE "inventory" CONSTRAINT "inventory_drink_id_fkey" FOREIGN KEY (drink_id) REFERENCES drinks(id) ON DELETE CASCADE
    TABLE "order_items" CONSTRAINT "order_items_drink_id_fkey" FOREIGN KEY (drink_id) REFERENCES drinks(id)


== \x + SELECT — расширенный вывод: одна строка столбиком (для широких) ==
-[ RECORD 1 ]--------
id         | 4
sku        | CLD-01
name       | Колд брю
category   | cold
base_price | 520
stock      | 40
```

(The demo's headers print in Russian; the data is Brew's seed.) Cold brew is right there (`stock = 40`) — so the bug isn't in the data, it's in the application code. That took three commands and ten seconds.

> [!NOTE]
> **Check yourself.** Which of these does psql send to the server, and which does
> it run itself without sending: `\dt`, `SELECT count(*) FROM drinks;`, `\d drinks`,
> `\timing`? And why can't you call `\d drinks` from `pgx` in Go?

> [!TIP]
> **Answer.** Only `SELECT count(*) FROM drinks;` goes to the server — it's SQL.
> The other three start with `\` — they're meta-commands, run by psql itself on the
> client. That's exactly why `pgx` can't call them: `\d` isn't SQL, the server
> doesn't understand such a command; from code you get the same facts by querying
> `pg_catalog`/`information_schema`.

## The fence

- psql is a tool for reconnaissance and debugging **by hand**: look, estimate, test a hypothesis. In production you don't work with data this way — the application doesn't shell out to `psql` and parse text output; it goes to the database through a driver with a typed query (the course reaches that pipeline in 00-04 and 00-05).
- Meta-commands (`\dt`, `\d`) exist only inside psql: you can't call them from `pgx` in Go, they aren't SQL.
- `\d` output and `pg_catalog` are a convenient map, not a contract. The structure your application relies on is pinned by migrations (module 02), not by "I looked via `\d` and it seemed to match."

## Takeaways

- psql takes two kinds of input: SQL (executed by the server) and backslash meta-commands (handled by the client). Meta-commands are not SQL and aren't reachable from the application.
- The everyday kit: `\dt`/`\d` — look around and break down a table; `\x` — readable output; `\timing` — a rough measurement; `\i` — run a file; `\?` and `\h` — help, so you don't have to memorize everything.
- `\d <table>` shows foreign keys in both directions — a map of relationships without queries to the system catalogs.

Next up — the **00-04 "connecting from Go"** unit: we leave interactive psql for application code. We'll open a `pgxpool`, run the first query with a parameter via `$1` — and on an anti-demo we'll see why gluing SQL together with strings opens the door to injection, while binding parameters closes it.
