# 07-02 — when NOT to use jsonb

Ruslan messages from BREW-CENTRAL in the middle of the day — the register won't ring up a latte.

> **Ruslan (chat, 13:20):** Latte on the storefront — minus five rubles. Ring it up?
>
> **You:** Minus five? Where does a latte get a negative price?
>
> **Botyr:** Nobody set it. Last sprint I moved the drink card into a `doc jsonb` — more flexible, no schema changes. The price is a key inside the document now.
>
> **Dmitry:** And when did it go negative?
>
> **Botyr:** That's exactly what nobody can say. The column had `CHECK (price > 0)`. Inside the document there's none — a network analyst tried to add one, it won't fit `jsonb`.
>
> **Pavel:** And that's only half of it. Change one flag in the card — you rewrite the whole document. On a hot table I'll bring you the WAL bill.
>
> **Botyr:** It flew in testing. Ten cards — instant.
>
> **Dmitry:** Ten. In production there are thousands, each edited ten times a day. We'll count production, not the test.
>
> **Botyr:** I know. I'll write the postmortem myself. And the rule goes in it: a field with an invariant is a column, not a key in `jsonb`. Don't trust the word "just" — I'm the one who taught that.
>
> **Pavel:** Write it down. And return the constrained fields to the schema. I'll ask separately.

Flexibility in `jsonb` has a price — and that price has two parts.

