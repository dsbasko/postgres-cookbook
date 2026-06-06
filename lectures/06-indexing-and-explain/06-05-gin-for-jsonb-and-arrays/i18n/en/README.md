# 06-05 — GIN for jsonb and arrays

In Brew's admin panel each drink had a "specs" block — a flexible `jsonb` (`{"milk": "oat", "size": "L"}`) and an array of tags (`{coffee, seasonal}`). While the menu was small, the filter "show everything with a gift" (`attrs @> '{"gift": true}'`) worked fine. At two hundred thousand items it stalled: every query a full `Seq Scan`. The developer tried the familiar `CREATE INDEX ON drink_specs (attrs)` — and `EXPLAIN` still showed a `Seq Scan`. The B-tree we built in every previous unit is useless here in principle: it indexes the value **as a whole**, but we need to look **inside** — does the jsonb have such a key, does the array contain such an element.

The goal of this unit is to understand why "searching inside" jsonb and arrays needs a different index type, and to meet **GIN** (Generalized Inverted Index), which is built exactly for that. This is the last brick before module 07, where jsonb, arrays, and full-text search are covered in detail — here we only put the right index under them.

## Why a B-tree is no good and GIN is

A B-tree stores the sorted **whole values** of a column. It answers "equals/greater/less/starts with" — questions about the value as one unit. But `attrs @> '{"gift": true}'` doesn't ask "is the whole jsonb equal to this," it asks "does it **contain** this key-value pair." A whole-value comparison won't do: `attrs` may have any number of other keys. Same with the array: `tags @> ARRAY['limited']` is "does the element **belong** to the set," not "is the whole array equal."

**GIN** is built the other way around — it's an *inverted* index. It decomposes each value into its parts (jsonb keys and values, array elements, later text words) and, for each part, keeps a list of the rows where it occurs. Asking "who has `gift: true`" means taking a ready-made list from the index, like the subject index at the back of a book. The containment operator `@>` (for both jsonb and arrays) can read that index — hence a `Bitmap Index Scan` on GIN instead of a `Seq Scan`.

## Containment and opclasses

GIN over jsonb serves a whole family of operators: `@>` (contains), `?` (does a key exist), `?|`/`?&` (any/all keys from a list). GIN for jsonb has two **opclasses**:

- `jsonb_ops` (the default) — indexes both keys and values; supports all the operators above.
- `jsonb_path_ops` — indexes only "key→value" paths; **smaller and faster on `@>`**, but can't do `?` (key-existence check). Pick it when your code has only containment queries:

```sql
CREATE INDEX ... USING gin (attrs jsonb_path_ops);
```

For arrays, GIN serves `@>` (contains all), `<@` (is contained in), `&&` (overlaps), and `= ANY`.

> ⚠️ GIN reads great but is **more expensive to write**: inserting one row touches the index once per element/key inside the value. So for workloads with frequent jsonb updates, GIN has a `fastupdate` parameter (deferred batch insertion) — but that's tuning your DBA holds. The developer takeaway: GIN is for "read by content a lot, write moderately."

## What our code shows

`demo.sql` builds `drink_specs_lab` (200,000 rows: `jsonb attrs` + `text[] tags`, the rare flags `gift`/`limited` on 0.5%) and explains two containment queries twice — before and after GIN:

```sql
-- without an index both run a Seq Scan
SELECT id FROM drink_specs_lab WHERE attrs @> '{"gift": true}';   -- jsonb
SELECT id FROM drink_specs_lab WHERE tags  @> ARRAY['limited'];   -- array

CREATE INDEX drink_specs_lab_attrs_gin ON drink_specs_lab USING gin (attrs);
CREATE INDEX drink_specs_lab_tags_gin  ON drink_specs_lab USING gin (tags);

-- with GIN both run a Bitmap Index Scan on GIN
```

## Running it

```sh
docker compose up -d
make lecture L=06-indexing-and-explain/06-05-gin-for-jsonb-and-arrays
```

Output:

