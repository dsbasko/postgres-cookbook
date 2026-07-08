# 08-04 — Recursive CTEs: tree traversal and cycle guards

Brew opened the menu admin panel and realised the categories had long since become a tree: "Drinks" splits into "Coffee" and "Tea", "Coffee" splits into "Espresso drinks" and "Filter", and "Espresso drinks" is already "Cappuccino" and "Latte". In the table this is a single `parent_id` column pointing back at its own `id`. Marketing asked to export the menu "the way it looks on the board" — indented by nesting level, with the full path from the root — to drop into a banner. A plain `JOIN` is helpless here: nobody knows the depth of the tree in advance, and writing eight self-joins "just in case" is not an engineering solution.

An hour later a second incident landed. The logistics lead built a "where to ship the batch next" graph: from the warehouse to the workshop, from the workshop to the café. Someone mistakenly entered a row saying the leftovers ship from the café back to the warehouse — that closed a ring, `Warehouse → Workshop → Café → Warehouse`. The nightly route export, which usually finishes in a second, went round and round this ring forever:

> **Nodyr (in chat, 23:50):** The route export is hanging. Usually a second.
>
> **Pavel (in chat):** i see it. the query is spinning in a circle. killed it.

Pavel finds the stuck query in `pg_stat_activity`, among the server's active queries, and kills it by hand: without a guard, recursion over this ring never stops on its own — it just keeps walking in circles until the server hits a limit. Both incidents are about the same construct — the recursive `CTE` we promised back in `04-06`, when we first started talking about trees in the relational model. Time to deliver it.

## Why this unit is on psql, not sqlc

The whole course keeps `sqlc` in the leading role by default. But here we deliberately drop the tool, because the lesson is exactly about a feature `sqlc` cannot read. Cycle protection in the `SQL` standard is expressed by the `CYCLE` clause: it appends to the recursion result two "virtual" columns, `is_cycle` and `path`, that exist in no table of the schema. `sqlc` v1.30.0 analyses the query statically, against the `DDL`, fails to find those columns, and crashes with `column "is_cycle" does not exist`. Reshaping the lesson to fit the tool would mean throwing out the lesson's core. So we pick the feature over the tool and drive the demo with a plain `psql` script, `demo.sql`. It is the same escape-hatch logic as in `02-05` and `03-05`: when a construct runs into the limits of the static analyser, the construct wins.

## WITH RECURSIVE: anchor plus step

A recursive `CTE` always consists of two parts glued together by `UNION ALL`. The first is the **anchor**: the starting rows the traversal begins from. For a tree these are the roots — categories with no parent (`parent_id IS NULL`). The second is the **recursive step**: it takes the rows already found and attaches their children. Postgres repeats the step again and again until it stops adding new rows — on a tree this naturally happens once the traversal reaches the leaves.

What matters is that at each level we don't merely descend, we **accumulate context**. In the anchor we set `depth = 1` and a path consisting of the single current node; in the step we add one to `depth` and append the current node to the parent's path. This way every row carries both its depth and the entire road from the root down to it — exactly what marketing asked for.

Here is the whole tree — those same `parent_id`s laid out by level. The traversal runs top to bottom in exactly this order (pre-order: a parent, then its whole subtree, then the sibling):

```
depth 1   Drinks
depth 2   ├─ Coffee
depth 3   │  ├─ Espresso drinks
depth 4   │  │  ├─ Cappuccino
depth 4   │  │  └─ Latte
depth 3   │  └─ Filter
depth 2   └─ Tea
depth 3      └─ Green
```

The anchor is the root `Drinks` (no parent); each recursive step descends one level, to the children of already found nodes. `idpath` — an array of `id`s along the branch — sets this pre-order in the output.

## What our code shows

The first part of `demo.sql` traverses the category tree. The anchor selects the roots and initialises three accumulators: `depth`, an array of identifiers `idpath`, and a string path `namepath`. The recursive step joins the table to itself on `c.parent_id = t.id` and continues all three accumulators:

```sql
WITH RECURSIVE tree AS (
    -- якорь: корни (нет родителя), глубина 1, путь = [свой id]
    SELECT id, parent_id, name, 1 AS depth,
           ARRAY[id]   AS idpath,
           name::text  AS namepath
    FROM category_tree_lab
    WHERE parent_id IS NULL
    UNION ALL
    -- рекурсивный шаг: дети уже найденных узлов, глубина +1, путь дополняем
    SELECT c.id, c.parent_id, c.name, t.depth + 1,
           t.idpath   || c.id,
           t.namepath || ' > ' || c.name
    FROM category_tree_lab c
    JOIN tree t ON c.parent_id = t.id
)
SELECT depth,
       repeat('  ', depth - 1) || name AS category,
       namepath                        AS path
FROM tree
ORDER BY idpath;   -- массив id → детерминированный pre-order, без зависимости от collation
```

The `ORDER BY idpath` deserves a separate word. A recursive `CTE` itself promises nothing about row order — without an explicit sort the output could shuffle from run to run. We want the familiar pre-order: a parent, then its whole subtree, then the next sibling. It is tempting to sort by `namepath` — the string path with names — but then the order of "Coffee" and "Tea" would start depending on the database `collation`: in one locale Cyrillic sorts one way, in another differently, and the output drifts. So we sort by `idpath` — an **array** of numeric `id`s. Arrays in Postgres compare element by element, number against number, untouched by any collation. The result is a stable, reproducible traversal on any installation.

