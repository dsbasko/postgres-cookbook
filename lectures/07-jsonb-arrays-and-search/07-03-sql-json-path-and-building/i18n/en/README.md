# 07-03 — SQL/JSON path and building

Brew gained drink recipes: each has an array of ingredients with gram weights, nested inside `jsonb`. The barista trainer wants answers to questions like "what are the latte's ingredients?", "what weighs more than 100 grams?", "does the drink contain milk?". Writing this with the `->`/`#>` chains from 07-01 is clumsy: extract the array, unfold it, filter it, fold it back. For nested documents Postgres has a better language — **jsonpath**.

The goal of this unit is to learn to extract data from nested `jsonb` with path expressions (`jsonb_path_query_*`, the `@?`/`@@` predicates) and to build `jsonb` back (`jsonb_set`, `jsonb_build_object`, `jsonb_agg`). Along the way we note a version subtlety: `JSON_TABLE` (unfold `jsonb` into a relational table right in `FROM`) is **PG17**, not PG18; we don't need it here, and the `jsonb_path_*` and building functions have existed since PG12.

## jsonpath: a path as a mini-language

jsonpath is the standard addressing language inside JSON. `$` is the document root, `.name` a field, `[*]` all array elements, `[0]` a specific one, and `? (@.grams > 100)` is a **filter**: keep only elements where the condition is true (`@` is the current element). So `$.ingredients[*] ? (@.grams > 100).name` reads as "names of ingredients with more than 100 grams." The path is applied by `jsonb_path_query_array` (collect all matches into one `jsonb` array) and `jsonb_path_query_first` (take the first). One compact path replaces the manual array unpacking via `->`.

## Path predicates: @? and @@

Sometimes you don't need values — you need a yes/no answer. There are two operators for that. `@?` is "does at least one path match exist": `recipe @? '$.ingredients[*] ? (@.name == "milk")'` is true if milk is among the ingredients. `@@` is "is the condition true as a predicate": `recipe @@ '$.kcal > 100'` is true for high-calorie drinks. The difference is subtle (`@?` is about a node existing, `@@` about a boolean check), but both return `boolean` and both can lean on a GIN index (`jsonb_path_ops`) on large tables.

## Building: jsonb_set, build_object, jsonb_agg

The reverse task is to assemble `jsonb`. `jsonb_set(doc, '{path}', value)` patches a field precisely and returns a **new** document (the stored row is unchanged — it's a pure function; the same "edit = rebuild the value" that causes the write amplification in 07-02). `jsonb_build_object('a', x, 'b', y)` builds an object from key-value pairs, and the aggregate `jsonb_agg(... ORDER BY ...)` folds result rows into one `jsonb` array. Together they assemble an API response right in SQL: for example, the whole menu from the canon `drinks` as one document.

## What our code shows

A lab table `drink_recipe_lab` (recipes with a nested ingredient array) for paths, and building over the canon `drinks`:

```sql
SELECT jsonb_path_query_array(recipe, '$.ingredients[*].name'),                    -- all names
       jsonb_path_query_array(recipe, '$.ingredients[*] ? (@.grams > 100).name'),  -- heavy ones
       jsonb_path_query_first(recipe, '$.ingredients[0].name')                     -- first
FROM drink_recipe_lab WHERE id = 1;                                                -- PathQueries

SELECT name, recipe @? '$.ingredients[*] ? (@.name == "milk")', recipe @@ '$.kcal > 100'
FROM drink_recipe_lab;                                                             -- PathPredicates

SELECT jsonb_set(recipe, '{kcal}', '130') ->> 'kcal' FROM drink_recipe_lab WHERE id = 1;  -- SetField

SELECT jsonb_agg(jsonb_build_object('sku', sku, 'price_cents', base_price) ORDER BY id)
FROM drinks;                                                                       -- BuildMenu
```

sqlc doesn't know the signatures of `jsonb_path_*`/`@?`/`@@` from the catalog and types their result as `interface{}`; pgx returns concrete `string`/`bool` into them, so the demo prints via `%v`. The unit adds its own table to the schema → `make db-reset` applies it via `brew.Apply`.

## Running it

```sh
docker compose up -d
make lecture L=07-jsonb-arrays-and-search/07-03-sql-json-path-and-building T=db-reset
make lecture L=07-jsonb-arrays-and-search/07-03-sql-json-path-and-building
```

Output:

```
1) jsonpath по рецепту «Латте» ($.ingredients[*], фильтр ? (@.grams > 100)):
   все ингредиенты   $.ingredients[*].name              = ["espresso", "milk"]
   тяжёлые (>100 г)   ... ? (@.grams > 100).name         = ["milk"]
   первый             $.ingredients[0].name (first)      = "espresso"

2) предикаты пути @? и @@ по всем рецептам:
НАПИТОК   @? есть milk  @@ kcal > 100
Латте     true          true
Эспрессо  false         false
Колд брю  false         false

3) jsonb_set(recipe, '{kcal}', '130') — правка возвращает новый документ:
   kcal до = 190, после = 130

4) jsonb_agg(jsonb_build_object(...)) — меню канона drinks одним документом:
   [{"sku": "ESP-01", "price_cents": 300}, {"sku": "CAP-01", "price_cents": 450}, {"sku": "LAT-01", "price_cents": 480}, {"sku": "CLD-01", "price_cents": 520}, {"sku": "TEA-01", "price_cents": 250}]
```

The path filter `? (@.grams > 100)` kept only `milk` (220 g) of the latte's ingredients, `@?` found milk in the latte alone, and `@@` flagged the same one as high-calorie. `jsonb_set` returned `kcal=130` without touching the stored `190`. And `jsonb_agg` assembled all five canon drinks into one document — note that the keys come out in normalized order (`sku` before `price_cents`): `jsonb` stores keys sorted.

## The fence

jsonpath is powerful — and so it tempts you to keep in `jsonb` what has long been asking for columns. If you regularly filter by `$.kcal > 100` or `? (@.name == "milk")`, that's a signal: the data is structured, and it belongs in normal columns with an ordinary B-tree (07-02). For `jsonb` itself, remember the index: `@?`/`@@`/`@>` on a large table without GIN is a `Seq Scan` (06-05). Building (`jsonb_agg`) right in SQL is handy for an API response, but don't overdo it: heavy aggregation of a whole catalog into one document is better cached than rebuilt on every request — in production that's load on the database CPU. And `JSON_TABLE` is PG17: if you read someone else's code using it, don't attribute it to the "18 novelties."

## Takeaways

- jsonpath: `$` root, `.field`, `[*]`/`[0]`, the filter `? (@.x > N)`; applied by `jsonb_path_query_array`/`_first`.
- The predicates `@?` ("a path match exists") and `@@` ("the condition is true") return `boolean` and lean on GIN.
- Building: `jsonb_set` (precise patch → new document), `jsonb_build_object` (object from pairs), `jsonb_agg(... ORDER BY ...)` (rows → array).
- `JSON_TABLE` is PG17, not PG18; `jsonb_path_*` and the building functions exist since PG12.

Next up — **07-04 "Arrays vs a junction table"**: we return from documents to lists and compare two ways to store "many values" — a native `text[]` (with `@>`/`= ANY` and GIN) and the classic normalization into a junction table — so the choice is a deliberate one.
