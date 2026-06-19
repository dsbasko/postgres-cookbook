# 03-06 — Sober NULL semantics

The morning starts with a report in the team chat.

> **Anna (in chat, 09:20):** The app. Menu's empty. 09:20. Guests are ordering at the counter.

The "drinks not on the stop-list" query — `... WHERE id NOT IN (SELECT drink_id FROM unavailable)` — returned **zero** drinks, as if the whole menu were unavailable. No error, no warning: the logs are clean.

> **Marat:** Show me the query.
>
> **You:** Here. It ran for a year — what broke overnight?
>
> **Marat:** The query is honest. Second question: what did the database say?
>
> **You:** Zero rows. Not a single error — that's the scary part.

The trail leads into `unavailable`: yesterday the stop-list was edited by hand, and in one row a `NULL` sits where a drink id should be. The edit itself was needed — Stas was pulling a seasonal raf off sale. Marat writes on a napkin: `true AND NULL = NULL`.

> **Stas:** The same NULL that eats my customers? Now it's gone after the menu?
>
> **Marat:** It doesn't eat anyone. For every drink the database answered "unknown" — and to WHERE, "unknown" means "no."

So here is the reckoning for the teaser in 01-02: `NULL` is not "empty" but "unknown." The goal of this unit is to take that three-valued logic apart for good — and never fall into it again.

## Three-valued logic: a comparison with NULL → NULL

