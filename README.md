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
docker compose up -d                                                        # start the sandbox (repo root)
make -C lectures list                                                       # tree of units
make -C lectures lecture L=00-getting-connected/00-01-client-server-and-sandbox  # run one
```

The `list` / `lecture` / `build` targets live in `lectures/Makefile` — run them from
`lectures/` (or via `make -C lectures …` as above); the repo-root `Makefile` carries only the
`web-*` targets.

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
   heading (built up in SQL stages), "what our code shows" (centred on `query.sql`), then
   `## Запуск` with the **real** pasted output, and a hook ending that hands off to the next
   lesson. Add an ASCII diagram and/or a comparison table where Appendix A of
   [`docs/course-canon.md`](docs/course-canon.md) prescribes one. Every simplification gets a
   "fence" — render it as bullets (one production concern each), not a comma-wall. The canon
   (skeleton §6, playbook §7, Appendix A) is the prose source of truth — read it before writing.
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
- [00-02 — psql survival kit](lectures/00-getting-connected/00-02-psql-survival-kit/i18n/en/README.md)
- [00-03 — Connecting from Go](lectures/00-getting-connected/00-03-connecting-from-go/i18n/en/README.md)
- [00-04 — Typed queries via sqlc](lectures/00-getting-connected/00-04-typed-queries-with-sqlc/i18n/en/README.md)
- [00-05 — Connection lifecycle and pooling](lectures/00-getting-connected/00-05-connection-lifecycle-and-pooling/i18n/en/README.md)

### 01 — Data types

Which type to pick and why: numeric vs float for money, text/boolean/null, timestamptz for
time, uuid and PG18 uuidv7, enums/arrays, and an intro to jsonb. The right type up front
removes a whole class of production bugs.

- [01-01 — Numbers and money](lectures/01-data-types/01-01-numbers-and-money/i18n/en/README.md)
- [01-02 — text](lectures/01-data-types/01-02-text-boolean-and-null-teaser/i18n/en/README.md)
- [01-03 — Date](lectures/01-data-types/01-03-date-time-timestamptz/i18n/en/README.md)
- [01-04 — uuid and uuidv7](lectures/01-data-types/01-04-uuid-and-uuidv7/i18n/en/README.md)
- [01-05 — enums](lectures/01-data-types/01-05-enums-arrays-and-jsonb-intro/i18n/en/README.md)

### 02 — Schema

Identity vs serial, NOT NULL, primary and foreign keys, UNIQUE/CHECK, generated columns and
domains (PG18 virtual vs stored), and a migration mindset — which ALTERs are instant and
which rewrite the table and block writes.

- [02-01 — identity and defaults](lectures/02-schema-and-constraints/02-01-identity-and-defaults/i18n/en/README.md)
- [02-02 — NOT NULL, PK, natural vs surrogate key](lectures/02-schema-and-constraints/02-02-not-null-pk-natural-vs-surrogate/i18n/en/README.md)
- [02-03 — Foreign keys (CASCADE/SET NULL)](lectures/02-schema-and-constraints/02-03-foreign-keys/i18n/en/README.md)
- [02-04 — UNIQUE and CHECK (NULLS NOT DISTINCT)](lectures/02-schema-and-constraints/02-04-unique-and-check/i18n/en/README.md)
- [02-05 — Generated columns and domains](lectures/02-schema-and-constraints/02-05-generated-columns-and-domains/i18n/en/README.md)
- [02-06 — ALTER TABLE: a migration mindset](lectures/02-schema-and-constraints/02-06-alter-table-migration-mindset/i18n/en/README.md)

### 03 — CRUD fluency

Confident CRUD: INSERT ... RETURNING, SELECT with WHERE/ORDER/LIMIT and keyset pagination,
safe UPDATE/DELETE, upsert via ON CONFLICT, PG18 RETURNING old/new, and sober NULL semantics
(the NOT IN + NULL trap, COALESCE/NULLIF/IS DISTINCT FROM).

