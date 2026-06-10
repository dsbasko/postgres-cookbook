# 02-02 — NOT NULL, PK, natural vs surrogate key

Brew created a shops table and made the primary key a human-readable code: `BREW-CENTRAL`, `BREW-NORTH`. Reasonable — the code is short, meaningful, easy to look up by. Stock levels and order line items referenced that code via foreign keys. And then marketing renamed `BREW-NORTH` to `BREW-NEVA`. That's when it turned out "renaming" means changing the **value of the primary key**: they had to cascade-update the code across every referencing table, and reports that cached the old code started pointing into the void. The pain came not from the rename, but from making a **mutable business code the row's identity**.

This unit has two goals. First: what `PRIMARY KEY` actually is — `NOT NULL` plus `UNIQUE` in one declaration (so the key is never empty and never repeats). Second: when to key on a **natural** code (a business value) versus a **surrogate** id (synthetic, meaningless). Short answer: surrogate for identity, the natural code as a separate `UNIQUE` column for lookups.

## PRIMARY KEY = NOT NULL + UNIQUE

`PRIMARY KEY` isn't a separate "column type" but a pairing of two constraints. By declaring `code TEXT PRIMARY KEY` you automatically got `NOT NULL` (without even writing it) and `UNIQUE`. So you can't put `NULL` into a PK column — that's `not_null_violation` (`SQLSTATE 23502`) — and you can't repeat a value — `unique_violation` (`23505`). These two invariants are what make a row addressable: for any key value there's exactly one row, or none.

`NOT NULL` also works on its own, on a regular column: `name TEXT NOT NULL` catches a missing required field with the same `23502`. It's the first line of validation — in the schema, not in app code — so it can't be bypassed by a forgotten check or a buggy client.

## Natural vs surrogate key

A **natural key** is a business value that's already unique: a shop code, a book's ISBN, a phone number. The plus — no extra column, the key is "meaningful." The minus that shot Brew in the foot: business values **change**, and changing a primary key is painful — foreign keys and caches hold onto it.

A **surrogate key** is a synthetic id (`GENERATED ALWAYS AS IDENTITY` from 02-01) that means nothing beyond "this particular row." The business code doesn't disappear — it lives in a separate column with `UNIQUE` (still convenient to look up by). Renaming the code now doesn't touch the id: foreign keys reference the stable id and don't break. The cost — an extra column and index. For most application tables this is the right default; a natural key is good where the value is truly immutable (an `ISO 3166` country code).

## Which key to use

| Axis | Natural key | Surrogate key |
|---|---|---|
| What it is | a business value as identity (code, ISBN, phone) | a synthetic `id`, meaningless |
| On a business-value change | the key itself changes → drags FKs and caches along | `id` unchanged, only the `UNIQUE`-code attribute changes |
| Extra column and index | no | yes (`id` + `UNIQUE` code) |
| Where "real" uniqueness lives | on the key itself | on the `UNIQUE` code, not the `id` |
| When to use | truly immutable values (country/currency codes), composite junction PKs | the default for most app tables |

## What our code shows

Two tables (DDL in `schema.sql`): one on a natural key, the other on a surrogate with a `UNIQUE` code. Note: `NOT NULL` on `code` in `shop_natural` isn't written — `PRIMARY KEY` imposed it:

```sql
CREATE TABLE shop_natural (
    code  TEXT  PRIMARY KEY,          -- PK ⇒ NOT NULL + UNIQUE automatically
    name  TEXT  NOT NULL
);
CREATE TABLE shop_surrogate (
    id    BIGINT  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code  TEXT    NOT NULL UNIQUE,    -- business code is an attribute, not identity
    name  TEXT    NOT NULL
);
```

`query.sql` hits the PK invariants and shows the contrast on rename:

```sql
-- name: InsertNaturalNullCode :exec     -- NULL into PK → 23502
INSERT INTO shop_natural (code, name) VALUES (NULL, $1);

-- name: RenameNaturalCode :exec         -- changes the KEY VALUE itself
UPDATE shop_natural SET code = $2 WHERE code = $1;

-- name: RenameSurrogateCode :exec       -- changes an attribute, id unchanged
UPDATE shop_surrogate SET code = $2 WHERE code = $1;
```

`main.go` renames `BREW-OLD` → `BREW-NEW` in both tables. In the natural one it checks: the old key is gone, a new one appeared — **the key itself moved**. In the surrogate one — that `id` before and after is the same: only the `code` attribute changed. Errors are printed as `SQLSTATE` (the code is deterministic, the text is not).

## Running it

```sh
docker compose up -d
make lecture L=02-schema-and-constraints/02-02-not-null-pk-natural-vs-surrogate T=db-reset
make lecture L=02-schema-and-constraints/02-02-not-null-pk-natural-vs-surrogate
```

Output:

```
1) PRIMARY KEY = NOT NULL + UNIQUE (таблица на натуральном ключе code):
   NULL в PK-колонку code      → отклонён: SQLSTATE 23502 (not_null_violation)
   дубль code 'BREW-CENTRAL'   → отклонён: SQLSTATE 23505 (unique_violation)
2) NOT NULL на обычной колонке:
   NULL в name                 → отклонён: SQLSTATE 23502 (not_null_violation)
3) Переименование ключа 'BREW-OLD' → 'BREW-NEW':
   натуральный PK (code):  старого ключа нет (false), новый есть (true) — сменилось само значение ключа
   суррогат (id):          id = 1 → 1 неизменен, сменился только атрибут code — identity строки стабильна
```

(The demo prints in Russian.) The PK batted away both `NULL` (`23502`) and a duplicate (`23505`); `NOT NULL` on `name` is the same `23502`. And block 3 shows the key point: the natural key changed entirely on rename (`BREW-OLD` gone, `BREW-NEW` appeared — every foreign key was supposed to "move" with it), whereas the surrogate `id` stayed `1` and only the `code` attribute changed. That's exactly what spares you Brew's pain.

## The fence

What we simplified: we presented the surrogate as an "almost always right default" but didn't finish the foreign-key story — no table here references our `code`/`id`, so the rename cascade stayed off-screen (we'll do it in 02-03). In production the choice of key is a trade-off your DBA and you both keep in mind:

- A surrogate decouples identity from the business value (the recommended default for app tables) but adds a column and an index and requires remembering that "real" uniqueness lives on the `UNIQUE` code, not the id.
- A natural key is good for genuinely immutable values (currency, country codes) and in many-to-many junction tables, where a composite natural PK is natural (our base table `inventory (shop_id, drink_id)` is exactly that).
- Always put `NOT NULL` explicitly on business-required fields: let the schema, not the code, ensure data doesn't arrive empty.

## Takeaways

- `PRIMARY KEY` is `NOT NULL` + `UNIQUE`: the key is never `NULL` (`23502`) and never repeats (`23505`); `NOT NULL` makes the schema the first line of validation.
- A natural key = a business value as identity: it changes with the business, and changing it "drags" the foreign keys along.
- A surrogate key = a synthetic `id` owns identity, the business code lives in a `UNIQUE` column; renaming the code doesn't touch the `id`. This is the right default for most app tables.
- A composite natural key fits junction tables (`inventory (shop_id, drink_id)` in the base schema).

Next up — the **02-03 "Foreign keys (ON DELETE CASCADE/SET NULL)"** unit: how an FK enforces referential integrity (a dangling reference → `23503`) and what to do with children when a parent is deleted — cascade-delete them, null out the reference, or forbid the delete entirely.
