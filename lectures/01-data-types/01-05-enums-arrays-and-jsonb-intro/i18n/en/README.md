# 01-05 — enums, arrays, and a jsonb intro

Brew's menu gained drink sizes (S/M/L), the blog's articles gained tags, and marketing wanted to stash "arbitrary options" into an order, like "oat milk, +1 shot." Three different tasks — and three different "container" types in Postgres: `enum` for a fixed set of sizes, an array for tags, `jsonb` for flexible options. Each is handy exactly in its niche, and each is easy to apply in the wrong place.

The goal of this unit is to meet the three containers and develop a feel for when each fits. This is an **introduction**: a deep dive into `jsonb`, GIN indexes, and full-text search is coming in module 07; here it's basic operators and the "when to normalize, when not to" intuition.

## enum: an ordered finite set

An `enum` is a type with a fixed list of values (`small`, `medium`, `large`). Its strength is not only the constraint ("you can't store `xl` here, there's no such value") but also the **order**: values are ordered by how they're declared, not alphabetically. So `'small'::drink_size < 'large'::drink_size` → `true` (small is declared first), even though alphabetically `large` is smaller. That's handy for sorting and comparisons on a "scale." The price is inflexibility: you can add a value (`ALTER TYPE ... ADD VALUE`), but removing or reordering is painful.

## Arrays: text[] and the @> operator

An array (`text[]`, `int[]`, …) stores a list of same-typed values in one column. In the base schema, an article's tags live as the string `'coffee,basics'` (that's how kafka-cookbook has it — byte-compatibility), but `string_to_array(tags, ',')` unfolds it into `text[]`, and in Go that's `[]string`. The basic operator is `@>` ("array contains"): `tags @> ARRAY['coffee']` finds articles with the `coffee` tag. At scale this search is sped up by a GIN index (module 06/07). An array is good when the values are simple, few, and don't need their own attributes; the moment a tag needs its own fields (color, counter), it's time for a separate junction table.

## jsonb: flexibility with an asterisk

`jsonb` stores a JSON structure in a parsed binary form — it works with the operators `->` (extract as `jsonb`), `->>` (extract as `text`), and `?` (does the key exist; in SQL it's `jsonb_exists`). The key nuance up front: `->>` returns the value as text (`oat`), while `->` keeps it `jsonb` — with surrounding quotes (`"oat"`). `jsonb` is irreplaceable for genuinely flexible, sparse data. But this is an intro, and here the warning matters more: `jsonb` is not an excuse to skip normalization. Fields you filter, count, and join on should almost always be columns; `jsonb` is for what is shapeless by nature. The details and pitfalls are in module 07.

## Which container to pick

| Container | Stores | Access | When to pick | When to normalize |
|---|---|---|---|---|
| `enum` | a fixed ordered set | comparison on a scale (`<`, `>`) | stable scales (S/M/L, statuses) | a frequently changing reference → table with an FK |
| array (`text[]`) | a list of same-typed simple values | `@>` "contains" (sped up by GIN — 06/07) | simple tags/labels with no attributes | a tag needs its own fields → junction table |
| `jsonb` | a sparse/shapeless structure | `->`, `->>`, `?` | genuinely flexible, sparse data | you filter / count / join → a column |

The right-hand column is the boundary: a container fits while the data is simple and handled "whole"; the moment you need to filter, count, or join on individual fields, it's time for columns and tables.

## What our code shows

A dedicated `enum` type (in `schema.sql`) and three demonstrations. The enum order on literals; arrays and `jsonb` on the base schema and literals:

```sql
SELECT ('small'::drink_size < 'large'::drink_size) AS small_lt_large,   -- EnumOrder
       ('large'::drink_size < 'small'::drink_size) AS large_lt_small;

SELECT id, title, string_to_array(tags, ',')::text[] AS tag_list        -- TagsAsArray
FROM articles ORDER BY id;

SELECT coalesce('{"size":"L","milk":"oat","shots":2}'::jsonb ->> 'milk', '')        AS milk_text,
       coalesce(('{"size":"L","milk":"oat","shots":2}'::jsonb -> 'milk')::text, '')  AS milk_json,
       jsonb_exists('{"size":"L","milk":"oat","shots":2}'::jsonb, 'milk')            AS has_milk;
```

sqlc types `tag_list` as `[]string`; the `jsonb`-operator results arrive as strings. Like 01-04, this unit adds its own object to the schema (the `drink_size` type), so `make db-reset` applies it via `brew.Apply` (base schema → unit DDL → seed).

## Running it

```sh
docker compose up -d
make lecture L=01-data-types/01-05-enums-arrays-and-jsonb-intro T=db-reset
make lecture L=01-data-types/01-05-enums-arrays-and-jsonb-intro
```

Output:

```
1) enum drink_size = ('small','medium','large') — порядок по объявлению:
   'small' < 'large' = true   (по алфавиту было бы наоборот)
   'large' < 'small' = false

2) string_to_array(tags) → text[] (в Go это []string):
ID  ЗАГОЛОВОК                   TAGS ([]string)
1   Почему эспрессо — это база  [coffee basics]
2   Гайд по колд брю            [coffee cold-brew]
   tags @> ARRAY['coffee'] → статей с тегом coffee: 2

3) jsonb '{"size":"L","milk":"oat","shots":2}' — базовые операторы:
   ->> 'milk'  = oat        (text: без кавычек)
   ->  'milk'  = "oat"      (jsonb: с кавычками)
   ->> 'shots' = 2          (text '2')
   ? 'milk'    = true       (есть ли ключ)
```

(The demo prints in Russian.) The `enum` is ordered by declaration (`small < large`), not alphabetically. The tags unfolded into `[]string`, and `@>` found both articles with `coffee`. And `jsonb` showed the key contrast: `->>` gives plain text `oat`, `->` gives `jsonb` `"oat"` with quotes.

## The fence

Three containers, three temptations:

- **`enum` tempts you to add values "on the fly".** In production that's an `ALTER TYPE` shipped as a migration, and you can't remove a value at all; for frequently changing reference data a separate table with an FK is better.
- **An array lures you into stashing entities with their own attributes.** Then `@>` search and counts become painful — normalize into a junction table.
- **`jsonb` is the most dangerous.** It lets you skip designing a schema entirely, and the application quickly accretes "fields inside json" that can't be checked with `CHECK`, indexed without tricks, or joined.

The rule is simple: **what you filter / count / join on is a column; `jsonb` is only for the truly shapeless**. When and how to do it right — module 07.

## Takeaways

- `enum` is ordered by value declaration, not alphabetically; good for fixed scales, but inflexible to change.
- Arrays (`text[]`) + the `@>` ("contains") operator are handy for simple lists; in the base schema tags are a string, `string_to_array` gives `text[]` → Go `[]string`.
- `jsonb`: `->>` extracts `text`, `->` extracts `jsonb` (with quotes), `jsonb_exists`/`?` tests key presence. This is an intro; the depth is module 07.
- A container is not a substitute for normalization: keep what you filter/count/join on as columns.

Next up — module **02 "Schema, DDL, and constraints"**: how to assemble a reliable schema from the right types — `IDENTITY` vs `serial`, `NOT NULL`, primary and foreign keys, `UNIQUE`/`CHECK`, generated columns, and a migration mindset.