- [03-01 — INSERT and RETURNING](lectures/03-crud-fluency/03-01-insert-and-returning/i18n/en/README.md)
- [03-02 — SELECT: WHERE/ORDER/LIMIT and keyset](lectures/03-crud-fluency/03-02-select-where-order-limit/i18n/en/README.md)
- [03-03 — UPDATE/DELETE safely](lectures/03-crud-fluency/03-03-update-delete-safely/i18n/en/README.md)
- [03-04 — upsert via ON CONFLICT](lectures/03-crud-fluency/03-04-upsert-on-conflict/i18n/en/README.md)
- [03-05 — RETURNING old/new](lectures/03-crud-fluency/03-05-returning-old-new/i18n/en/README.md)
- [03-06 — Sober NULL semantics](lectures/03-crud-fluency/03-06-null-semantics-reckoning/i18n/en/README.md)

### 04 — Querying across tables

Tying data together: joins (inner/left/right/full), self-joins, aggregation with
GROUP BY/HAVING, DISTINCT ON (the latest order per customer), EXISTS vs IN subqueries, and
CTEs with materialization. This is where data turns into answers to business questions.

- [04-01 — JOIN: inner/left/right/full](lectures/04-querying-across-tables/04-01-joins-inner-left-right-full/i18n/en/README.md)
- [04-02 — Multi-table and self-joins](lectures/04-querying-across-tables/04-02-multi-table-and-self-joins/i18n/en/README.md)
- [04-03 — Aggregation: GROUP BY/HAVING](lectures/04-querying-across-tables/04-03-aggregation-group-by-having/i18n/en/README.md)
- [04-04 — DISTINCT ON](lectures/04-querying-across-tables/04-04-distinct-on/i18n/en/README.md)
- [04-05 — Subqueries: EXISTS vs IN](lectures/04-querying-across-tables/04-05-subqueries-exists-vs-in/i18n/en/README.md)
- [04-06 — CTEs and materialization](lectures/04-querying-across-tables/04-06-ctes-and-materialization/i18n/en/README.md)

### 05 — Transactions

How Postgres behaves under concurrency: ACID and transactions, an MVCC mental model
(visible xmin/xmax), row locks and lost updates, isolation levels for developers, retries on
40001, and deadlocks with advisory locks.

- [05-01 — Transactions and ACID](lectures/05-transactions-and-mvcc/05-01-transactions-and-acid/i18n/en/README.md)
- [05-02 — The MVCC mental model](lectures/05-transactions-and-mvcc/05-02-mvcc-mental-model/i18n/en/README.md)
- [05-03 — Row locks and lost updates](lectures/05-transactions-and-mvcc/05-03-row-locks-and-lost-updates/i18n/en/README.md)
- [05-04 — Isolation levels for developers](lectures/05-transactions-and-mvcc/05-04-isolation-levels-for-devs/i18n/en/README.md)
- [05-05 — Retrying on 40001](lectures/05-transactions-and-mvcc/05-05-retry-on-40001/i18n/en/README.md)
- [05-06 — Deadlocks and advisory locks](lectures/05-transactions-and-mvcc/05-06-deadlocks-and-advisory-locks/i18n/en/README.md)

### 06 — Indexing and EXPLAIN

Performance through reading plans: EXPLAIN ANALYZE with buffers (on by default in PG18),
B-tree and column order (PG18 skip-scan), when indexes don't help (expression index),
partial/covering/unique and Index-Only Scan, GIN for jsonb/arrays, CREATE INDEX CONCURRENTLY.

