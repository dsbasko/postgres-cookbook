# 01-04 — uuid and uuidv7

Brew decided to give customers a link to their loyalty-program signup and didn't want to expose a sequential numeric id in the URL (`/signup/42` reveals that there are only 42 signups, and adjacent ones are trivial to enumerate). They switched to `uuid` — and immediately hit a second problem: new signups, now keyed by a random `uuid`, started inserting "all over the place" in the index, the "latest signups" page stopped sorting by the key, and inserts got a bit slower. That's how Brew met the difference between a random `uuid` (version 4) and a time-based `uuidv7` (version 7).

The goal of this unit is to choose a key deliberately. A numeric `IDENTITY` (which we saw on the canon tables) is good, but it reveals order and count. A random `gen_random_uuid()` (v4) is unpredictable but doesn't sort and fragments the index. PG18 added `uuidv7()` — a `uuid` with a timestamp embedded at the front: its tail is just as unpredictable, but it **grows monotonically over time**, so it works as a time-sortable primary key. We demonstrate it on a **new** table — the canon is byte-compatible with kafka-cookbook (`customers.id` is `BIGINT` there) and must not be touched.

## v4 vs v7: what's embedded in them

`gen_random_uuid()` returns a version-4 `uuid` — 122 random bits. Its plus is unpredictability; its minus is full randomness: values close in time are scattered across the whole range, so they insert into random places in a B-tree index (page fragmentation), and you can't sort by such a key "in creation order."

`uuidv7()` (PG18) puts Unix time in milliseconds into the high bits and randomness into the rest. The version is 7, and it has an embedded timestamp you can extract with `uuid_extract_timestamp()` (for v4 it returns `NULL` — there's no time there). The key property: values generated later are numerically larger — the key is monotonic, inserts go "to the right" in the index, and sorting by the key matches creation order.

## Why we show properties, not values

A `uuid` is random in its tail by nature — specific values differ on every run, and you can't paste them into a README verbatim. So the demo checks **properties** that are deterministic: the version number (`4` vs `7`), the presence of embedded time (`NULL` for v4, non-`NULL` for v7), and monotonicity — whether the row order by the `uuidv7` key matches the insertion order.

## What our code shows

A dedicated table `loyalty_signups` (DDL in `schema.sql`) keyed on `uuidv7()` with an independent counter `seq` (`IDENTITY`) to check ordering:

```sql
CREATE TABLE IF NOT EXISTS loyalty_signups (
    id   UUID   NOT NULL DEFAULT uuidv7() PRIMARY KEY,
    seq  BIGINT GENERATED ALWAYS AS IDENTITY,
    ...
);
```

The first query is deterministic facts about versions; the third is the monotonicity check:

```sql
SELECT uuid_extract_version(gen_random_uuid())::int AS v4_version,           -- 4
       uuid_extract_version(uuidv7())::int          AS v7_version,           -- 7
       (uuid_extract_timestamp(gen_random_uuid()) IS NULL)::boolean   AS v4_has_no_timestamp,
       (uuid_extract_timestamp(uuidv7()) IS NOT NULL)::boolean        AS v7_has_timestamp;

SELECT bool_and(id_rank = seq_rank)::boolean AS ids_match_insertion_order    -- SignupsTimeOrdered
FROM (SELECT row_number() OVER (ORDER BY id) AS id_rank,
             row_number() OVER (ORDER BY seq) AS seq_rank FROM loyalty_signups) t;
```

`main.go` clears the table, inserts three signups (the DB assigns `id` from `DEFAULT uuidv7()`), and asks: did the order by `id` match the insertion order? For v7 — yes. In Go a `uuid` arrives as `pgtype.UUID`; we don't print the actual values (they're random).

By the way, this unit's `schema.sql` is the first in the course to add its **own** table: `make db-reset` applies it via `brew.Apply(ctx, pool, ddl)` (canon → unit DDL → seed), and `main.go` reads that DDL next to itself via `runtime.Caller`.

## Running it

```sh
docker compose up -d
make lecture L=01-data-types/01-04-uuid-and-uuidv7 T=db-reset
make lecture L=01-data-types/01-04-uuid-and-uuidv7
```

Output:

```
1) gen_random_uuid() (v4) против uuidv7() — проверяемые свойства:
   версия:           v4 = 4,  v7 = 7
   встроено время?   v4: нет (timestamp = NULL) = true;  v7: да = true

2) Вставили строк с ключом uuidv7: 3. Порядок по id = порядку вставки? true
   → uuidv7 монотонен во времени: годится как сортируемый по времени PK.
     (v4 случаен — такой порядок был бы лишь совпадением.)
```

(The demo prints in Russian.) The versions are `4` and `7`; v4 has no embedded time, v7 does. And the key point: three rows inserted in a row are ordered by the `uuidv7` key exactly as by the insertion counter — `uuidv7` is monotonic. The "latest signups" page sorts by the key again, and inserts go "to the right" in the index.

## The fence

What we simplified: the monotonicity of `uuidv7` is about **order**, not security. `uuidv7` doesn't hide the creation time (it's easy to extract), so if it matters that "when" stays private, this isn't the tool. Conversely, v4 reveals less but loses on index locality. In production the choice of key is a trade-off: `IDENTITY` (compact, fast, but reveals order/volume and merges poorly across databases), `uuidv7` (distribution-friendly, sortable, but larger and leaks time), `uuid` v4 (maximally unpredictable but fragments the index). For distributed inserts and public identifiers `uuidv7` is a good default; for narrow internal tables `IDENTITY` is often still better. The Brew canon deliberately stays on `BIGINT` for byte-compatibility with kafka-cookbook — we try new ideas on new tables.

## Takeaways

- `gen_random_uuid()` — v4, random: unpredictable, but doesn't sort and fragments the B-tree.
- PG18 `uuidv7()` — v7, with embedded time: monotonic, works as a time-sortable primary key; the time is extracted with `uuid_extract_timestamp()`.
- `uuid` values are random — check and show **properties** (version, monotonicity), not specific values.
- Demonstrate modern idioms (`uuidv7`) on new tables; don't touch the Brew canon (`*.id BIGINT`) — that keeps the handoff with kafka-cookbook.

Next up — the **01-05 "enums, arrays, and a jsonb intro"** unit: container types — the ordered `enum`, arrays (`text[]` with the `@>` operator), and a first look at `jsonb`; when each container fits, and when it's time to normalize.
