# 03-02 — SELECT: WHERE / ORDER / LIMIT and keyset pagination

The Brew menu is shown page by page in the app: "20 more drinks," "next page." While there are few pages, everything works. But as soon as the catalog grows and the user scrolls to page 500, a query with `OFFSET 10000` suddenly slows down — even though it returns the same 20 rows. The reason isn't the amount returned but the amount traversed: `OFFSET` makes the server compute and **discard** the first 10000 rows before returning the ones you want.

The goal of this unit is to learn to fetch exactly the rows you need (`WHERE`), in the right order (`ORDER BY`), in the right batch (`LIMIT`), and to page so the cost of a page doesn't depend on its depth. That's keyset pagination (a.k.a. "seek"): instead of "skip N rows" it's "give me the rows after this one."

## WHERE / ORDER / LIMIT — the three pillars of a query

`WHERE` selects rows by a condition (category, price range), `ORDER BY` sets the order, `LIMIT` caps the count. The order of logical steps — filter first, then sort, then truncate — matters for understanding: a `LIMIT` without `ORDER BY` returns "some N rows," and on the next run they may be different, because rows have no guaranteed order without an explicit `ORDER BY`.

For pagination this is critical: the order must be **total**. If you sort only by price and several drinks have the same price, their relative order is undefined — between pages they can "swap," and the same row ends up on two pages or on none. The cure is a tie-break: add a unique column (`id`) to `ORDER BY`. `ORDER BY base_price DESC, id DESC` is a strict, stable order.

## OFFSET: simple, but expensive at depth

`LIMIT n OFFSET k` reads as "skip `k`, return the next `n`." It's the simplest way to paginate, and for the first pages it's excellent. The trouble is that `OFFSET` doesn't "jump" over `k` rows — the server still computes them (applies `WHERE`, sorts) and only then discards them. The cost of a page grows linearly with its number: page 1 is cheap, page 1000 reads a thousand unneeded rows before the twenty you want.

## Keyset: "give me the rows after this one"

Keyset pagination keeps a **cursor** — the sort values of the last row of the current page. The next page is requested not by number but by cursor: "give me the rows that, in `ORDER BY` order, come after `(price, id)` of the last one." A tuple comparison does this in one expression:

```sql
WHERE (base_price, id) < (:after_price, :after_id)
ORDER BY base_price DESC, id DESC
```

`(a, b) < (x, y)` in Postgres is a lexicographic comparison: by `a` first, by `b` on ties. It exactly mirrors `ORDER BY base_price DESC, id DESC`, so "rows after the cursor" coincides with the sort order. With an index on `(base_price, id)` the server jumps straight to the right place (an index range scan) and reads only `LIMIT` rows — no discarding. Page 1000 costs the same as page 1.

The price: keyset can't "jump to page 500" (there's no cursor without traversing) and requires a total `ORDER BY`. But for an "infinite feed" / "show more" it's exactly what you want.

## OFFSET vs keyset: what the server does

Take the menu sorted `base_price DESC, id DESC`, and page 2 (two rows each). `OFFSET 2` traverses and **discards** the rows of page 1; keyset, by the cursor of page 1's last row, **jumps straight** to the right place via the index:

```
menu descending (base_price DESC, id DESC):

  #4 Колд брю    5.20   OFFSET 2 → read and discard · keyset → skip via index
  #3 Латте       4.80   OFFSET 2 → read and discard · keyset → skip via index
  ─────────────────────  cursor after page 1 = (4.80, #3)
  #2 Капучино    4.50   ┐
  #1 Эспрессо    3.00   ┘ page 2 (page_size = 2) — this is what we return
  #5 Зелёный чай 2.50    (next — page 3)

OFFSET 2 LIMIT 2                     → server computed 4 rows, discarded 2 (costlier with depth)
WHERE (base_price, id) < (4.80, #3)  → the index jumped to the cursor and read exactly 2
```

| | `LIMIT n OFFSET k` | keyset (`WHERE (cols) < cursor`) |
|---|---|---|
| How it advances | "skip `k` rows" | "give me the rows after the cursor" |
| Deep-page cost | grows linearly (reads and discards `k`) | constant, if `ORDER BY` lands on an index |
| Jump to page N | yes (just change `k`) | no — only "next" |
| Under inserts/deletes | rows "shift" between pages | the cursor is tied to data, not a number |
| When to use | first pages of a small set | "feed" / "show more", deep navigation |

