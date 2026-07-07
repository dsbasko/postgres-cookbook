# 07-04 — arrays vs a junction table

Brew is hanging tags on drinks: `coffee`, `hot`, `limited`, `classic`. At a review Evgeny comes down with his phone — a mockup of the showcase filter on the screen.

> **Evgeny:** A guest wants to filter "coffee only," "seasonal only." Tags: coffee, hot, limited, classic. By Friday.
>
> **Botyr:** An array. `tags text[]` right on the drink row — compact, not a single join: `@>` by tag, done.
>
> **Dmitry:** You can. One question: which queries will we ask? "Does the tag exist" — an array. "How many drinks per tag," "a dictionary with no typos," "a tag's own color" — a junction. Not an argument — a table.

Both are first-class in Postgres: an array has its own operators and a GIN index, a junction is classic normalization.

The goal of this unit is to feel both models on the same data: array operators (`@>` "contains," `= ANY` "belongs") versus ordinary joins and `GROUP BY` on a junction. We'll see where they're equivalent and where the cost of a question diverges sharply.

## Array: `@>` and `= ANY`

An array (`text[]`) stores a list right in the column — one row per drink, tags at hand without a join. Two working operators: `@>` ("the array contains a subarray") — `tags @> ARRAY['coffee']` finds drinks with the `coffee` tag; and `= ANY` ("a value belongs to the array") — `'cold' = ANY(tags)` checks a single tag. Both are sped up by a GIN index on the column (`USING gin (tags)`, see 06-05): on a large table it's a `Bitmap Index Scan`, not a `Seq Scan`. An array is ideal when tags are simple labels: few, without their own attributes, and you mostly ask "does tag X exist."

## Junction: normalization into rows

The junction table `drink_tags(drink_sku, tag)` stores the same thing as rows — one per pair. The same query "drinks with the coffee tag" is a plain `WHERE tag = 'coffee'`, and the result **matches** the `@>` on the array: by data the models are equivalent. But the junction unlocks what the array can't: a composite `PRIMARY KEY (drink_sku, tag)` guarantees pair uniqueness; you can put a foreign key on `tag` to a tag dictionary (and the database won't let a typo in); a tag can easily gain its own columns (color, priority); and "how many drinks per tag" is a trivial `GROUP BY tag`. On an array the same count requires unfolding `unnest(tags)` and only then grouping — an extra step.

## A bridge between the models

The models aren't enemies — there's a bridge between them. `array_agg(tag ORDER BY tag)` folds junction rows back into an array (as in an API response), and `unnest(tags)` unfolds an array into rows (to count or join). So the normal play is to **store normalized (junction) and serve as an array**: analytic queries run over rows, while the client gets a compact `text[]`/`json` on the outside.

## When an array, when a junction

Both models store the same thing; what diverges is the cost of different questions:

| Question / axis | Array `text[]` | Junction `(drink_sku, tag)` |
|---|---|---|
| "does tag X exist" | `tags @> ARRAY['x']`, `'x' = ANY(tags)` | `WHERE tag = 'x'` |
| what speeds it up | GIN on the column (06-05) | B-tree / PK on the pair |
| pair uniqueness | none — `{coffee,coffee}` passes | `PRIMARY KEY (drink_sku, tag)` |
| FK to a tag dictionary | impossible | `tag` → dictionary, typo rejected |
| a tag's own attributes | none | columns in the junction/dictionary |
| tag frequency | `unnest(tags)` + `GROUP BY` | a direct `GROUP BY tag` |
| compact serving | already an array | `array_agg(tag ORDER BY tag)` |
| when to use | a short list of simple labels, only "does it exist" | relationships, attributes, analytics, integrity |

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

An array tempts you with compactness — and punishes you when a tag stops being just a label. The signals for the choice:

- a tag needs its own attributes (when attached, by whom, with what weight), a dictionary with typo checking (FK), or regular counting/joining on tags — go junction;
- an array gives you neither a foreign key on an element nor uniqueness within (`{coffee, coffee}` passes), and analytics over it always goes through `unnest`;
- the opposite extreme also hurts: a junction for simple immutable labels with the single question "does the tag exist" is a needless join out of nowhere.

A practical rule: junction by default for anything with relationships and attributes; an array for short simple lists where the only operation is `@>`/`= ANY`. In production an "array we now join and count on" migrates to a junction — and a DBA will ask you to do it before it bloats.

## Takeaways

- Array `text[]`: the operators `@>` ("contains") and `= ANY` ("belongs"), sped up by GIN; compact, no join.
- Junction `(entity, value)`: normalization into rows — FK to a dictionary, pair uniqueness (PK), per-tag columns, frequency via `GROUP BY`.
- The models are equivalent by data ("drinks with coffee" matched); what diverges is the cost of different questions (frequency is trivial on a junction).
- The bridge: `array_agg` (rows → array, for serving), `unnest` (array → rows, for analytics); the "store normalized, serve as an array" play.

Next up — **07-05 "Full-text search"**: from point tags and containment we move to searching by words inside text — `tsvector`/`tsquery`, `ts_rank` ranking, `setweight` weights, and a generated `tsvector` column with GIN over Brew's blog and menu.
