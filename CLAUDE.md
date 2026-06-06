# CLAUDE.md

Guidance for Claude Code when working in this repository.

## What this repo is

A two-part project:

1. **The course** — Go lecture modules under `lectures/` (a sandbox stack at the
   repo root: Postgres 18 + Adminer) plus the course manifest `course.yaml` and
   markdown content in `lectures/<module>/<slug>/i18n/<lang>/README.md`.
2. **The site** — a static Next.js export that renders the course.

It targets **application developers**, not DBAs. Everything is about writing
queries and shipping code against Postgres; operational topics (replication,
backups, tuning a server, connection-pool sizing for a fleet) are out of scope.

The whole course shares one narrative — **Brew**, a fictional chain of coffee
shops — with the sibling [`kafka-cookbook`](https://github.com/dsbasko/kafka-cookbook).
The two repos use the same data model on purpose (see the byte-compatibility
rule below), so the Postgres capstone `10-05` hands its CDC stream straight into
the Kafka course.

## Site architecture (engine + thin wrapper)

The site engine is a published npm package, **`@dsbasko/cookbook-engine`**. All
UI, data-loading, markdown rendering, i18n, gating, SEO and build config live in
the package. `web/` is a **thin wrapper** — pure data + re-exports, zero TS
logic of its own:

- `web/app/**` — every route file is a bare re-export of an engine entry-point
  (`export { default, generateStaticParams, generateMetadata } from
  '@dsbasko/cookbook-engine/pages/lesson'`). Next requires these symbols to be
  named exports of the route file itself, so re-export is the only form.
- `web/next.config.mjs` — `export default createCookbookConfig()`. The factory
  reads `course.yaml` via `process.cwd()`, enables `output:'export'`, wires
  `transpilePackages` + the `@`→engine webpack alias, and injects
  `brand.siteUrl` → `NEXT_PUBLIC_SITE_URL`.
- Branding is declarative — the `brand` section in `course.yaml` (glyph, level,
  hero, breadcrumb root, OG text, accent). Unlike Kafka (which keeps the
  engine's default accent), this course sets `brand.accent: '#336791'` /
  `brand.accentDark: '#5a9fd4'` to recolour the UI to the Postgres blue.

The repo is a **pnpm workspace** (root `pnpm-workspace.yaml` lists only `web`).
The engine is installed from the registry at an **exact pin** (`@dsbasko/cookbook-engine@1.0.0`,
no caret) — updating the site means a deliberate version bump, not a floating
range. Install from the repo root (`pnpm install`) so there is a single
react/next instance; installing inside `web/` would give two copies and break
React hooks during SSG.

When changing site behaviour, change the **engine version**, not `web/`.

## The Go course (`lectures/`)

`lectures/` is its own Go space tied together by `lectures/go.work` (Go 1.26).
The module root is `github.com/dsbasko/postgres-cookbook` — note this differs
from the repo name only in the org prefix, and differs from Kafka's
`github.com/dsbasko/kafka-sandbox`, so `replace ../../internal` lines are written
fresh, never copied across repos.

Shared boilerplate lives in `lectures/internal/` (its own `go.mod`):

- `pg.NewPool(ctx, opts ...pg.Option)` — a `*pgxpool.Pool` with course defaults.
  `pg.DSN()` reads `DATABASE_URL`, else assembles it from `PG*` with
  sandbox defaults. The pool is lazy; `pg.WithMaxConns(n)` is the escape hatch.
- `brew.Reset(ctx, pool)` / `brew.Apply(ctx, pool, extraDDL ...string)` — apply
  `schema/brew.sql` → per-unit `schema.sql` → `schema/seed.sql`, in that order,
  idempotently. The schema dir resolves via `runtime.Caller` (override with
  `BREW_SCHEMA_DIR`).
- `config.MustEnv(name)` / `config.EnvOr(name, default)` — `os.Getenv` wrappers.
- `runctx.New()` — a context cancelled on `SIGINT` / `SIGTERM`.
- `log.New()` — an `slog` logger with level from `LOG_LEVEL`, **text to stderr**.

Logs go to stderr, stdout stays clean. That is deliberate: a unit's `## Запуск`
section pastes the *actual* stdout of `make run`, so stdout must carry only the
demo's tabular output.

## The sqlc convention (the protagonist)

The default unit shape keeps SQL in the lead role. You write `query.sql` by
hand, run `sqlc generate`, and **commit the generated `internal/db/` package**:

```
schema.sql       # DDL this unit adds on top of schema/brew.sql
query.sql        # ★ hand-written SQL — the centre of the lesson
sqlc.yaml        # canonical config (see below)
internal/db/     # generated: db.go, models.go, querier.go, query.sql.go — committed
cmd/demo/main.go # thin: pgxpool → db.New → typed query → tabwriter
```

`sqlc.yaml` is frozen as the canonical template in
`lectures/00-getting-connected/00-01-client-server-and-sandbox/sqlc.yaml`:
`version: "2"`, `engine: postgresql`, `sql_package: pgx/v5`,
`schema: [../../../schema/brew.sql, schema.sql]` (three levels up reaches the
repo root), `out: internal/db`. Copy it verbatim; the only thing that ever
changes is the `out` path. Pin: `go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0`.
`make gen` must be reproducible — a second run produces no diff.

## The escape hatch

sqlc is the default, not a dogma. When a lesson needs **interactivity, system
columns (`xmin`/`xmax`/`ctid`), concurrent sessions, DDL, or `EXPLAIN`**, drop
sqlc and write `.sql` files driven by psql (`session-a.sql` / `session-b.sql`,
`demo.sql`, or `run.sql`) — or raw pgx (`conn.Query`/`Exec`) in `main.go`. Such
a unit may have no `go.mod` at all (it is then not in `go.work`, and `make build`
ignores it). The reference escape-hatch unit is
`lectures/05-transactions-and-mvcc/05-02-mvcc-mental-model/`.

**Invariant:** every unit — sqlc or escape-hatch — exposes a `make run` target.
For escape-hatch units `run` is an alias onto the main demo or session.

## The Brew canon and the byte-compatibility rule

`schema/brew.sql` has two groups of tables:

- **CANON** — `orders`, `outbox` (+ `outbox_unpublished_idx`),
  `processed_outbox_ids`, `drinks`, `articles`, `customers`. These are
  transcribed **verbatim** from the `init.sql` files of `kafka-cookbook`, down
  to column names and types (`orders.customer_id TEXT`, `drinks.base_price
  BIGINT`, `customers.id BIGINT`). `REPLICA IDENTITY FULL` is set on the three
  CDC sources (`drinks`, `articles`, `customers`). **Never rename a canon
  column.** Capstone `10-05` publishes exactly these tables into logical
  replication, and Debezium on the Kafka side reads them without a schema
  rewrite — a rename breaks the handoff. A DB-free test
  (`TestBrewSchema_ByteCompatCanon`) guards this.
- **RICH** — `shops`, `order_items`, `inventory`. Added for richer relational
  examples (JOIN, LATERAL, window functions). They are not part of the CDC
  handoff, so this is where modern PG18 idioms live: `GENERATED ALWAYS AS
  IDENTITY`, and any `uuidv7()` / generated-column demos go on **new** tables,
  never on the canon.

## Accuracy guardrails (when authoring content)

- `JSON_TABLE` is **PG17, not PG18** — do not attribute it to 18.
- The AIO (async I/O) speedup: present as "up to ~2–3× on read-heavy scans", not
  a flat "3×".
- PG18 OAuth: Go drivers are immature — at most a brief aside, no lesson.
- `EXPLAIN ANALYZE` in PG18 shows `BUFFERS` by default — call this out in `06-01`.
- PG18 is treated as "just the version": modern features are presented as the
  current way to do things, with no version badges and no intro module.

## Common commands

```sh
# Sandbox
docker compose up -d                               # Postgres 18 + Adminer
docker compose down -v                             # tear down + wipe data

# Course (Go)
make list                                          # tree of units
make lecture L=00-getting-connected/00-01-...      # run one unit (defaults to its `run`)
make lecture L=<path> T=help                       # a unit's own help
make build                                         # build all workspace modules (from lectures/)

# Site
pnpm install                                       # workspace install (root)
make web-dev                                       # next dev → localhost:3000
make web-build                                     # static export → web/out/
make web-typecheck / web-lint
make web-check-coverage                            # reconcile course.yaml ↔ filesystem + RU/EN
make web-generate-readme-toc [TOC_LANG=ru]         # regenerate the root TOC
```

`web-check-coverage` / `web-generate-readme-toc` run engine scripts from `web/`
so course data resolves via `process.cwd()`.

> The engine requires **every declared module to have a non-empty `lessons[]`**.
> So `course.yaml` lists only modules that already have at least one finished
> unit; the rest are parked in a comment block and un-commented as their first
> lesson lands. The course publishes incrementally.

## Conventions

- Adding a unit: see "How to add a unit" in `README.md` (folder + `go.mod` +
  `go.work` use-line + `schema.sql`/`query.sql` + `sqlc.yaml` + `cmd/demo` +
  both `i18n/{ru,en}/README.md`, then declare in `course.yaml` and run
  `make web-check-coverage`).
- A unit is "done" only when the verification gate is green: `make db-reset`
  idempotent, `make gen` produces no diff, `make run` output matches what is
  pasted in the README `## Запуск` section, `make build` green, **both** RU and
  EN READMEs present, `make web-check-coverage` green.
- RU-first: write `i18n/ru/README.md` first; `i18n/en/README.md` must exist
  before a unit is marked released.
- Every simplification in a unit carries a "fence" — a line saying what you would
  do differently in production / what your DBA would do.
- AI-plan files go in `docs/plans/`.
