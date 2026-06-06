<div align="center">

# PostgreSQL Cookbook

**Postgres for people who write Go — from your first connection to the patterns you actually ship.**

[![Live site](https://img.shields.io/badge/live-dsbasko.github.io%2Fpostgres--cookbook-336791?style=flat-square)](https://dsbasko.github.io/postgres-cookbook/)
[![Deploy](https://img.shields.io/github/actions/workflow/status/dsbasko/postgres-cookbook/deploy.yml?branch=main&label=pages&style=flat-square)](https://github.com/dsbasko/postgres-cookbook/actions/workflows/deploy.yml)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-18-336791?style=flat-square&logo=postgresql&logoColor=white)](https://www.postgresql.org/)
[![pgx](https://img.shields.io/badge/pgx-v5-336791?style=flat-square)](https://github.com/jackc/pgx)
[![cookbook-engine](https://img.shields.io/npm/v/%40dsbasko%2Fcookbook-engine?style=flat-square&label=cookbook-engine&color=cb3837&logo=npm)](https://www.npmjs.com/package/@dsbasko/cookbook-engine)

[Live site](https://dsbasko.github.io/postgres-cookbook/) ·
[Table of contents](#table-of-contents) ·
[Getting started](#getting-started) ·
[Sandbox stack](#sandbox-stack) ·
[The site](#the-site)

</div>

---

Eleven modules, roughly sixty units, built for **application developers** — the people who
write the queries, not the people who run the server. The arc goes from "what is a client and a
server, and how do I connect" to the things you actually ship: transactions and isolation under
real concurrency, indexes you can defend with `EXPLAIN`, JSONB and full-text search, window
functions, the transactional outbox, and a CDC seam that hands off to a second course.

No DBA, no DevOps. Replication, backups and server tuning stop where they start — this is about
writing SQL and shipping Go against it. Every unit is a self-contained Go module you run against
one sandbox at the repo root (Postgres 18 + Adminer), and every unit leaves an observable trace:
rows changed, a plan you can read, a publication you can stream. SQL is written by hand, typed
with [sqlc](https://sqlc.dev/), and the demo output pasted into each README is the real output,
not a sketch.

One story threads all eleven modules: **Brew**, a fictional chain of coffee shops whose backend
grows as the course moves forward. The same tables — orders, drinks, customers, an outbox — run
from the first connection to the last capstone. That data model is shared, byte-for-byte, with
the sibling [`kafka-cookbook`](https://github.com/dsbasko/kafka-cookbook): the final unit here
(`10-05`) opens a CDC stream that the Kafka course picks up. See
[A shared universe](#a-shared-universe-with-kafka-cookbook) below.

Every unit ships in Russian and English. The live site has a language toggle; in the repo each
unit carries both `i18n/ru/README.md` and `i18n/en/README.md`. RU is authored first.

> The course publishes **incrementally** — a module appears on the site once its first unit is
> done. The [table of contents](#table-of-contents) lists what is live today; the full module
> plan is in [`docs/plans/`](docs/plans/).

## The site

The same material as a static site: course navigation, syntax highlighting, RU / EN toggle,
light / paper / dark themes, reading preferences (font size and family, set separately for prose
and code), and a free-reading mode that unlocks every lesson and hides progress tracking.

> **[dsbasko.github.io/postgres-cookbook](https://dsbasko.github.io/postgres-cookbook/)**

The site is split in two. The engine —
[`@dsbasko/cookbook-engine`](https://www.npmjs.com/package/@dsbasko/cookbook-engine), a published
npm package — holds all the UI, data loading, markdown rendering, i18n, SEO and build config.
`web/` is a thin wrapper with no TypeScript logic of its own: every route in `app/**` is a bare
re-export of an engine entry-point, and `next.config.mjs` is a single `createCookbookConfig()`
call. The course itself is data — `course.yaml` plus `lectures/`. Branding lives in the `brand`
section of `course.yaml` (glyph, level, hero, breadcrumb, OG text, and the Postgres-blue accent
`#336791`). Updating the site means bumping the `@dsbasko/cookbook-engine` version — it is pinned
exactly, with no caret.

A local run needs Node ≥ 20 and pnpm 9.15.0. The repo is a pnpm workspace, so install once from
the root:

```sh
pnpm install     # workspace install — hoists a single react/next instance
make web-dev     # dev server at http://localhost:3000
make web-build   # static export into web/out/
```

Deployment is automatic. [`.github/workflows/deploy.yml`](.github/workflows/deploy.yml) builds the
static export and publishes it to GitHub Pages on every push to `main` (enable it once under
Settings → Pages → Source: "GitHub Actions"). The canonical origin for the sitemap and OG tags
comes from `brand.siteUrl` in `course.yaml`, wired into `NEXT_PUBLIC_SITE_URL` at build time.

## Stack

- Go 1.26
- PostgreSQL 18 — run as `postgres:18-alpine`, modern features treated as just the current way
- [jackc/pgx](https://github.com/jackc/pgx) v5 + `pgxpool` — the driver used throughout
- [sqlc](https://sqlc.dev/) — the protagonist: hand-written `query.sql` → `sqlc generate` → typed pgx code, committed
- psql (libpq) — for the interactive / EXPLAIN / concurrency units
- Adminer — a lightweight web UI for the sandbox database

## Sandbox stack

Everything the units talk to runs from one [`docker-compose.yml`](docker-compose.yml): a single
Postgres instance shared across the whole course (each unit layers its own `schema.sql` on top of
the Brew canon via `make db-reset`, so one database is enough), plus Adminer as a web UI. Bring it
up from the repo root with `docker compose up -d`:

| Service | Image | Local endpoint |
|---------|-------|----------------|
| Postgres 18 | `postgres:18-alpine` | `localhost:5432` |
| Adminer | `adminer:4.8.1` | http://localhost:8090 |

The instance starts with `wal_level=logical` and replication slots already configured, so the
module 09 eventing units and the `10-05` CDC capstone work without a per-unit
`docker-compose.override.yml`. Both ports are bound to loopback — the sandbox is for local
development and does not face the network.

## Getting started

Bring up the sandbox, then run a unit:

```sh
docker compose up -d                                                        # start the sandbox
make list                                                                   # tree of units
make lecture L=00-getting-connected/00-01-client-server-and-sandbox          # run one
```

`make lecture` delegates into the unit's own Makefile and defaults to its `run` target; pass
`T=help` to see a unit's own help, `T=db-reset` to reset its schema, and so on. Every unit —
whether sqlc-based or an escape-hatch (psql / raw-pgx) unit — exposes a `run` target.

The pool reads its connection from the environment; the defaults already match the local sandbox:

```sh
DATABASE_URL=postgres://brew:brew@localhost:5432/brew?sslmode=disable
# or the equivalent PG* vars:
# PGHOST=localhost PGPORT=5432 PGDATABASE=brew PGUSER=brew PGPASSWORD=brew
LOG_LEVEL=info   # debug | info | warn | error — text to stderr, so stdout stays clean
```

## Repository layout

```
.
├── docker-compose.yml      # the sandbox: Postgres 18 + Adminer
├── course.yaml             # course manifest + branding (consumed by the site engine)
├── schema/                 # the Brew canon: brew.sql (baseline) + seed.sql
├── web/                    # thin Next.js wrapper over @dsbasko/cookbook-engine
└── lectures/
    ├── go.work             # workspace tying all unit modules together
    ├── Makefile            # delegates `make` into individual units
    ├── internal/           # shared helpers: pg pool, brew schema, config, runctx, log
    ├── 00-getting-connected/
    │   └── 00-01-client-server-and-sandbox/
    │       ├── go.mod
    │       ├── i18n/{ru,en}/README.md
    │       ├── Makefile
    │       ├── sqlc.yaml · schema.sql · query.sql
    │       ├── internal/db/        # generated by sqlc — committed
    │       └── cmd/demo/main.go
    └── 05-transactions-and-mvcc/ … 10-use-cases/
```

A typical unit is its own Go module with a `go.mod`, a README in Russian and English, a Makefile,
hand-written `query.sql`, and the sqlc-generated `internal/db/`. Escape-hatch units (interactive,
EXPLAIN, concurrent-session, or DDL lessons) trade sqlc for `.sql` files driven by psql and may
carry no Go at all. Use cases (module 10) are bigger: several files, integration tests, sometimes
a `docker-compose.override.yml`.

## Shared helpers

Common boilerplate lives in `lectures/internal/` instead of being copied into every unit:

- `pg.NewPool(ctx, opts ...pg.Option)` — a `*pgxpool.Pool` with course defaults (reads
  `DATABASE_URL`, else assembles it from `PG*`; `pg.WithMaxConns(n)` is the escape hatch)
- `brew.Reset(ctx, pool)` / `brew.Apply(ctx, pool, extraDDL ...string)` — apply the Brew canon
  (`schema/brew.sql`) → a unit's `schema.sql` → `schema/seed.sql`, idempotently
- `config.MustEnv(name)`, `config.EnvOr(name, default)` — wrappers over `os.Getenv`
- `runctx.New()` — a context cancelled on `SIGINT` / `SIGTERM`
- `log.New()` — an `slog` logger with level from `LOG_LEVEL`, text to stderr

`go.work` resolves the local path to `internal` for every module, so each unit just requires it:

```go.mod
require (
    github.com/dsbasko/postgres-cookbook/lectures/internal v0.0.0
    github.com/jackc/pgx/v5 v5.9.2
)

replace github.com/dsbasko/postgres-cookbook/lectures/internal => ../../internal
```

## A shared universe with kafka-cookbook

This course and [`kafka-cookbook`](https://github.com/dsbasko/kafka-cookbook) are siblings: same
engine, same pedagogy, same Brew story. They share the data model on purpose. Six tables —
`orders`, `outbox`, `processed_outbox_ids`, `drinks`, `articles`, `customers` — are defined here
in [`schema/brew.sql`](schema/brew.sql) **byte-for-byte identical** to the `init.sql` files in the
Kafka course, down to column names and types.

That is not cosmetic. The Postgres capstone `10-05` sets `REPLICA IDENTITY FULL` on the CDC
sources and runs `CREATE PUBLICATION dbz_publication`, opening a logical-replication stream.
Because the schema matches exactly, Debezium on the `kafka-cookbook` side reads that stream
without a single schema rewrite — the Postgres course literally hands the baton to the Kafka
course. The byte-compatibility rule (never rename a canon column) is what keeps that handoff
working; it is enforced by a test and documented in [`CLAUDE.md`](CLAUDE.md).

## How to add a unit

1. Create `lectures/<NN-module>/<MM-slug>/` with a `go.mod` whose module path is
   `github.com/dsbasko/postgres-cookbook/lectures/<NN-module>/<MM-slug>`, plus a
   `replace github.com/dsbasko/postgres-cookbook/lectures/internal => ../../internal`.
2. Add `use ./<NN-module>/<MM-slug>` to `lectures/go.work`.
3. Write `schema.sql` (this unit's DDL) and `query.sql` (the hand-written SQL at the centre of the
   lesson). Copy `sqlc.yaml` verbatim from the reference unit
   (`00-getting-connected/00-01-client-server-and-sandbox`) — only the `out` path ever changes —
   and run `make gen` to produce `internal/db/`.
   *Escape-hatch units* (interactive / EXPLAIN / concurrent sessions / DDL) skip sqlc: write
   `session-a.sql` / `session-b.sql` / `demo.sql` driven by psql, or raw pgx in `main.go`.
4. Write a thin `cmd/demo/main.go` and a `Makefile` (`help · run · gen · db-reset · db-shell ·
   build · clean`) modelled on the reference units. Every unit must expose `run`.
5. Write `i18n/ru/README.md` along the arc — cold open with a Brew incident, one concept per
   heading, "what our code shows" (centred on `query.sql`), then `## Запуск` with the **real**
   pasted output. Every simplification gets a "fence" — a line on what you would do in production.
6. Run the verification gate: `make db-reset` idempotent, `make gen` no diff, `make run` output
   matches the README, `make build` green. Then write `i18n/en/README.md` before marking the unit
   released.
7. Declare the lesson in `course.yaml` (`id`, `title.{ru,en}`, `duration`, `tags`) and un-comment
   its module if this is the module's first unit. Run `make web-check-coverage` — it reconciles
   the manifest with the filesystem and reports any mismatch. Regenerate the table below with
   `make web-generate-readme-toc` (add `TOC_LANG=ru` for the Russian variant).

## Table of contents

The course is ordered by difficulty. Module 00 is the on-ramp; after that the modules build on
one another roughly in order. Every lesson is available in both languages: the links below point
to the English README, and the Russian one sits next to it under `i18n/ru/`. On the
[site](https://dsbasko.github.io/postgres-cookbook/) the toggle does the same.

<!-- generated by: make web-generate-readme-toc -->

### 00 — Getting connected

Client-server, the local sandbox (postgres:18 + Adminer), psql as a working tool,
connecting from Go via pgxpool, and typed queries via sqlc. After this module you have a
working pipeline — "SQL by hand → sqlc generate → typed pgx code" — that every other topic
builds on.

- [00-01 — Client](lectures/00-getting-connected/00-01-client-server-and-sandbox/i18n/en/README.md)

### 05 — Transactions

How Postgres behaves under concurrency: ACID and transactions, an MVCC mental model
(visible xmin/xmax), row locks and lost updates, isolation levels for developers, retries on
40001, and deadlocks with advisory locks.

- [05-02 — The MVCC mental model](lectures/05-transactions-and-mvcc/05-02-mvcc-mental-model/i18n/en/README.md)

<!-- end generated -->

## What's not covered

This is a course for application developers, so it stops where DBA and DevOps work begins.
Replication and high availability, backups and PITR, server-config tuning, monitoring a fleet,
roles and security hardening, extensions management, and migration tooling are all out of scope —
they belong to whoever runs the database, not whoever writes the queries. Where a unit simplifies
something a production system would treat differently, it says so in a "fence" and points at what
the real answer would be.

If something on the sandbox misbehaves, check [`docker-compose.yml`](docker-compose.yml) and the
container logs (`docker compose logs postgres`).