```
== 1) jsonb @> БЕЗ индекса — Seq Scan (B-tree тут не помощник) ==
                        QUERY PLAN                         
-----------------------------------------------------------
 Seq Scan on drink_specs_lab (actual rows=1000.00 loops=1)
   Filter: (attrs @> '{"gift": true}'::jsonb)
   Rows Removed by Filter: 199000


== 2) массив @> БЕЗ индекса — тоже Seq Scan ==
                        QUERY PLAN                         
-----------------------------------------------------------
 Seq Scan on drink_specs_lab (actual rows=1000.00 loops=1)
   Filter: (tags @> '{limited}'::text[])
   Rows Removed by Filter: 199000


== создаём два GIN-индекса: по attrs (jsonb) и по tags (массив) ==

== 3) jsonb @> С GIN — Bitmap Index Scan по GIN ==
                                     QUERY PLAN                                     
------------------------------------------------------------------------------------
 Bitmap Heap Scan on drink_specs_lab (actual rows=1000.00 loops=1)
   Recheck Cond: (attrs @> '{"gift": true}'::jsonb)
   Heap Blocks: exact=1000
   ->  Bitmap Index Scan on drink_specs_lab_attrs_gin (actual rows=1000.00 loops=1)
         Index Cond: (attrs @> '{"gift": true}'::jsonb)
         Index Searches: 1


== 4) массив @> С GIN — Bitmap Index Scan по GIN ==
                                    QUERY PLAN                                     
-----------------------------------------------------------------------------------
 Bitmap Heap Scan on drink_specs_lab (actual rows=1000.00 loops=1)
   Recheck Cond: (tags @> '{limited}'::text[])
   Heap Blocks: exact=1000
   ->  Bitmap Index Scan on drink_specs_lab_tags_gin (actual rows=1000.00 loops=1)
         Index Cond: (tags @> '{limited}'::text[])
         Index Searches: 1
```

(The demo prints in Russian.) Without an index, both `@>` run a `Seq Scan` and discard 199,000 rows each — you can't put a B-tree here, it can't look inside the value. After two `CREATE INDEX ... USING gin (...)`, both queries take GIN via a `Bitmap Index Scan`: the index returned the list of matching rows by content. `Recheck Cond` is a normal part of a bitmap plan: after the index, Postgres re-checks the condition on the rows themselves.

## The fence

What we simplified. First, GIN is about **content search** (`@>`, `?`, `&&`); if you need an equality search on a specific scalar in a jsonb field (`attrs->>'milk' = 'oat'`), that's again an ordinary B-tree, but on an **expression** (`(attrs->>'milk')`, like in 06-03), not GIN. You pick the index type for the query shape, not the column type. Second, we took a selective flag (0.5% of rows) — there GIN clearly wins; on a flag present in half the rows the planner will deliberately fall back to a `Seq Scan` (the same selectivity lesson as 06-01). Third, GIN has its own write and maintenance cost (`fastupdate`, bloat, rebuild) — that's load tuning, DBA territory. And fourth, jsonb is a powerful but not free modeling tool: when a flexible jsonb schema is justified versus when a field belongs in a normal column with a constraint is a separate conversation in 07-02. Here the developer takeaway is one: **for containment search over jsonb/arrays, use GIN and check the plan in `EXPLAIN`**.

## Takeaways

- A B-tree indexes the whole value (=, <, >, prefix); it can't "search inside" jsonb/arrays — that's a `Seq Scan`.
- GIN is an inverted index: it decomposes a value into keys/elements and serves containment `@>` (and for jsonb also `?`, `?|`, `?&`).
- `CREATE INDEX ... USING gin (col)`; for arrays `@>`/`<@`/`&&`, for jsonb — the operator family.
- The `jsonb_path_ops` opclass is smaller and faster on `@>` but without `?`; pick it for pure containment queries.
- GIN is expensive to write (touches the index once per element) — it's for "read by content a lot, write moderately."

Next up — **06-06 "CREATE INDEX CONCURRENTLY"**: how to build an index on a hot table without blocking writes for the duration of the build.
