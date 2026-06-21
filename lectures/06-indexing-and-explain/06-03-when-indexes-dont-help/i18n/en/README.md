# 06-03 — When indexes don't help

Brew had a "log in by e-mail" button, and an index on the `email` column. Makes sense — we search by e-mail, the index is there. Until Evgeny came down from the marketing floor and turned his phone screen-out toward the table — on it, a complaint from a guest, Alice Ivanova (the very owner of anchor order #1):

> **Evgeny:** Alice can't log in. She types in her e-mail — and the login just hangs. Nobody dropped the e-mail index, right?

He's right: the index is there. But login is case-insensitive: a user registered as `Alice@Brew.example` signs in typing `alice@brew.example`. To make that match, the backend wrote `WHERE lower(email) = lower($1)`. And the query suddenly fell back to a `Seq Scan`, even though the index on `email` was right there. No one dropped it — Postgres simply couldn't use it: the index stores `email` as-is, but the condition has `lower(email)` — a **different value**, one that isn't in the index.

The goal of this unit is to understand the class of conditions that "switch off" an index (they're called **non-sargable** — non-Search-ARGument-ABLE) and the main fix: an **expression index**. This explains the common riddle "the index exists, yet `EXPLAIN` shows a Seq Scan."

## Why a function over a column switches off the index

An ordinary B-tree index on `email` is a sorted directory of the **values of the `email` column**. It can answer questions about `email` itself: "equals," "greater than," "starts with." But `lower(email)` is already the **result of a function**, not a column value; no such values are in the index. To check `lower(email) = 'alice@...'`, Postgres must take every row, compute `lower(email)`, and compare — which is exactly a full `Seq Scan`.

The same trap fires in every condition where the column is wrapped in a computation:

- `WHERE lower(email) = ...`, `WHERE date(created_at) = ...` — a function over the column.
- `WHERE price + 100 > 500` — arithmetic over the column (write `price > 400` instead).
- `WHERE email LIKE '%@brew.example'` — a **leading** `%`: the index sorts by the start of the string, and here the start is unknown.

The rule is simple: **keep the column "bare" on one side of the comparison**, move all the math to the constant side — then the condition is sargable again, and the index switches on.

## The expression index

But for case-insensitive search you genuinely need `lower(email)` — you can't make it bare. So you index **the expression itself**:

```sql
CREATE INDEX accounts_lab_lower_email_idx ON accounts_lab (lower(email));
```

Now the index holds the already-computed `lower(email)` values, sorted. The query `WHERE lower(email) = 'alice@...'` matches the index's expression **verbatim** — and the plan takes an `Index Scan` on it. An important detail: the expression in the query must match the expression in the index letter for letter (`lower(email)`, not `lower(email || '')`), or the planner won't connect them.

> ⚠️ For case-insensitive equality there's also an alternative — the `citext` type (case-insensitive text): comparisons on it are insensitive out of the box, and a plain index works. But `citext` is an extension and a schema-level decision; an index on `lower(email)` changes nothing in the types and is therefore easier to bolt onto an existing table.

## A map of non-sargable conditions

The trap is always the same: the column is wrapped in a computation, and an index on the "bare" column doesn't fit it. The cure is either to unwrap the condition or to index the expression itself:

| Condition (non-sargable) | Why the index stays silent | Cure |
|---|---|---|
| `WHERE lower(email) = …` | the index holds `email` values, not `lower(email)` | expression index `(lower(email))` (or the `citext` type) |
| `WHERE date(created_at) = …` | the index holds `timestamptz`, not `date(...)` | unwrap into a range `created_at >= … AND < …` |
| `WHERE price + 100 > 500` | arithmetic over the column | move it to the constant: `price > 400` |
| `WHERE email LIKE '%@brew.example'` | leading `%` — the start of the string is unknown | `text_pattern_ops` (prefix) / `pg_trgm` (substring), module 07 |

## What our code shows

`demo.sql` builds a lab table `accounts_lab` (200,000 mixed-case e-mails) with an ordinary index on `email` and explains three queries:

```sql
-- Q1: bare column → ordinary index works (Index Scan)
SELECT * FROM accounts_lab WHERE email = 'User150000@Brew.example';
-- Q2: lower(email) with the same index → Seq Scan, Rows Removed by Filter: 199999
SELECT * FROM accounts_lab WHERE lower(email) = 'user150000@brew.example';

CREATE INDEX accounts_lab_lower_email_idx ON accounts_lab (lower(email));

-- Q3: same query → Index Scan on the expression index
SELECT * FROM accounts_lab WHERE lower(email) = 'user150000@brew.example';
```

Q1 and Q3 are equally fast (`Index Scan`), though they search differently; between them is Q2, with the same condition as Q3 but no matching index: `Seq Scan` and 199,999 discarded rows.

## Running it

```sh
docker compose up -d
make lecture L=06-indexing-and-explain/06-03-when-indexes-dont-help
```

Output:

```
== Q1) точное равенство email = ... — обычный индекс работает (Index Scan) ==
                                     QUERY PLAN                                     
------------------------------------------------------------------------------------
 Index Scan using accounts_lab_email_idx on accounts_lab (actual rows=1.00 loops=1)
   Index Cond: (email = 'User150000@Brew.example'::text)
   Index Searches: 1


== Q2) lower(email) = ... с тем же индексом — Seq Scan (условие non-sargable) ==
                         QUERY PLAN                         
------------------------------------------------------------
 Seq Scan on accounts_lab (actual rows=1.00 loops=1)
   Filter: (lower(email) = 'user150000@brew.example'::text)
   Rows Removed by Filter: 199999


== создаём индекс по ВЫРАЖЕНИЮ lower(email) ==

== Q3) тот же lower(email) = ... — теперь Index Scan по индексу-выражению ==
                                        QUERY PLAN                                        
------------------------------------------------------------------------------------------
 Index Scan using accounts_lab_lower_email_idx on accounts_lab (actual rows=1.00 loops=1)
   Index Cond: (lower(email) = 'user150000@brew.example'::text)
   Index Searches: 1
```

(The demo prints in Russian.) Q1 (`email = ...`, bare column) runs as an `Index Scan` on the ordinary index. Q2 is the same `lower(email) = ...` as Q3, but there's no index for the expression yet: `Seq Scan`, `Rows Removed by Filter: 199999` (read everything to find one). After `CREATE INDEX ... (lower(email))` query Q3 matched the index's expression verbatim and took an `Index Scan`. The index was there — just "the wrong one."

## The fence

What we simplified:

- **We fixed one query.** In production you first look at which queries are even hot (`pg_stat_statements`) and index for them, not "just in case": an expression index, like any index, slows writes and takes space.
- **The function in the index must be `IMMUTABLE`** (the same output for the same input). `lower()` is; but `now()` or timezone-dependent expressions can't be indexed (on function volatility — see 09-05).
- **A leading `%` in `LIKE` isn't cured by a bare column.** You need separate mechanisms: `text_pattern_ops` for prefix search or `pg_trgm` trigrams for substring search — that's module 07.
- **The choice of `lower()` vs `citext`, auditing "sleeping" and bloated indexes** — that's schema maintenance, which in a large system your DBA runs.

Your job is to **recognize a non-sargable condition in your own query** (column wrapped in a function/arithmetic) and either unwrap it or add an expression index.

## Takeaways

- An ordinary column index doesn't help if the condition wraps the column in a function or arithmetic (`lower(col)`, `col + 1`) — the condition becomes non-sargable and the plan falls back to a `Seq Scan`.
- The default fix: **keep the column bare**, move the computation to the constant side.
- If the function is genuinely needed (case-insensitive search) — an **expression index** `CREATE INDEX ... (lower(email))`; the query must match the expression verbatim.
- The function in an expression index must be `IMMUTABLE`.
- A leading `%` in `LIKE` isn't cured by a bare column — you need `text_pattern_ops`/`pg_trgm` (module 07).

Next up — **06-04 "Partial, covering, and unique indexes"**: an index over only part of the rows (`WHERE`), `INCLUDE` columns, and the Index-Only Scan that doesn't visit the table at all.
