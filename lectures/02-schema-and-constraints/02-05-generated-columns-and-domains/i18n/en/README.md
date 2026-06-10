# 02-05 — generated columns and domains (PG18 virtual vs stored)

At Brew the line-item total (`qty × unit_price`) was computed in application code. Computed in three places: on the cart page, in the receipt email, and in the nightly revenue report. And at some point the numbers diverged — the cart showed one thing, the receipt another, the report a third. The usual cause: the formula was copied three times, and then one place added rounding and another forgot the discount. The source of truth had crept across the codebase. Meanwhile the rule "a price in cents is strictly positive" lived as a `CHECK` on five tables at once — five copies of one invariant to remember and keep in sync.

The goal of this unit is two tools that move both copy-pastes into the schema. A **generated column** is a column whose value the DB computes itself from other columns of the same row; the formula is single, in the DDL, and can no longer drift. PG18 offers two kinds: `STORED` (computed on write and kept on disk) and the new `VIRTUAL` (computed on the fly on read, takes no space). A **domain (`DOMAIN`)** is a reusable type with a built-in `CHECK`: declare `positive_cents` once, and every column of that type automatically carries its check.

This is an escape-hatch unit, for a pointed reason: `VIRTUAL` is so new that the `sqlc` parser doesn't understand it yet (v1.30.0 fails with `syntax error at or near "VIRTUAL"`). And the lesson is precisely about it — so we run it with a psql script directly; the PG18 server knows the feature.

## A generated column: one formula instead of three copies

`total_stored BIGINT GENERATED ALWAYS AS (qty * unit_price) STORED` is a promise: the value of `total_stored` always equals `qty * unit_price`, and the DB maintains it, not the app. You can't write your own value into such a column (`SQLSTATE 428C9` — exactly like the `GENERATED ALWAYS` id from 02-01): the formula can't be bypassed. The three reports now read the same column — nowhere to diverge.

The `STORED` vs `VIRTUAL` difference is about **when** the value is computed:

- `STORED` — computed on `INSERT`/`UPDATE` and physically stored on disk. Reads are free (the value is already there), writes are a bit more expensive, it takes space. Only such a column can be indexed.
- `VIRTUAL` (PG18, and the SQL-standard default) — not stored on disk at all, computed each time on read. Writes are free, zero space, but reads spend CPU, and you can't index it.

