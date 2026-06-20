# 01-04 — uuid and uuidv7

Dmitry's prophecy from 00-01 comes true: Evgeny comes down from his floor in person for the first time — phone already turned screen-first toward you.

> **Evgeny:** Small tweak. Every loyalty member gets a personal link to their signup. The mock-up is ready: `brew.app/signup/42`. Flyers go to print by Friday.
>
> **You:** And 42 is the sequential number from the database? It's right there in the URL.
>
> **Evgeny:** A number's a number. Who'd care about it besides us?

The tweak really does turn out small: the id is already there, the link takes an evening. The answer to Evgeny's question arrives a week later — a competitor publishes a post, "Brew has only 42 loyalty signups," and the number is exact: `/signup/42` enumerates, adjacent numbers respond happily, a sequential id leaks both volume and order. The obvious first fix is to switch the key to a random `uuid` (`gen_random_uuid()`, version 4): you can't enumerate that. Another week later, Pavel shows up in the team chat.

> **Pavel (in chat):** loyalty_signups. inserts dropped after the move to v4. "latest signups" no longer sorts by the key. who picked the type.

The second hit lands harder than the first: a random key scatters new rows "all over the place" across the index, and the "latest signups" page dies.

The goal of this unit is to choose a key deliberately. A numeric `IDENTITY` (which we saw on the base tables) is good, but it reveals order and count. A random `gen_random_uuid()` (v4) is unpredictable but doesn't sort and fragments the index. PG18 adds `uuidv7()` — a `uuid` with a timestamp embedded at the front: its tail is just as unpredictable, but it **grows monotonically over time**, so it works as a time-sortable primary key. We demonstrate it on a **new** table — the Brew base schema is byte-compatible with kafka-cookbook (`customers.id` is `BIGINT` there) and must not be touched.

## v4 vs v7: what's embedded in them

`gen_random_uuid()` returns a version-4 `uuid` — 122 random bits. Its plus is unpredictability; its minus is full randomness: values close in time are scattered across the whole range, so they insert into random places in a B-tree index (page fragmentation), and you can't sort by such a key "in creation order."

`uuidv7()` (PG18) puts Unix time in milliseconds into the high bits and randomness into the rest. The version is 7, and it has an embedded timestamp you can extract with `uuid_extract_timestamp()` (for v4 it returns `NULL` — there's no time there). The key property: values generated later are numerically larger — the key is monotonic, inserts go "to the right" in the index, and sorting by the key matches creation order.

## B-tree locality and the key choice

Why "scattered" costs more than "to the tail": the index is a B-tree, and a new row's place is decided by its key.

```
B-tree by key — where does a NEW insert land?

  uuid v4 (122 random bits)           uuidv7 (time in the high bits)
  ┌──┬──┬──┬──┬──┐                    ┌──┬──┬──┬──┬──┐
  │  │  │  │  │  │                    │  │  │  │  │▓▓│ ← all new ones here
  └──┴──┴──┴──┴──┘                    └──┴──┴──┴──┴──┘
   ↑   ↑   ↑   ↑                                    ↑
  hit leaves all over                 one "hot" rightmost leaf
  → page fragmentation                → dense packing, growth "to the tail"
```

| Key | Predictability | B-tree locality | Time-sortable | When to pick |
|---|---|---|---|---|
| `IDENTITY` (`BIGINT`) | low: reveals order and volume | excellent (grows "to the tail") | yes | narrow internal tables; compact and fast |
| `uuid` v4 | high | poor (fragments) | no | a public id where sorting by the key isn't needed |
| `uuidv7` | high in the tail, but leaks time | excellent (grows "to the tail") | yes | distributed inserts + a public sortable id |

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

By the way, this unit's `schema.sql` is the first in the course to add its **own** table: `make db-reset` applies it via `brew.Apply(ctx, pool, ddl)` (base schema → unit DDL → seed), and `main.go` reads that DDL next to itself via `runtime.Caller`.

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

> **Pavel — in review, one line:** `customers.id` stays untouched. It's a contract with the neighboring team.

What we simplified:

- **Monotonicity is about order, not security.** `uuidv7` doesn't hide the creation time (it's easy to extract) — if "when" must stay private, this isn't the tool. Conversely, v4 reveals less but loses on index locality.
- **The key choice is a trade-off (see the table above).** For distributed inserts and public identifiers `uuidv7` is a good default; for narrow internal tables `IDENTITY` is often still better — more compact, faster, though it reveals order/volume and merges poorly across databases.
- **The base tables stay on `BIGINT`.** Deliberately, for byte-compatibility with kafka-cookbook — we try new ideas on new tables, not on the base tables.

## Takeaways

- `gen_random_uuid()` — v4, random: unpredictable, but doesn't sort and fragments the B-tree.
- PG18 `uuidv7()` — v7, with embedded time: monotonic, works as a time-sortable primary key; the time is extracted with `uuid_extract_timestamp()`.
- `uuid` values are random — check and show **properties** (version, monotonicity), not specific values.
- Demonstrate modern idioms (`uuidv7`) on new tables; don't touch the Brew base tables (`*.id BIGINT`) — that keeps the handoff with kafka-cookbook.

Next up — the **01-05 "enums, arrays, and a jsonb intro"** unit: container types — the ordered `enum`, arrays (`text[]` with the `@>` operator), and a first look at `jsonb`; when each container fits, and when it's time to normalize.
