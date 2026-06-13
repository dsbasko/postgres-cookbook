# 00-05 — Typed queries via sqlc

In 00-04 we read Brew's menu from Go by hand: `pool.Query`, a `rows.Next` loop, `rows.Scan` into struct fields in order. The code works, but it's fragile — and that's not hypothetical:

> **Danya (in chat):** Reordered the columns in the `SELECT` — tests green, but the latte on the storefront moved to the tea category. `Scan` said nothing: `name` and `category` are both `text`, the compiler doesn't care.

That's this whole class of bugs: as long as the types line up, a swapped column order gets past both the compiler and the tests — it surfaces in production, not at review. `rows.Err()` is in the same bucket: forget it and you'll miss a read error.

The goal of this unit is to close that class entirely: we write SQL in `query.sql`, run `sqlc generate`, and get typed Go code where column order and types are checked against the schema at build time. 00-02 already showed this pipeline in broad strokes; here we take it apart as a working tool — with a `$1` parameter, three result shapes, and that very boilerplate from 00-04 now generated for us.

## What sqlc is (and what it isn't)

sqlc is a **code generator**, not an ORM and not a driver. It takes two inputs: the schema (the DDL of your tables) and `query.sql` (the queries you wrote by hand). It parses both, works out which columns and which types each query returns, and generates Go functions that run that query and map the result into typed structs.

> **Danya:** So it's an ORM, just sideways?
>
> **Marat:** No. The SQL stays yours. Only the plumbing is generated.

Marat's verdict is worth unpacking. sqlc doesn't hide queries behind a "magic" API like `.Where("category", cat).First()` — you still write `SELECT ... WHERE category = $1`, and that exact SQL goes to the server. sqlc removes only the mechanical row mapping — the very `Scan` loop we wrote in 00-04.

It's not an ORM: sqlc doesn't manage relationships, doesn't do lazy loading, doesn't build queries dynamically, and doesn't apply migrations (schema and migrations are module 02's job). It does exactly one thing — turn hand-written SQL into type-safe Go. That's why SQL stays at the center of the course.

## query.sql: annotations and result shapes

A query for sqlc is plain SQL plus an annotation line above it:

```sql
-- name: ListDrinksByCategory :many
SELECT id, sku, name, category, base_price
FROM drinks
WHERE category = $1
ORDER BY id;
```

`-- name: ListDrinksByCategory` sets the method name. The suffix sets the result shape:

