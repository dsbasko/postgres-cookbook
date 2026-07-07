# 07-01 — jsonb access and containment

Evgeny comes down with his phone screen-forward — on the screen, a guest's order: "oat milk, double syrup, decaf, cinnamon on top."

> **Evgeny:** The "everything with a gift" filter works. Now I want the options themselves — right in the orders. This guest built this one, the next builds something else. Everyone has their own set — I need a field for everything.
>
> **You:** Add columns? `milk`, `size`, `syrup`, `extras`…
>
> **Dmitry:** How many will there be by spring? Syrup, second syrup, temperature, "no lid," allergens. Forty nullable columns. You can. It'll hurt.
>
> **You:** And if we don't know the set in advance?
>
> **Dmitry:** Then you don't spread it across columns. The shapeless and sparse go into one `jsonb`. One `options` column, and any set fits.
>
> **Evgeny:** And can I filter by them? Find every oat-milk order?

Filter — you can: `jsonb` has its own operator for that, and the very GIN from 06-05 speeds it up. First we'll learn to extract and check.

The goal of this unit is to master four working `jsonb` access operators: `->` and `->>` (extract a value), `#>>` (extract by path), and `@>` (containment, "contains"), plus `?` (does the key exist). This is the foundation of any application work with `jsonb`; deep `jsonb_path`, document building, and indexes come in the next units of the module.

## `->` vs `->>`: jsonb or text

Two twin operators, and confusing them is a classic mistake. `->` extracts the value as **`jsonb`**: `options -> 'milk'` returns `"oat"` — with surrounding quotes, because it's still a json string. `->>` extracts the same value as **`text`**: `options ->> 'milk'` returns plain `oat`. When you compare, concatenate, or print — you almost always want `->>`. You need `->` when you drill deeper: `options -> 'meta' -> 'flags'` is a chain over a nested object, `jsonb` again at each step. Correspondingly `#>` and `#>>` are the same "extract" but by **path**: `options #>> '{extras,0}'` descends into the `extras` array and takes element zero as `text`.

## Containment `@>`: "the document contains a pair"

`@>` is the main `jsonb` search operator. `options @> '{"milk":"oat"}'` is true if the left-hand document **contains** the pair `"milk":"oat"` — regardless of whatever else is in it. This is not whole-document equality (that's almost never tested), it's "does it contain." Containment can look inside arrays too: `options @> '{"extras":["honey"]}'` finds orders whose `extras` array contains `honey`. One operator covers both flat fields and nested structures — and it's exactly what a GIN index speeds up (see 06-05): on a large table `@>` without an index is a `Seq Scan`, with GIN it's a `Bitmap Index Scan`.

## `?`: does the key exist

`?` answers a different question — not "what value" but "does this top-level key exist at all." `options ? 'extras'` is true for orders where `extras` is present — even if the array is empty (`[]`). That's an important difference from `@>`: an empty `extras` has the key but no value inside, so `@> '{"extras":[...]}'` won't catch it while `? 'extras'` will. Next to it live `?|` (any of the keys exist) and `?&` (all of the keys exist) — the same logic for a list.

## What our code shows

A lab table `order_options_lab` (in `schema.sql`) with five orders whose `options` are deliberately heterogeneous. Four queries:

```sql
SELECT customer,
       coalesce((options -> 'milk')::text, '∅') AS milk_jsonb,   -- "oat" (jsonb)
       coalesce(options ->> 'milk', '∅')         AS milk_text,    -- oat   (text)
       coalesce(options #>> '{extras,0}', '∅')   AS first_extra   -- by path
FROM order_options_lab ORDER BY id;                               -- AccessOps

SELECT customer FROM order_options_lab WHERE options @> '{"milk":"oat"}';        -- OatMilkOrders
SELECT customer FROM order_options_lab WHERE options @> '{"extras":["honey"]}';  -- HoneyInExtras
SELECT customer FROM order_options_lab WHERE options ? 'extras';                 -- HasExtrasKey
```

`coalesce` substitutes a missing value with `∅` (Egor has no `milk`, Boris has no `extras`) and also gives sqlc the type `string` instead of a nullable `interface{}`. Like units 01-04/01-05, this one adds its own object to the schema, so `make db-reset` applies it via `brew.Apply` (Brew base schema → unit DDL+seed → Brew seed).

## Running it

```sh
docker compose up -d
make lecture L=07-jsonb-arrays-and-search/07-01-jsonb-access-and-containment T=db-reset
make lecture L=07-jsonb-arrays-and-search/07-01-jsonb-access-and-containment
```

Output:

```
1) Доступ к полям options (-> даёт jsonb с кавычками, ->> — text, #>> — по пути):
КЛИЕНТ  -> 'milk'  ->> 'milk'  size  shots  #>> '{extras,0}'
Алиса   "oat"      oat         L     2      cinnamon
Борис   "cow"      cow         M     1      ∅
Карина  "oat"      oat         S     1      honey
Дина    "soy"      soy         L     3      ∅
Егор    ∅          ∅           M     2      ∅

2) options @> '{"milk":"oat"}' — заказы на овсяном молоке:
   Алиса (size L)
   Карина (size S)

3) options @> '{"extras":["honey"]}' — в массиве extras есть honey:
   Карина

4) options ? 'extras' — указан ключ extras (пустой массив тоже считается):
   Алиса
   Карина
   Дина
```

The first table shows the key contrast: `->` returned `"oat"` in quotes (it's `jsonb`), `->>` returned plain `oat` (it's `text`), and `#>>` extracted the zeroth array element. Egor has no `milk` key → both operators returned `NULL` (we substituted `∅`). Containment `@>` matched by a flat pair (oat milk) and by an array element (honey), while `?` caught Dina with an empty `extras` that `@>` would never have seen.

## The fence

`jsonb` tempts you to dump everything into one column — and immediately punishes you for it. The line is simple:

- A field you regularly filter, count, or join on is a **column**, not a key inside `jsonb`: it's checked by `CHECK` and `NOT NULL`, carries an ordinary B-tree, and its query plan is predictable.
- A containment search without GIN is a `Seq Scan` over the whole table (see 06-05); an index under `@>` is mandatory on any sizable table.
- A `?`-filter on a missing key easily produces three-valued-logic surprises — fully covered by 03-06.
- The rule: `jsonb` is for the genuinely shapeless and sparse; anything your application logic leans on goes into columns.

In production the document size isn't free either — which is exactly the next unit.

## Takeaways

- `->` extracts a value as `jsonb` (`"oat"` in quotes), `->>` as `text` (`oat`); `#>`/`#>>` do the same by path (`'{extras,0}'`).
- `@>` (containment) is "the document contains a key-value pair," and works both for flat fields and inside arrays; it's exactly what GIN speeds up (06-05).
- `?` is "does a top-level key exist" (catches even an empty array); `?|`/`?&` are for a list of keys.
- A missing key is `NULL`: wrap it in `coalesce` to get a definite type and behavior.

Next up — **07-02 "When NOT to use jsonb"**: flexibility has a physical price (write amplification) and a semantic one (no per-field constraints) — we'll look at both bills and decide where `jsonb` fits and where it's deferred pain.
