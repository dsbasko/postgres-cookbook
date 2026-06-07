# 07-04 — arrays vs a junction table

Brew decides to attach tags to drinks: `coffee`, `hot`, `limited`, `classic`. The eternal modeling question arises: stash the tags as an array right on the drink row (`tags text[]`) — or set up a separate junction table, "one row per (drink, tag)"? Both are first-class in Postgres: arrays have their own operators and a GIN index, and a junction is classic normalization. The choice isn't "right/wrong," it's about which questions you'll be asking.

The goal of this unit is to feel both models on the same data: array operators (`@>` "contains," `= ANY` "belongs") versus ordinary joins and `GROUP BY` on a junction. We'll see where they're equivalent and where the cost of a question diverges sharply.

## Array: `@>` and `= ANY`

An array (`text[]`) stores a list right in the column — one row per drink, tags at hand without a join. Two working operators: `@>` ("the array contains a subarray") — `tags @> ARRAY['coffee']` finds drinks with the `coffee` tag; and `= ANY` ("a value belongs to the array") — `'cold' = ANY(tags)` checks a single tag. Both are sped up by a GIN index on the column (`USING gin (tags)`, see 06-05): on a large table it's a `Bitmap Index Scan`, not a `Seq Scan`. An array is ideal when tags are simple labels: few, without their own attributes, and you mostly ask "does tag X exist."

## Junction: normalization into rows

The junction table `drink_tags(drink_sku, tag)` stores the same thing as rows — one per pair. The same query "drinks with the coffee tag" is a plain `WHERE tag = 'coffee'`, and the result **matches** the `@>` on the array: by data the models are equivalent. But the junction unlocks what the array can't: a composite `PRIMARY KEY (drink_sku, tag)` guarantees pair uniqueness; you can put a foreign key on `tag` to a tag dictionary (and the database won't let a typo in); a tag can easily gain its own columns (color, priority); and "how many drinks per tag" is a trivial `GROUP BY tag`. On an array the same count requires unfolding `unnest(tags)` and only then grouping — an extra step.

## A bridge between the models

The models aren't enemies — there's a bridge between them. `array_agg(tag ORDER BY tag)` folds junction rows back into an array (as in an API response), and `unnest(tags)` unfolds an array into rows (to count or join). So the normal play is to **store normalized (junction) and serve as an array**: analytic queries run over rows, while the client gets a compact `text[]`/`json` on the outside.

## What our code shows

The same tags in two tables: `drink_tags_arr` (array + GIN) and `drink_tags` (junction). Five queries:

```sql
SELECT drink_sku FROM drink_tags_arr WHERE tags @> ARRAY['coffee'];   -- ArrayTaggedCoffee
SELECT drink_sku FROM drink_tags_arr WHERE $1::text = ANY(tags);      -- ArrayHasTag ('cold')
SELECT drink_sku FROM drink_tags     WHERE tag = 'coffee';            -- JunctionTaggedCoffee (same answer)
SELECT tag, count(*) FROM drink_tags GROUP BY tag ORDER BY count(*) DESC, tag;          -- TagPopularity
SELECT drink_sku, array_agg(tag ORDER BY tag) FROM drink_tags GROUP BY drink_sku;       -- TagsFromJunction
```

sqlc types a `text[]` element as `string` (the `$1` parameter for `= ANY`), `array_agg(...)::text[]` as `[]string`, and `count(*)::bigint` as `int64`. The unit adds its own tables → `make db-reset` applies them via `brew.Apply`.

## Running it

```sh
docker compose up -d
make lecture L=07-jsonb-arrays-and-search/07-04-arrays-vs-junction-table T=db-reset
make lecture L=07-jsonb-arrays-and-search/07-04-arrays-vs-junction-table
```

Output:

```
1) Массив text[] — операторы поиска:
   tags @> ARRAY['coffee']  → CAP-01, CLD-01, ESP-01
   'cold' = ANY(tags)       → CLD-01

2) Junction — те же напитки с тегом coffee (WHERE tag = 'coffee'):
   → CAP-01, CLD-01, ESP-01   (совпало с @> по массиву)

3) Частота тегов (GROUP BY на junction — тривиально):
ТЕГ      НАПИТКОВ
coffee   3
hot      3
classic  1
cold     1
limited  1
milk     1
tea      1

4) array_agg(tag ORDER BY tag) — junction свёрнут обратно в массив:
   CAP-01 = {coffee, hot, milk}
   CLD-01 = {coffee, cold, limited}
   ESP-01 = {classic, coffee, hot}
   TEA-01 = {hot, tea}
```

The `@>` on the array and `WHERE tag = 'coffee'` on the junction gave the same list (CAP/CLD/ESP) — the data is equivalent. But "tag popularity" on the junction is a single `GROUP BY` (coffee and hot at 3, the rest at 1), whereas on the array you'd first need `unnest`. `array_agg` showed the reverse path: the junction folded into the same arrays that live in `drink_tags_arr`.

## The fence

An array tempts you with compactness — and punishes you when a tag stops being just a label. The moment a tag needs its own attributes (when attached, by whom, with what weight), needs a dictionary with typo checking (FK), or you regularly count/join on tags — that's a signal for a junction table. The array won't give you a foreign key on an element or uniqueness within (nothing forbids a duplicate in `{coffee, coffee}`), and analytics over it always goes through `unnest`. The opposite extreme can also hurt: a junction for simple immutable labels with the single question "does the tag exist" is a needless join out of nowhere. A practical rule: **junction by default for anything with relationships and attributes; an array for short simple lists where the only operation is `@>`/`= ANY`**. In production an "array we now join and count on" usually migrates to a junction — and a DBA will ask you to do it before it bloats.

## Takeaways

- Array `text[]`: the operators `@>` ("contains") and `= ANY` ("belongs"), sped up by GIN; compact, no join.
- Junction `(entity, value)`: normalization into rows — FK to a dictionary, pair uniqueness (PK), per-tag columns, frequency via `GROUP BY`.
- The models are equivalent by data ("drinks with coffee" matched); what diverges is the cost of different questions (frequency is trivial on a junction).
- The bridge: `array_agg` (rows → array, for serving), `unnest` (array → rows, for analytics); the "store normalized, serve as an array" play.

Next up — **07-05 "Full-text search"**: from point tags and containment we move to searching by words inside text — `tsvector`/`tsquery`, `ts_rank` ranking, `setweight` weights, and a generated `tsvector` column with GIN over Brew's blog and menu.