The choice is simple: a value you filter/sort by and read often → `STORED` (pay disk for an index and fast access); a cheap derived value you read rarely → `VIRTUAL` (don't bloat the table).

## A domain: an invariant declared once

`CREATE DOMAIN positive_cents AS BIGINT CHECK (VALUE > 0)` is a new type, "a `BIGINT` that's always positive." A column `price positive_cents` automatically rejects `0` and negatives with the same `SQLSTATE 23514` as the plain `CHECK` from 02-04 — but the rule is written in one place and reused. When the rule changes, you edit the domain, not five tables.

## STORED vs VIRTUAL: the axes

The formula is one and the same — only the moment of computation and the cost differ:

| Axis | `STORED` | `VIRTUAL` (PG18, the standard default) |
|---|---|---|
| When computed | on `INSERT`/`UPDATE` | on read (`SELECT`) |
| On disk | stored (takes space) | not stored (zero space) |
| Write cost | a bit more expensive | free |
| Read cost | free (value is ready) | spends CPU |
| Indexable | yes | no |
| PK/FK, domain types | yes | no |
| When to use | a value in `WHERE`/`ORDER BY`/an index | a cheap, rarely-read derived value |

## What our code shows

The lesson is in `demo.sql`, on a lab table (we don't touch the base tables). A table with two generated columns on one formula, and a domain:

```sql
CREATE TABLE gen_lab (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    qty           INT    NOT NULL,
    unit_price    BIGINT NOT NULL,
    total_stored  BIGINT GENERATED ALWAYS AS (qty * unit_price) STORED,
    total_virtual BIGINT GENERATED ALWAYS AS (qty * unit_price) VIRTUAL
);
CREATE DOMAIN positive_cents AS BIGINT CHECK (VALUE > 0);
```

The demo inserts one row (both columns yield the same `1350`), reads `pg_attribute.attgenerated` (`s` vs `v` — observable proof that they're stored differently), tries to write into a generated column directly, and tests the domain on `0` and `300`. The errors (`428C9`, `23514`) are shown as-is — that's the normal output of a psql escape-hatch unit.

## Running it

```sh
docker compose up -d
make lecture L=02-schema-and-constraints/02-05-generated-columns-and-domains T=db-reset
make lecture L=02-schema-and-constraints/02-05-generated-columns-and-domains
```

Output:

```
== 1) Генерируемый столбец считается из других колонок (qty * unit_price) ==
 qty | unit_price | total_stored | total_virtual 
-----+------------+--------------+---------------
   3 |        450 |         1350 |          1350


== 2) Как столбец хранится (pg_attribute.attgenerated): s = STORED, v = VIRTUAL ==
      col      | gen 
---------------+-----
 total_stored  | s
 total_virtual | v


== 3) Писать в генерируемый столбец напрямую нельзя (как и в GENERATED ALWAYS id) ==
psql:demo.sql:47: ERROR:  cannot insert a non-DEFAULT value into column "total_stored"

== 4) DOMAIN positive_cents = BIGINT + встроенный CHECK (VALUE > 0) ==
-- price = 0 (нарушает CHECK домена):
psql:demo.sql:57: ERROR:  value for domain positive_cents violates check constraint "positive_cents_check"
-- price = 300 (валидно):
 price 
-------
   300
```

(The demo prints in Russian.) Both generated columns yielded `1350` — one formula. `attgenerated` shows `s` and `v`: the values are equal, but `total_stored` sits on disk while `total_virtual` was computed during that very `SELECT`. A direct write into the generated column was rejected (`428C9`), and the domain rejected `0` (`23514`) and accepted `300`. Three copies of the formula and five copies of the `CHECK` shrank to one line of DDL each.

## The fence

What we simplified and where it bites in production (the part your DBA watches):

- `VIRTUAL` in PG18 is tempting as "free," but it has hard limits. It **can't be indexed**, can't be part of a primary/foreign key, and (as we saw while building this unit) doesn't work with user-defined types like domains. So `VIRTUAL` is for cheap derived values you read but don't search by; anything that lands in `WHERE`/`ORDER BY`/an index must be `STORED`.
- `STORED`, in turn, bloats the table and makes writes more expensive — for a heavy formula on a hot table that's noticeable.
- Domains are convenient, but changing the `CHECK` of an existing domain in production means validating all dependent columns under a lock (the same story as `ALTER` in 02-06), and some ORMs/tools introspect domains poorly and just see a `bigint`.
- The key byte-compatibility rule: generated columns and domains go only on **new** tables (`gen_lab` here, `shops`/`order_items` in the base schema), while the six CDC tables of Brew stay verbatim — otherwise the 10-05 handoff breaks.

## Takeaways

- A generated column (`GENERATED ALWAYS AS (expr)`) — the DB computes the value from other columns itself; one formula instead of copies in code, a direct write is rejected (`428C9`).
- `STORED` — computed on write, kept on disk, can be indexed (for values in `WHERE`/`ORDER BY`); `VIRTUAL` (PG18) — computed on read, takes no space, but can't be indexed (for cheap, rarely-read derived values).
- `DOMAIN` — a reusable type with a built-in `CHECK` (`positive_cents`): the invariant is declared once, a violation → `23514`.
- Modern idioms (`VIRTUAL`, domains) — on new tables; the Brew base tables are byte-compatible and untouchable.
- Why escape-hatch: `VIRTUAL` isn't parsed by sqlc yet — the lesson runs with psql directly.

Next up — the **02-06 "ALTER TABLE: a migration mindset"** unit: which schema changes are instant (metadata only) and which rewrite the whole table under a write lock — and how to add constraints in two phases (`NOT VALID` → `VALIDATE`) without taking production down.
