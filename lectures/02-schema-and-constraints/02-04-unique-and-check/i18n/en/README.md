# 02-04 — UNIQUE and CHECK (NULLS NOT DISTINCT)

Brew loaded a new price list with a script, and by morning support was flooded: duplicate menu entries appeared, a couple of drinks had a price of `0` (the register gave them away for free), and one had a size `XXL` that fits no cup. The import had run "successfully" — the DB silently swallowed the garbage, because the schema forbade nothing. None of this is fixed in the importer's code, but with two declarative constraints right on the table: `UNIQUE` against duplicates and `CHECK` against meaningless values.

This unit has two constraints and one trap. `UNIQUE` forbids a repeated value — but it behaves unexpectedly with `NULL`: by default `NULL ≠ NULL`, so a `UNIQUE` column accepts any number of `NULL` rows. PG15 added `NULLS NOT DISTINCT` — a mode where `NULL = NULL` and the second `NULL` is already a duplicate. `CHECK` validates the value itself (`price > 0`, `size` from a set) and rejects a violator with `SQLSTATE 23514`. We'll work out which `NULL` behavior you want when.

## UNIQUE and the treachery of NULL

`UNIQUE (slot)` promises: no two rows will share the same `slot`. But `NULL` in SQL means "unknown," and two unknowns aren't equal (`NULL = NULL` → `unknown`, not `true`). So a standard `UNIQUE` treats all `NULL`s as **distinct** and lets through as many as you like. Often that's exactly right: `NULL` means "unset / not applicable," and there can be many such rows (dozens of drinks with no promo slot). Non-null values are still unique, though: a second `'A'` is a duplicate (`SQLSTATE 23505`).

`UNIQUE NULLS NOT DISTINCT` (PG15+) flips this for `NULL`: now `NULL` is considered equal to `NULL`, and the second `NULL` is rejected with the same `23505`. You need this when `NULL` isn't "many N/A" but one specific state that there should be at most one of (the classic — "exactly one active record"). This invariant used to require a partial unique index `WHERE col IS NULL`; now it's a single clause in the declaration.

## CHECK: value validation in the schema

`CHECK (price > 0)` and `CHECK (size IN ('small','medium','large'))` are validation baked into the table: any row violating the condition is rejected with `check_violation` (`23514`). An import with price `0` or size `XXL` simply can't "land" — regardless of which client and which code writes to the DB. That's the point of declarative constraints: the rule lives in one place, the schema, rather than smeared across every code path that inserts something.

## What our code shows

Two tables for the two `UNIQUE` modes and one for `CHECK` (DDL in `schema.sql`):

```sql
CREATE TABLE uniq_default (id ..., slot TEXT, UNIQUE (slot));                  -- NULL ≠ NULL
CREATE TABLE uniq_nnd     (id ..., slot TEXT, UNIQUE NULLS NOT DISTINCT (slot)); -- NULL = NULL
CREATE TABLE check_drink  (id ..., name TEXT NOT NULL,
    price BIGINT NOT NULL CHECK (price > 0),
    size  TEXT   NOT NULL CHECK (size IN ('small','medium','large')));
```

`main.go` pours two `NULL`s into each `UNIQUE` table and compares the result, then tries a duplicate non-null value and three `check_drink` variants:

```go
queries.InsertUniqDefaultNull(ctx); queries.InsertUniqDefaultNull(ctx) // both pass → rows = 2
queries.InsertUniqNNDNull(ctx);     queries.InsertUniqNNDNull(ctx)     // second → 23505 → rows = 1
queries.InsertCheckDrink(ctx, ...Price: 0 ...)   // 23514
queries.InsertCheckDrink(ctx, ...Size: "huge" ...) // 23514
```

Errors are printed as `SQLSTATE` — the code is deterministic, unlike the message text.

## Running it

```sh
docker compose up -d
make lecture L=02-schema-and-constraints/02-04-unique-and-check T=db-reset
make lecture L=02-schema-and-constraints/02-04-unique-and-check
```

Output:

```
1) UNIQUE по умолчанию: NULL ≠ NULL (NULLs distinct)
   две строки slot = NULL          → обе приняты: строк = 2
   дубль непустого slot = 'A'      → отклонён: SQLSTATE 23505 (unique_violation)
2) UNIQUE NULLS NOT DISTINCT (PG15+): NULL = NULL
   две строки slot = NULL          → вторая отклонена: SQLSTATE 23505; строк = 1
3) CHECK (price > 0; size IN ('small','medium','large')):
   price = 0,   size = 'small'     → отклонён: SQLSTATE 23514 (check_violation)
   price = 300, size = 'huge'      → отклонён: SQLSTATE 23514 (check_violation)
   price = 300, size = 'small'     → принят
```

(The demo prints in Russian.) In the first table two `NULL` slots landed calmly (`NULL ≠ NULL`), while the non-null duplicate `'A'` was rejected (`23505`). In the second, those same two `NULL`s are now considered equal — the second became a duplicate, one row remained. And `CHECK` let through neither a zero price nor a size outside the set (both `23514`), while accepting the correct entry. That nightly import, with these constraints, would simply have failed on the bad rows instead of dumping garbage into the menu.

## The fence

What we hid under NOT NULL: `CHECK` **lets `NULL` through**. The condition is violated only if it evaluates to `false`; `NULL` yields `unknown`, and `unknown` ≠ `false` — so a row with `price = NULL` would sail right through `CHECK (price > 0)`. That's exactly why the columns in `check_drink` are marked `NOT NULL`: without it, `CHECK` catches "`-5`" but silently passes "nothing." In production this pair — `NOT NULL` alongside `CHECK` — is kept together deliberately. Two more things your DBA handles: adding a `CHECK`/`UNIQUE` to a large live table is a scan with a lock (so the constraint is first added `NOT VALID`, then validated separately — see 02-06); and complex business validation ("a discount no larger than the order total") shouldn't always be forced into a `CHECK` — it's harder to evolve than code and can't see other tables (for "across rows" you need an EXCLUDE constraint or a trigger). The rule: `UNIQUE`/`CHECK`/`NOT NULL` are for single-row, single-column invariants; they're cheap, declarative, and non-disableable — use them by default.

## Takeaways

- `UNIQUE` forbids duplicates, but by default `NULL ≠ NULL` → several `NULL`s pass (this is "many N/A"); a non-null duplicate → `SQLSTATE 23505`.
- `UNIQUE NULLS NOT DISTINCT` (PG15+) makes `NULL = NULL` → allows at most one `NULL` (this is "exactly one active record"); replaces the old partial-unique-index trick.
- `CHECK` validates the value in the schema (`price > 0`, `size IN (...)`) → a violator is rejected with `23514`.
- `CHECK` lets `NULL` through (the condition is `unknown`, not `false`) — so keep `NOT NULL` next to `CHECK`.

Next up — the **02-05 "Generated columns and domains (PG18 virtual vs stored)"** unit: a value the DB computes itself from other columns (`STORED` on disk vs PG18 `VIRTUAL` on the fly), and a `DOMAIN` — a reusable type with a built-in `CHECK`. It's an escape-hatch unit: `VIRTUAL` is so new the code generator doesn't understand it yet, so we run the lesson with a psql script.