The second part is the same machinery aimed at a cyclic route graph. Here we care not about the path but about the very fact of looping, and that is caught by the `SQL`-standard `CYCLE` clause (Postgres has understood it since version 14):

```sql
WITH RECURSIVE walk AS (
    SELECT id, next_id, name, 1 AS step
    FROM cyclic_routes_lab
    WHERE id = 1
    UNION ALL
    SELECT r.id, r.next_id, r.name, w.step + 1
    FROM cyclic_routes_lab r
    JOIN walk w ON r.id = w.next_id
) CYCLE id SET is_cycle USING path   -- помечаем повтор id и кладём пройденный путь в path
SELECT step, id, name, is_cycle, path
FROM walk
ORDER BY step;
```

It reads like this: "watch for a repeat of the `id` value; as soon as the recursion comes back to an already visited `id`, set the `is_cycle` flag to `true` and keep in `path` the list of all visited nodes". Postgres materialises the `is_cycle` and `path` columns itself — they are not in `cyclic_routes_lab`, and it is precisely their absence from the schema that trips `sqlc`. The key part is the behaviour on a repeat: having found the node already in `path`, Postgres marks the row and does **not** take the next recursive step from it. The ring is broken, the recursion ends on its own.

## Running it

```sh
docker compose up -d
make lecture L=08-analytical-and-lateral/08-04-recursive-ctes
```

`T=run` is the default target: it runs `demo.sql` through `psql`. From inside the unit directory, `make run` is enough. The output is deterministic — it repeats verbatim on every run.

(The demo prints in Russian.)

```
1) Обход дерева категорий сверху вниз (WITH RECURSIVE):
 depth |       category       |                     path                     
-------+----------------------+----------------------------------------------
     1 | Напитки              | Напитки
     2 |   Кофе               | Напитки > Кофе
     3 |     Эспрессо-напитки | Напитки > Кофе > Эспрессо-напитки
     4 |       Капучино       | Напитки > Кофе > Эспрессо-напитки > Капучино
     4 |       Латте          | Напитки > Кофе > Эспрессо-напитки > Латте
     3 |     Фильтр           | Напитки > Кофе > Фильтр
     2 |   Чай                | Напитки > Чай
     3 |     Зелёный          | Напитки > Чай > Зелёный


2) Тот же обход по зациклённому графу, но с CYCLE — рекурсия сама тормозит:
 step | id | name  | is_cycle |       path        
------+----+-------+----------+-------------------
    1 |  1 | Склад | f        | {(1)}
    2 |  2 | Цех   | f        | {(1),(2)}
    3 |  3 | Кафе  | f        | {(1),(2),(3)}
    4 |  1 | Склад | t        | {(1),(2),(3),(1)}
```

In the first block the indentation `repeat('  ', depth - 1)` draws the tree, and `path` shows the full road from the root — exactly what goes into the banner. In the second, the traversal walked `Warehouse → Workshop → Café` and on the fourth step came back to `id = 1`. Postgres saw that `id` in the accumulated `path`, set `is_cycle = t`, and put the closed ring `{(1),(2),(3),(1)}` into `path`. On that row the traversal stops — instead of circling through warehouse, workshop and café forever.

## The fence

- Without an explicit guard, recursion over a cyclic graph never terminates — and that is not a theoretical risk but the very hung query from the incident. Before Postgres 14 there was no `CYCLE` clause, and cycles were caught by hand: you carried an array of visited `id`s through the recursion and added to the step a condition like `NOT c.id = ANY(path)`. It worked, but it was verbose and easy to break when copied around. `CYCLE` says the same thing in one line; if you spot the old manual variant, now you know what it does.
- Deep recursion costs memory and time: the intermediate result lives in full, and on a large or poorly branching graph the query can eat a noticeable chunk of resources. On such data, in production you set a depth limit (a condition on `depth` in the recursive step) so the query is guaranteed to finish even if an anomaly hides in the graph.
- Trees that are read often and in bulk — catalogue navigation, comment threads, org charts — are usually **denormalised** in production: a materialised path is kept alongside, an `ltree` type is used, a closure table is built. Then "give me the whole subtree" turns into an ordinary indexed `SELECT` with no recursion every time. Recursive traversal is good for one-off exports and admin queries, but as a hot path under load it almost always loses to a precomputed structure.
- Remember the frame: this lesson is about the **expressiveness** of `SQL` — that a tree and a graph fit into a single query — not an invitation to move traversal business logic into the database where it belongs in code.

## What to take away

A recursive `CTE` is an anchor plus a recursive step, glued by `UNION ALL`: the anchor gives the start (the tree's roots), the step attaches the children of already found nodes and repeats while there is anything to add. At each level we accumulate context — `depth` and `path` — and it is this accumulation that turns a flat table with `parent_id` into a tree with indentation and full paths. Deterministic ordering comes from sorting by an **array** of `id`s rather than by a name string: arrays compare element by element and don't depend on the database collation, so the pre-order is stable on any installation.

Loop protection lives in the `SQL`-standard `CYCLE id SET is_cycle USING path` clause: on a repeated `id` it raises a flag and cuts the recursion on that branch, so traversal of a cyclic graph finishes on its own rather than hanging up to the server limit. And the escape hatch here is no whim: `sqlc` does not see the virtual `is_cycle`/`path` columns and fails on static analysis — a lesson about the feature dictates the tool.

We've learned to descend into a tree and a graph in depth. Next, in `08-05`, we turn the axis ninety degrees: `LATERAL` lets us run a separate subquery on the right for each row on the left — "for each shop give its three latest orders" — and that is no longer about going deep, but about a correlated join going wide.