## What our code shows

Three queries in `query.sql`. The base query:

```sql
-- name: FilterMenu :many
SELECT id, name, base_price FROM drinks
WHERE category = sqlc.arg(category) ORDER BY base_price LIMIT sqlc.arg(page_size);
```

And two paginations — both with the same total order `base_price DESC, id DESC`, but a different way to "advance":

```sql
-- PageByOffset:  ... ORDER BY base_price DESC, id DESC LIMIT :page_size OFFSET :skip;
-- PageByKeyset:  ... WHERE (base_price, id) < (:after_price, :after_id)
--                    ORDER BY base_price DESC, id DESC LIMIT :page_size;
```

In `main.go` the keyset pagination walks the whole menu in a loop: after each page the cursor `(after_price, after_id)` becomes `(price, id)` of the last row. The first page uses a "sentinel" cursor — deliberately larger than any real row (descending, that's "from the very start").

## Running it

Bring up the sandbox (from the repo root) and apply the Brew base schema:

```sh
docker compose up -d
make lecture L=03-crud-fluency/03-02-select-where-order-limit T=db-reset
make lecture L=03-crud-fluency/03-02-select-where-order-limit
```

(`T=run` is the default. From inside the unit directory it's `make db-reset`, `make run`.)

Output:

```
1) WHERE/ORDER/LIMIT — category='coffee', по возрастанию цены, LIMIT 2:
   #1 Эспрессо 3.00
   #2 Капучино 4.50

2) Keyset-пагинация по всему меню (по убыванию цены, page_size=2):
   страница 1 (∞ (с начала)): #4 Колд брю 5.20 | #3 Латте 4.80
   страница 2 (после 4.80 / #3): #2 Капучино 4.50 | #1 Эспрессо 3.00
   страница 3 (после 3.00 / #1): #5 Зелёный чай 2.50

3) OFFSET — та же страница 2 через LIMIT 2 OFFSET 2:
   #2 Капучино 4.50 | #1 Эспрессо 3.00
   → результат тот же, но сервер вычислил и отбросил первые 2 строки; keyset — нет.
```

(The demo prints in Russian.) Keyset walked the whole menu in three pages, advancing by cursor each time. Page 2 of keyset (`#2`, `#1`) and page 2 via `OFFSET 2` are **the same rows**: the way to page differs, the result is identical. The difference isn't visible on five drinks, but on a million rows `OFFSET` to a deep page reads a million rows for nothing, while keyset reads only its own two.

## The fence

On five seed rows there's no speed difference — it shows on large tables and with the right index. What we simplified:

- **Keyset is fast precisely when `ORDER BY` lands on an index** — here that would be an index on `(base_price, id)`. Without it the server sorts the whole table anyway and the advantage evaporates (indexes and reading plans — module 06).
- **We didn't show a "total page count".** A `count(*)` over a large filtered set is itself expensive; in production you either cache it or replace it with "is there more" (request `LIMIT n+1` and check whether row `n+1` arrived).
- **`OFFSET` isn't "bad".** For the first few pages of a small set it's simpler and perfectly fine; keyset is for paging deep.
- **The choice depends on navigation.** Need jumps to an arbitrary page — you can't avoid `OFFSET`/numbering; "show more" is enough — use keyset.

## Takeaways

- `WHERE` filters, `ORDER BY` sorts, `LIMIT` caps — and a `LIMIT` without `ORDER BY` returns an undefined set of rows.
- For pagination, `ORDER BY` must be **total**: add a unique tie-break (`id`), or rows with an equal key "float" between pages.
- `OFFSET k` makes the server compute and discard the first `k` rows — the cost of a deep page grows linearly.
- Keyset pagination (`WHERE (cols) < (cursor)`) pages by the cursor from the last row — page cost is independent of depth, if `ORDER BY` lands on an index.
- Keyset can't jump to an arbitrary page and requires a total order — that's its price for scalability.

Next up — the **03-03 "UPDATE/DELETE safely"** unit: we'll learn to change and delete rows, see the blast radius via `RETURNING` and `RowsAffected`, and understand why risky writes belong inside a transaction — so a forgotten `WHERE` can be rolled back.