The goal of this unit is to see both prices of `jsonb` in numbers: the **physical** one (write amplification — changing one field rewrites the whole document) and the **semantic** one (inside `jsonb` there's no type, no `CHECK`, no `NOT NULL` on an individual field). This is a "stop" unit: not "how to do it with `jsonb`," but "where you shouldn't."

## Physics: one field — yet the whole document rewritten

In Postgres rows are immutable: any `UPDATE` writes a brand-new version of the entire row (that's MVCC, see 05-02). But `jsonb` adds its own multiplier on top: a document is stored as a single value, and you can't change one key "in place" — `jsonb_set` builds and writes a **new full document**. If the price lives in a plain `bigint` column, the change touches 8 bytes. If the same price is a key inside a half-kilobyte card, then for the sake of one number the entire half-kilobyte document goes to disk (and to WAL, and to replication). On a hot table with frequent point updates that's write amplification: you write many times more than you change. And if the document grows past ~2 KB it moves to TOAST, and an update drags de/compression along too.

## Semantics: no constraints inside jsonb

A column is a contract: the type rejects `'banana'` in a numeric field, `CHECK (price_cents > 0)` rejects a negative price, `NOT NULL` rejects an omission, a foreign key rejects a dangling reference. Inside `jsonb` none of these guarantees exist. `doc || '{"price": -5}'` writes a negative price silently; `'{"price": "banana"}'` writes a string instead of a number, and the database won't object. You can't require "key `price` is mandatory and positive" in one line: you'd either drag in `CHECK (doc ? 'price' AND (doc->>'price')::numeric > 0)` (fragile, won't catch nested, easy to bypass) or validate in the application — which is exactly the work the schema used to do for you. A field with an invariant wants to be a column.

## One field — the whole document

Why a point update inside `jsonb` is expensive, in a picture. We change the price `450 → 999`:

```
  plain bigint column
    [ 450 ] ──UPDATE──▶ [ 999 ]        8 bytes, the number changes in place

  the same field inside doc (531 bytes):
    { sku, name, price:450, nutrition, sizes, milk_options, allergens, i18n }
                       │  jsonb_set(doc, '{price}', '999')  — rebuild from scratch
                       ▼
    { sku, name, price:999, nutrition, sizes, milk_options, allergens, i18n }
                                       ↑ a NEW full document, 531 bytes again
```

The column touches 8 bytes; `jsonb`, for one number, rewrites all 531 — and that goes to the heap, to WAL, and to replication. On a hot table with frequent edits that's write amplification.

## What our code shows

A lab table `menu_doc_lab`: one drink card where the price exists BOTH as a separate `price_cents` column (type + `CHECK`) AND as a `price` key inside `doc`. A `psql` demonstration (escape-hatch: `sqlc` isn't needed here — we look at bytes and `SQLSTATE`):

```sql
-- 1) how many bytes "changing one field" costs
SELECT pg_column_size(price_cents), pg_column_size(doc) FROM menu_doc_lab;
SELECT pg_column_size(jsonb_set(doc, '{price}', '999')) FROM menu_doc_lab;  -- a new FULL document

-- 2) the column rejects garbage (we print SQLSTATE), 3) jsonb swallows it
INSERT INTO menu_doc_lab (price_cents, doc) VALUES (-5, '{}');        -- 23514 (CHECK)
INSERT INTO menu_doc_lab (price_cents, doc) VALUES ('banana', '{}');  -- 22P02 (type)
UPDATE menu_doc_lab SET doc = doc || '{"price": "banana"}';           -- passes silently
```

## Running it

```sh
docker compose up -d
make lecture L=07-jsonb-arrays-and-search/07-02-when-not-to-use-jsonb T=db-reset
make lecture L=07-jsonb-arrays-and-search/07-02-when-not-to-use-jsonb
```

Output:

```
== 1) write-amplification: байты на одно поле — колонка против jsonb ==
 price_column_bytes | doc_bytes 
--------------------+-----------
                  8 |       531

 doc_after_one_field_change_bytes 
----------------------------------
                              531


== 2) потеря ограничений: колонка отбивает мусор, jsonb — нет ==
колонка price_cents = -5      → SQLSTATE 23514 (CHECK price_cents > 0)
колонка price_cents = banana  → SQLSTATE 22P02 (invalid input for bigint)

== 3) тот же мусор ВНУТРИ jsonb проходит молча (ни типа, ни CHECK) ==
 doc_price_now | column_price_still 
---------------+--------------------
 banana        |                450
```

The column costs 8 bytes, the document 531, and `jsonb_set` returned another 531-byte document for the sake of one field: that's the cost of a point update inside `jsonb`. And only the column could reject garbage: `-5` was caught by the `CHECK` (23514), `banana` by the type (22P02), whereas the same values inside `doc` were written silently, and the card now carries a `banana` price next to an honest column price of `450`.

## The fence

`jsonb` is great for exactly one job: genuinely shapeless, sparse data on which you don't enforce invariants and rarely do point updates — incoming webhooks, snapshots of external APIs, user settings "as is." The signals that a field should move into a column:

- the field gained an invariant — mandatory, range, reference (all of which a column enforces and `jsonb` doesn't);
- it takes frequent point `UPDATE`s (the write amplification above) or a regular filter/join;
- a "fat `jsonb` we keep tweaking a little" — in production that's table and WAL bloat, autovacuum pressure, and loss of control over the data; your DBA will ask you to normalize exactly those fields.

A hybrid schema (stable fields as columns, the truly shapeless tail as one `jsonb`) almost always beats the extremes.

## Takeaways

- Write amplification: an `UPDATE` of one key rewrites the **whole** document (`jsonb_set` returns a new full `jsonb`); a column costs bytes, a document costs hundreds of bytes and TOAST.
- Inside `jsonb` there are no types, `CHECK`, `NOT NULL`, or foreign keys on individual fields — garbage is written silently.
- A field with an invariant / frequent point update / regular filter wants to be a **column**, not a key in `jsonb`.
- `jsonb` fits the shapeless and sparse; a hybrid schema (columns + one `jsonb` tail) beats both extremes.

Next up — **07-03 "SQL/JSON path and building"**: since the shapeless stuff did end up in `jsonb`, we'll learn to extract from it by path expressions (`jsonb_path_query`), patch it precisely (`jsonb_set`), and build documents (`jsonb_build_object`/`jsonb_agg`) — and note that `JSON_TABLE` arrived in PG17.