| Suffix | What the method returns | When to use |
|---|---|---|
| `:many` | `[]XxxRow` — a slice of rows (empty if no match) | `SELECT` returning 0..N rows |
| `:one` | `XxxRow` — one row (or `pgx.ErrNoRows` if there's none) | `SELECT` of exactly one row |
| `:one` (scalar) | the column's own type: `count(*)` → `int64`, no wrapper struct | a single column in a single row |
| `:exec` | only `error` | `INSERT`/`UPDATE`/`DELETE` without `RETURNING` |

And `$1` is the parameter from 00-04, but now sqlc takes care of it. Looking at the schema, it sees that `drinks.category` is `text` and generates a method with the argument `category string`. It takes the parameter's name from the column in the condition — so the method reads as `ListDrinksByCategory(ctx, category string)`, not `(ctx, arg1 string)`.

## What our code shows

From the three queries in `query.sql`, sqlc generated three methods. The signatures (from `internal/db/querier.go`) speak for themselves:

```go
ListDrinksByCategory(ctx, category string) ([]ListDrinksByCategoryRow, error)  // :many
GetDrinkBySKU(ctx, sku string)             (GetDrinkBySKURow, error)            // :one
CountDrinksByCategory(ctx, category string) (int64, error)                      // :one (scalar)
```

The parameters are typed (`category string`, `sku string`), and so is the result: `:many` returns a slice of structs, `:one` returns one struct, and `:one` with a scalar (`count(*)`) returns `int64` directly, without a wrapper. Inside the generated `ListDrinksByCategory` is exactly the `Query → rows.Next → Scan → rows.Err` loop from 00-04 — only the generator wrote it, not you, and it can't get the `Scan` order wrong.

`main.go` after that is thin, like in 00-02:

```go
queries := db.New(pool)
coffee, err := queries.ListDrinksByCategory(ctx, "coffee")   // :many
cold, err := queries.GetDrinkBySKU(ctx, "CLD-01")            // :one
teaCount, err := queries.CountDrinksByCategory(ctx, "tea")   // :one, scalar
```

No `rows.Scan`, no SQL strings in the Go code. If tomorrow you add a column to the `SELECT` and forget to update the mapping — after `make gen` the type simply changes, and the **compiler** points at every place the signature drifted. That's the whole difference from 00-04: the error is caught by the build, not by a user.

## Running it

Bring up the sandbox (from the repo root) and apply the base schema:

```sh
docker compose up -d
make lecture L=00-getting-connected/00-05-typed-queries-with-sqlc T=db-reset
```

Regenerate the code from `query.sql` (optional — it's already committed) and run the demo:

```sh
make lecture L=00-getting-connected/00-05-typed-queries-with-sqlc T=gen
make lecture L=00-getting-connected/00-05-typed-queries-with-sqlc
```

(`T=run` is the default. From inside the unit directory it's `make gen`, `make db-reset`, `make run`.)

Output:

```
1) ListDrinksByCategory("coffee") — :many, $1 типизирован как string:
ID  SKU     НАЗВАНИЕ  КАТЕГОРИЯ  ЦЕНА
1   ESP-01  Эспрессо  coffee     3.00
2   CAP-01  Капучино  coffee     4.50
3   LAT-01  Латте     coffee     4.80

2) GetDrinkBySKU("CLD-01") — :one, одна строка:
   #4  CLD-01  Колд брю  (cold)  5.20

3) CountDrinksByCategory("tea") — :one, скаляр int64:  1
```

(The demo prints in Russian.) `:many` returned three drinks in the `coffee` category, `:one` returned one row by SKU, and the `:one` scalar returned `1` (there's one green tea in the menu). The same data as in 00-04, but all the manual mapping is gone from the code.

> [!NOTE]
> **Check yourself.** (1) `CountDrinksByCategory("tea")` is declared `:one` with a
> scalar — what Go type does the method return, and what value? (2) You added a
> column to the `SELECT` but forgot to fix the mapping — where does that surface: at
> build time or for the user at runtime?

> [!TIP]
> **Answer.** (1) `int64`, no wrapper struct, value `1` — there's one tea on the
> menu (`TEA-01`), as in the output above. It may seem `:one` always returns a
> struct, but for a scalar (one column) sqlc unwraps it and returns the column type
> itself. (2) At build time: after `make gen` the method's type changes and the
> compiler flags every place that no longer lines up. That's the whole win over the
> manual `Scan` from 00-04, where a swapped column order would surface only at
> runtime.

## The fence

- sqlc is type-safe exactly to the extent its schema is truthful. It checks queries against the DDL listed in `sqlc.yaml` (`../../../schema/brew.sql` + `schema.sql`) — if the real database has drifted from the schema files, sqlc won't know (it doesn't connect to the DB during generation). So the source of truth about structure is migrations (module 02), and the schema files in `sqlc.yaml` must stay in step with them.
- sqlc is the course default, not dogma. When a query needs system columns (`xmin`/`ctid`), dynamic SQL, `EXPLAIN`, or interactive sessions — sqlc doesn't fit, and the lesson switches to the escape hatch (raw pgx or psql scripts, as in 00-03).
- We **commit** the generated code: it's reviewed in the diff and doesn't require running `sqlc` on someone else's machine.

## Takeaways

- sqlc is a code generator, not an ORM: you write SQL by hand, it just removes the manual `rows.Scan` and gives type-safe methods. SQL stays at the center.
- The `-- name: X :many|:one|:exec` annotation sets the method name and result shape; `$1` is typed and named from the schema (`category string`).
- A column/type mistake is caught by the **compiler** after `make gen`, not at runtime by a user — that's the win over the manual mapping from 00-04.
- We commit the generated `internal/db/`; `make gen` is reproducible (a re-run produces no diff).

Next up — the **00-06 "connection lifecycle and pooling"** unit: we used the pool as a black box (`pg.NewPool`), and now we'll look inside — how many connections it actually holds, when it opens and closes them, and how to see your own backends in `pg_stat_activity`.
