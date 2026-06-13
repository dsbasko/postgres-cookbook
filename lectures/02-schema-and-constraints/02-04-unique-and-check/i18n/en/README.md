# 02-04 — UNIQUE and CHECK (NULLS NOT DISTINCT)

The nightly price-list import finished "successfully" — at least that's what the script said. It was run by Pasha, Brew's procurement man: suppliers, the stockroom, nightly exports.

> **Pasha (in chat, 23:50):** Price list loaded, all green. The file's just a file.
>
> **Anna (in chat, 08:12):** Ringing up a raf from the new price list — zero rubles. Is that right?

Anna manages the chain's first coffee shop and knows her menu prices to the ruble. And the raf isn't the only casualty: a couple of drinks have a price of `0` (the register was giving them away for free all morning), the menu shows duplicates of the same item, and one drink has a size `XXL` that fits no cup. All of it reached the register because the database **allowed** it: the schema forbade nothing. None of this is fixed in the importer's code, but with two declarative constraints right on the table: `UNIQUE` against duplicates and `CHECK` against meaningless values.

This unit has two constraints and one trap. `UNIQUE` forbids a repeated value — but it behaves unexpectedly with `NULL`: by default `NULL ≠ NULL`, so a `UNIQUE` column accepts any number of `NULL` rows. PG15 added `NULLS NOT DISTINCT` — a mode where `NULL = NULL` and the second `NULL` is already a duplicate. `CHECK` validates the value itself (`price > 0`, `size` from a set) and rejects a violator with `SQLSTATE 23514`. We'll work out which `NULL` behavior you want when.

## UNIQUE and the treachery of NULL

`UNIQUE (slot)` promises: no two rows will share the same `slot`. But `NULL` in SQL means "unknown," and two unknowns aren't equal (`NULL = NULL` → `unknown`, not `true`). That's one facet of `NULL`'s three-valued logic — 03-06 works it out in full; here we take only the consequence for uniqueness. So a standard `UNIQUE` treats all `NULL`s as **distinct** and lets through as many as you like. Often that's exactly right: `NULL` means "unset / not applicable," and there can be many such rows (dozens of drinks with no promo slot). Non-null values are still unique, though: a second `'A'` is a duplicate (`SQLSTATE 23505`).

`UNIQUE NULLS NOT DISTINCT` (PG15+) flips this for `NULL`: now `NULL` is considered equal to `NULL`, and the second `NULL` is rejected with the same `23505`. You need this when `NULL` isn't "many N/A" but one specific state that there should be at most one of (the classic — "exactly one active record": say, at most one of Stas's promos with an empty end date). This invariant used to require a partial unique index `WHERE col IS NULL`; now it's a single clause in the declaration.

## CHECK: value validation in the schema

`CHECK (price > 0)` and `CHECK (size IN ('small','medium','large'))` are validation baked into the table: any row violating the condition is rejected with `check_violation` (`23514`). The price here is the same BIGINT cents as in 01-01: the type guards precision, `CHECK` guards meaning. An import with price `0` or size `XXL` simply can't "land" — regardless of which client and which code writes to the DB. That's the point of declarative constraints: the rule lives in one place, the schema, rather than smeared across every code path that inserts something.

## Two UNIQUE modes

The same `UNIQUE`, two behaviors for `NULL` — you choose by what an empty value means:

| Axis | `UNIQUE` (default) | `UNIQUE NULLS NOT DISTINCT` (PG15+) |
|---|---|---|
| `NULL` comparison | `NULL ≠ NULL` (two unknowns aren't equal) | `NULL = NULL` |
| How many `NULL`s pass | as many as you like | at most one |
| Non-null duplicate | rejected (`23505`) | rejected (`23505`) |
| Meaning of `NULL` | "many N/A": unset / not applicable | "exactly one active record" |
| Replaces | — | the old partial-unique-index trick `WHERE col IS NULL` |

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

What we hid under NOT NULL: `CHECK` **lets `NULL` through**. The condition is violated only if it evaluates to `false`; `NULL` yields `unknown`, and `unknown` ≠ `false` — so a row with `price = NULL` would sail right through `CHECK (price > 0)`. That's exactly why the columns in `check_drink` are marked `NOT NULL`: without it, `CHECK` catches "`-5`" but silently passes "nothing."

- The pair — `NOT NULL` alongside `CHECK` — is kept together deliberately in production, otherwise the check is leaky on empty values.
- Adding a `CHECK`/`UNIQUE` to a large live table is a scan with a lock, and your DBA watches it: the constraint is first added `NOT VALID`, then validated separately (see 02-06).
- Complex business validation ("a discount no larger than the order total") shouldn't always be forced into a `CHECK` — it's harder to evolve than code and can't see other tables (for "across rows" you need an EXCLUDE constraint or a trigger).

The rule: `UNIQUE`/`CHECK`/`NOT NULL` are for single-row, single-column invariants; they're cheap, declarative, and non-disableable — use them by default.

## Takeaways

- `UNIQUE` forbids duplicates, but by default `NULL ≠ NULL` → several `NULL`s pass (this is "many N/A"); a non-null duplicate → `SQLSTATE 23505`.
- `UNIQUE NULLS NOT DISTINCT` (PG15+) makes `NULL = NULL` → allows at most one `NULL` (this is "exactly one active record"); replaces the old partial-unique-index trick.
- `CHECK` validates the value in the schema (`price > 0`, `size IN (...)`) → a violator is rejected with `23514`.
- `CHECK` lets `NULL` through (the condition is `unknown`, not `false`) — so keep `NOT NULL` next to `CHECK`.

Next up — the **02-05 "Generated columns and domains (PG18 virtual vs stored)"** unit: a value the DB computes itself from other columns (`STORED` on disk vs PG18 `VIRTUAL` on the fly), and a `DOMAIN` — a reusable type with a built-in `CHECK`. It's an escape-hatch unit: `VIRTUAL` is so new the code generator doesn't understand it yet, so we run the lesson with a psql script.
