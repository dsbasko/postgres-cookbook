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

**sqlc v1.30.0 can't parse some PG18 / advanced SQL — and several lessons are
*about* exactly those features**, so they go escape-hatch (choose the feature,
not the tool). Confirmed gaps that forced it while authoring: `RETURNING old.*
/ new.*` (03-05, 10-02), `GENERATED … VIRTUAL` (02-05), temporal `PRIMARY KEY
(… WITHOUT OVERLAPS)` and `EXCLUDE USING gist` (10-02), `MERGE … RETURNING
merge_action()` (09-01), the recursive-CTE `CYCLE` clause (08-04), and
multi-array `unnest($1::int[], $2::text[])` (03-01 — use a multi-row `VALUES`
instead). `LISTEN`/`NOTIFY` and `CopyFrom` have no sqlc surface at all (09-04,
09-01). A parser error on a feature the lesson teaches is the signal to drop
sqlc — record the reason in the README fence.

**Escape-hatch `db-reset`** runs psql directly against
`../../../schema/{brew,seed}.sql` (there is no `internal/brew` when the unit
carries no Go). Quiet the NOTICEs with `PGOPTIONS=client_min_messages=warning`.
`go build ./...` from the workspace root does *not* work across the per-unit
modules — `make build` (a `go list -m` loop) is the canonical build, and it
only touches `go.mod` units that are in `go.work`.

## Determinism (so `make run` matches the README)

The `## Запуск` block pastes the *real* stdout of `make run`, and the gate
checks it byte-for-byte — so a demo must print only reproducible things:

- **Never print** `now()` / timestamps, uuid values, a transaction id (`xid`),
  or a backend `pid`. Print a *fact about* them instead — a boolean
  (`created_set`, `ctid_changed`, `xmin_changed`), a count, or a derived
  property (a uuid's *version* and monotonicity, not its value).
- **Errors:** print the `SQLSTATE` (e.g. `23505`, `23514`, `40001`, `42P17`),
  not the message text (locale/build-dependent). Raw error text, NOTICEs and
  WARNINGs go to **stderr**; stdout stays clean.
- **Reseed up front:** `TRUNCATE … RESTART IDENTITY CASCADE` (or a lab-table
  `DROP`+`CREATE`) plus a fixed seed at the start of `run`, so ids start at 1
  and a re-run is bit-identical.
- **EXPLAIN:** `EXPLAIN (ANALYZE, COSTS OFF, TIMING OFF, BUFFERS OFF)` +
  `SET max_parallel_workers_per_gather = 0`, and build lab data with
  `generate_series` (never `random()`). Discuss timing/buffers in prose with a
  hardware caveat.
- **psql output:** `-q`, `\pset footer off`, `\x`, `\set VERBOSITY terse` for
  clean, stable tables.
- **Inherently concurrent demos** (SKIP LOCKED workers, two psql sessions): the
  interleaving / worker split is non-deterministic — print *invariants* (total
  claimed, distinct, duplicates = 0) and make `run` the deterministic
  single-session demo. Two-session scripts can be pinned with `\prompt` (holds a
  transaction open until Enter).

## Authoring patterns (learned building the course)

- **Lab tables.** Demos that need throwaway tables name them `*_lab` and
  `DROP`+`CREATE` (or `TRUNCATE`) them inside `run`, so the run is idempotent and
  the Brew canon is never touched. Modern idioms (uuidv7, virtual generated
  columns, custom enums, extensions) live on these or on the RICH tables.
- **Per-unit DDL from a sqlc demo.** `go:embed` can't reach a sibling
  `schema.sql` from `cmd/demo/`; read it via `runtime.Caller` and apply with
  `brew.Apply(ctx, pool, ddl)` (canon → unit DDL → seed). The committed
  `internal/db/` is still generated from `schema.sql` at `make gen` time.
- **Extensions** (`btree_gist` for 10-02, `pg_trgm` for 07-06) are created in the
  unit's DDL, idempotent via `CREATE EXTENSION IF NOT EXISTS`.
- **Capstones (module 10)** are raw-pgx Go units with a `cmd/demo/main_test.go`
  of asserted integration tests that **skip when the DB is down** (`t.Skip`), so
  `go test ./...` is green without the sandbox; byte-compat is guarded by a
  DB-free token test.
- **Gotchas:**
  - A multi-statement string sent *with bind args* fails with `42601` (extended
    protocol = one command per query). Split DDL into separate no-arg `Exec`
    calls, or run the parameterized statement on its own.
  - A trivial `sql` function gets inlined and *loses* its volatility label; write
    `plpgsql` when the lesson depends on the label surviving (09-05).
  - `go vet` flags a `%`-bearing literal (e.g. `'%presso%'`) passed to `Println`;
    move it to a `%s` argument of `Printf`.
  - The README-TOC generator truncates each lesson label at the first comma — the
    embedded TOC between the `<!-- generated … -->` markers reproduces verbatim,
    so `make web-generate-readme-toc` stays diff-stable.

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

# Course (Go) — these targets live in lectures/Makefile; run from lectures/ (or `make -C lectures …`)
make list                                          # tree of units
make lecture L=00-getting-connected/00-01-...      # run one unit (defaults to its `run`)
make lecture L=<path> T=help                       # a unit's own help
make build                                         # build all workspace modules
make sync                                          # go work sync (after editing go.work)

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