- [06-01 — Reading EXPLAIN ANALYZE](lectures/06-indexing-and-explain/06-01-reading-explain-analyze-buffers/i18n/en/README.md)
- [06-02 — B-tree and column order](lectures/06-indexing-and-explain/06-02-btree-and-composite-column-order/i18n/en/README.md)
- [06-03 — When indexes don't help](lectures/06-indexing-and-explain/06-03-when-indexes-dont-help/i18n/en/README.md)
- [06-04 — Partial, covering, and unique](lectures/06-indexing-and-explain/06-04-partial-covering-and-unique/i18n/en/README.md)
- [06-05 — GIN for jsonb and arrays](lectures/06-indexing-and-explain/06-05-gin-for-jsonb-and-arrays/i18n/en/README.md)
- [06-06 — CREATE INDEX CONCURRENTLY](lectures/06-indexing-and-explain/06-06-create-index-concurrently/i18n/en/README.md)

### 07 — JSONB, arrays, and search

Semi-structured data and in-database search: jsonb access and containment (-> ->> @> ?),
when not to use jsonb, SQL/JSON path and building, arrays vs a junction table, full-text
search (tsvector + GIN), and fuzzy search via pg_trgm — with an FTS/trgm/engine decision
matrix.

- [07-01 — jsonb access and containment](lectures/07-jsonb-arrays-and-search/07-01-jsonb-access-and-containment/i18n/en/README.md)
- [07-02 — When not to use jsonb](lectures/07-jsonb-arrays-and-search/07-02-when-not-to-use-jsonb/i18n/en/README.md)
- [07-03 — SQL/JSON path and building](lectures/07-jsonb-arrays-and-search/07-03-sql-json-path-and-building/i18n/en/README.md)
- [07-04 — Arrays vs a junction table](lectures/07-jsonb-arrays-and-search/07-04-arrays-vs-junction-table/i18n/en/README.md)
- [07-05 — Full-text search](lectures/07-jsonb-arrays-and-search/07-05-full-text-search/i18n/en/README.md)
- [07-06 — Fuzzy search with pg_trgm](lectures/07-jsonb-arrays-and-search/07-06-pg-trgm-fuzzy/i18n/en/README.md)

### 08 — Analytics

Analytics right inside SQL: window functions (running total, ranking, top-N per group),
lag/lead and frames (day-over-day, moving average), recursive CTEs (a category tree),
LATERAL joins (top-3 per customer — the N+1 killer), and grouping sets/rollup/cube.

- [08-01 — Window functions: the basics](lectures/08-analytical-and-lateral/08-01-window-basics-partition-order/i18n/en/README.md)
- [08-02 — Ranking and top-N per group](lectures/08-analytical-and-lateral/08-02-ranking-and-top-n-per-group/i18n/en/README.md)
- [08-03 — lag/lead and window frames](lectures/08-analytical-and-lateral/08-03-lag-lead-and-frames/i18n/en/README.md)
- [08-04 — Recursive CTEs](lectures/08-analytical-and-lateral/08-04-recursive-ctes/i18n/en/README.md)
- [08-05 — LATERAL joins: top-N and the N+1 killer](lectures/08-analytical-and-lateral/08-05-lateral-joins/i18n/en/README.md)
- [08-06 — GROUPING SETS, ROLLUP, and CUBE](lectures/08-analytical-and-lateral/08-06-grouping-sets-rollup-cube/i18n/en/README.md)

### 09 — Writes

Advanced writes and database-side logic: MERGE and COPY, a job queue via SKIP LOCKED, the
transactional outbox, LISTEN/NOTIFY, triggers and function volatility (IMMUTABLE/STABLE/
VOLATILE) — with an explicit "when NOT to put logic in the database" section.

- [09-01 — MERGE and COPY](lectures/09-writes-eventing-and-server-logic/09-01-merge-and-copy/i18n/en/README.md)
- [09-02 — A job queue on SKIP LOCKED](lectures/09-writes-eventing-and-server-logic/09-02-skip-locked-job-queue/i18n/en/README.md)
- [09-03 — Transactional outbox](lectures/09-writes-eventing-and-server-logic/09-03-transactional-outbox/i18n/en/README.md)
- [09-04 — LISTEN / NOTIFY](lectures/09-writes-eventing-and-server-logic/09-04-listen-notify/i18n/en/README.md)
- [09-05 — Triggers and function volatility](lectures/09-writes-eventing-and-server-logic/09-05-triggers-and-volatility/i18n/en/README.md)

### 10 — Use cases

End-to-end capstones with integration tests that tie the whole course into working apps:
building the Brew schema, a price-and-promo engine (PG18 temporal), an app anti-patterns
clinic, pooling from the app (pgbouncer caveats), and the CDC seam — a byte-compatible
handoff into kafka-cookbook.

- [10-01 — Capstone: build the Brew schema](lectures/10-use-cases/10-01-build-the-brew-schema-capstone/i18n/en/README.md)
- [10-02 — Price-and-promo engine](lectures/10-use-cases/10-02-price-and-promo-engine/i18n/en/README.md)
- [10-03 — App anti-patterns clinic](lectures/10-use-cases/10-03-app-anti-patterns-clinic/i18n/en/README.md)
- [10-04 — Pooling from the app](lectures/10-use-cases/10-04-pooling-from-the-app/i18n/en/README.md)
- [10-05 — The CDC seam: handoff to kafka-cookbook](lectures/10-use-cases/10-05-the-cdc-seam-handoff/i18n/en/README.md)

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