`NULL` means "unknown," so any comparison with it returns "unknown": `1 = NULL` isn't `false`, it's `NULL`. `NULL = NULL` is also `NULL` (two unknowns aren't required to be equal). So checking for `NULL` with the `=` operator is meaningless — that's what `IS NULL` / `IS NOT NULL` are for.

For `WHERE`/`CHECK`/`ON` this matters: they let a row through only if the predicate is `true`. A `NULL` (UNKNOWN) predicate is treated as "didn't pass," exactly like `false`. Hence all the traps.

> **Danya:** Hold on, NULL = NULL has to be true: "don't know" on the left, "don't know" on the right. They're identical.
>
> **Marat:** NULL isn't a value, it's "don't know." Does "don't know" equal "don't know"?
>
> **Danya:** …I don't know.
>
> **Marat:** That's exactly how the database answers.

## The NOT IN + NULL trap

`x IN (a, b, c)` expands to `x = a OR x = b OR x = c`. `x NOT IN (a, b, c)` is its negation: `x <> a AND x <> b AND x <> c`. Now substitute `NULL` into the list: `x NOT IN (4, NULL)` = `x <> 4 AND x <> NULL`. The comparison `x <> NULL` is `NULL`. And `anything AND NULL`:

- if `x = 4`: `false AND NULL` = `false` → the row doesn't pass (which is even correct);
- if `x <> 4`: `true AND NULL` = `NULL` → the row **doesn't pass**, though it should!

The upshot: as soon as the `NOT IN` list contains a `NULL`, the predicate cannot become `true` for any row — the query returns empty. This isn't rare or "bad data": a subquery over a nullable column easily drags in a `NULL`.

The cure is `NOT EXISTS`: it asks "does a matching row exist," working at the "yes/no" level rather than on a comparison with `NULL`. An `unavailable` row with a `NULL` won't match any `drinks.id` (`u.drink_id = d.id` with `NULL` yields `NULL` → no match), so it excludes no one extra. `NOT EXISTS` (or `<> ALL (... WHERE col IS NOT NULL)`) is the proper "NULL-safe NOT IN."

## Three tools for working with NULL

- `COALESCE(a, b, c, ...)` — the first non-`NULL` from the list. The classic is a default value: `COALESCE(nickname, name, 'anonymous')`.
- `NULLIF(a, b)` — `NULL` if `a = b`, otherwise `a`. A frequent trick is guarding division by zero: `x / NULLIF(y, 0)` returns `NULL` instead of an error when `y = 0`.
- `IS DISTINCT FROM` / `IS NOT DISTINCT FROM` — `NULL`-safe "not equal"/"equal." Unlike `=`/`<>`, they treat `NULL` as an ordinary value: `NULL IS NOT DISTINCT FROM NULL` = `true`, `1 IS DISTINCT FROM NULL` = `true`.

## The `NOT IN` trap and NULL tools

Here's why a single `NULL` in the list zeroes out the answer — a step-by-step layout:

```
WHERE id NOT IN (SELECT drink_id FROM unavailable)      -- list = {4, NULL}

  id NOT IN (4, NULL)
        │  expands to the negation of IN
        ▼
  id <> 4  AND  id <> NULL  ←── id <> NULL is ALWAYS NULL (comparison with "unknown")
        │
        ├─ id = 4  :  false AND NULL = false  → doesn't pass (which is correct)
        └─ id <> 4 :  true  AND NULL = NULL   → does NOT pass, though it should!
        │
        ▼
  no row can become true  →  the result is empty
```

The cure is switching to `NOT EXISTS` (it works at the "yes/no" level, not on a comparison with `NULL`) or an explicit `WHERE col IS NOT NULL` in the subquery. And here's a cheat-sheet of tools for working with `NULL`:

| Tool | What it does | Typical use |
|---|---|---|
| `COALESCE(a, b, …)` | the first non-`NULL` from the list | a default value: `COALESCE(nickname, name, 'anonymous')` |
| `NULLIF(a, b)` | `NULL` if `a = b`, otherwise `a` | a divide-by-zero guard: `x / NULLIF(y, 0)` |
| `IS [NOT] DISTINCT FROM` | `NULL`-safe "not equal" / "equal" | comparing nullable values: "did a field change" |
| `IS [NOT] NULL` | a `NULL` check (not `=` / `<>`) | the only correct test for `NULL` |

## What our code shows

`NullLogic` gathers four facts on literals:

```sql
SELECT
    ((NULL = NULL) IS NULL)            AS eq_is_null,         -- (=) with NULL → NULL, not true
    (NULL IS NOT DISTINCT FROM NULL)   AS is_not_distinct,    -- NULL-safe equality
    (NULLIF(100, 100) IS NULL)         AS nullif_eq_is_null,  -- NULLIF(a,a) → NULL
    COALESCE(NULL::int, NULL, 42)      AS coalesce_val;       -- the first non-NULL
```

And the trap we show on data: a list `unavailable = {4, NULL}` and one question "how many drinks are available" two ways:

```sql
-- trap:    ... WHERE id NOT IN (SELECT drink_id FROM unavailable)
-- correct: ... WHERE NOT EXISTS (SELECT 1 FROM unavailable u WHERE u.drink_id = d.id)
```

## Running it

Bring up the sandbox (from the repo root) and apply the Brew base schema plus the unit's table:

```sh
docker compose up -d
make lecture L=03-crud-fluency/03-06-null-semantics-reckoning T=db-reset
make lecture L=03-crud-fluency/03-06-null-semantics-reckoning
```

(`T=run` is the default. From inside the unit directory it's `make db-reset`, `make run`.)

Output:

```
1) Трёхзначная логика NULL и инструменты:
   (NULL = NULL) IS NULL            = true   (= с NULL даёт NULL, не true)
   NULL IS NOT DISTINCT FROM NULL   = true   (NULL-безопасное равенство)
   NULLIF(100, 100) IS NULL         = true   (NULLIF → NULL, когда равны)
   COALESCE(NULL, NULL, 42)         = 42     (первое не-NULL)

2) Список недоступных напитков unavailable = {4, NULL} (NULL затесался по ошибке).

3) «Сколько напитков доступно?» — два способа:
   NOT IN (...)      → 0   ← ловушка: NULL в списке обнулил ответ
   NOT EXISTS (...)  → 4   ← правильно (5 напитков минус колд брю #4)
```

(The demo prints in Russian.) The same question, the same data — two different answers. `NOT IN` with a list containing a `NULL` returned `0` (the whole menu "unavailable"), while `NOT EXISTS` returned an honest `4` (five drinks minus cold brew). One `NULL` in the source — and `NOT IN` silently lied.

## The fence

The best defense against the trap is to not allow `NULL` where it isn't needed: a `NOT NULL` on the column (module 02) makes it impossible in principle. What we simplified:

- **The `NOT IN` trap is the most famous, but not the only one.** Three-valued logic surfaces everywhere there are nullable columns: `WHERE`, `JOIN ... ON`, `CHECK`, aggregates (`count(col)` skips `NULL`, `count(*)` doesn't — see 01-02 and later 04-03), `DISTINCT` (treats all `NULL`s as equal, unlike `=`).
- **The standard hygiene in production:** put `NOT NULL` where a value must exist; in subqueries for `NOT IN` either switch to `NOT EXISTS` or explicitly filter `WHERE col IS NOT NULL`; for comparing values that **can** be `NULL`, use `IS DISTINCT FROM`, not `<>`.
- **`NULL` is "unknown," not "zero" and not "empty string".** Conflating them is a separate source of bugs.

## Takeaways

- `NULL` is "unknown": a comparison with it (`=`, `<>`, `<`) yields `NULL` (UNKNOWN), not `true`/`false`. Check it via `IS NULL`.
- `WHERE`/`JOIN ON`/`CHECK` let a row through only on `true`; a `NULL` predicate is like `false` to them.
- `NOT IN (subquery with a NULL)` can never return `true` → silently returns empty. Use `NOT EXISTS` (or filter `NULL` in the subquery).
- `COALESCE` — a default value; `NULLIF(a,b)` — `NULL` on equality (a divide-by-zero guard); `IS [NOT] DISTINCT FROM` — `NULL`-safe comparisons.
- Where a value must exist — put `NOT NULL`: the best trap is the one that can't be armed.

This is the end of module 03 — "CRUD fluency." In the evening Stas finishes reading the stop-list postmortem — and lingers at your desk.

> **Stas:** I've got NULL memorized. Now tell me: how many more are out there like Karina — signed up and gone quiet?

There's no honest single number to answer that with yet — for that you need to tie tables together. Next up — module **04 "Querying across tables"**: tying data together with `JOIN`s, aggregating via `GROUP BY`/`HAVING`, taking "the latest per customer" via `DISTINCT ON`, and meeting the `NOT IN` + `NULL` trap again — now as part of the choice between `EXISTS` and `IN`.
