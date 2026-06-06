# 03-01 — INSERT and RETURNING

Brew launches a loyalty program. The app issues a card to a new customer, and right after the insert it needs that card's `id` — to show it to the customer, attach bonuses, write it to a log. Naive code does two queries: first `INSERT`, then `SELECT ... WHERE card_no = ...` to learn the generated `id`. That's an extra round-trip to the database, an extra chance for a race (between the insert and the read someone could have changed the row), and simply more code.

The goal of this unit is to collapse that into one query. `INSERT` (like `UPDATE`/`DELETE`) has a `RETURNING` clause: it returns the rows the command just wrote, including the values the server filled in — the generated `id`, columns with a `DEFAULT`. No second `SELECT`.

## RETURNING gives back what the server assigned

When you insert a row, some values don't come from you: `id` is handed out by `GENERATED ALWAYS AS IDENTITY`, while `points` and `created_at` are filled in by `DEFAULT`. To learn them the classic way you run a separate `SELECT` — but it sees an already-different (possibly changed) row and costs another trip to the server.

`RETURNING` fixes this at the root: it hands back the values of exactly the rows the command wrote, in the same query and the same transaction. `INSERT ... RETURNING id` means "insert and tell me right away which `id` you gave it." You can return any columns of the written row, including computed expressions over it.

## RETURNING works for many rows too

`RETURNING` is not a "single-row trick." A command that writes several rows (a multi-row `INSERT ... VALUES (...), (...)`, an `INSERT ... SELECT`, a bulk `UPDATE`) returns one result row per written row. In sqlc such a query is tagged `:many` and arrives in Go as a slice — one element per inserted card.

## What our code shows

Two queries in `query.sql`. The first is a single insert where the server fills in three values itself:

```sql
-- name: IssueCard :one
INSERT INTO loyalty_cards (customer_id, card_no)
VALUES ($1, $2)
RETURNING id, points, (created_at IS NOT NULL)::boolean AS created_set;
```

We don't pass `id` — `IDENTITY` assigns it; we don't pass `points` or `created_at` — `DEFAULT` fills them. `RETURNING` hands them back immediately. The value of `created_at` (it's `now()`) is non-deterministic, so in the demo we print not the time itself but the fact that "the column is set" (`created_set`) — so the output reproduces verbatim. The second query inserts two cards at once and returns the `id` of each:

```sql
-- name: IssueCardsBulk :many
INSERT INTO loyalty_cards (customer_id, card_no)
VALUES (sqlc.arg(cust_a), sqlc.arg(card_a)),
       (sqlc.arg(cust_b), sqlc.arg(card_b))
RETURNING id, card_no;
```

In `main.go` everything is thin: call the generated method and print what `RETURNING` gave back. Not a single separate `SELECT` for the `id`:

```go
card, err := queries.IssueCard(ctx, db.IssueCardParams{CustomerID: 1, CardNo: "BREW-0001"})
// card.ID, card.Points, card.CreatedSet — all from RETURNING
```

## Running it

Bring up the sandbox (from the repo root) and apply the canon plus the unit's table:

```sh
docker compose up -d
make lecture L=03-crud-fluency/03-01-insert-and-returning T=db-reset
make lecture L=03-crud-fluency/03-01-insert-and-returning
```

(`T=run` is the default. From inside the unit directory it's `make db-reset`, `make run`.)

Output:

```
1) INSERT ... RETURNING — серверные значения обратно одним запросом:
   выдали карту: id=1, points=0 (по DEFAULT), created_at заполнен=true
   → id и points не передавали — их вернул RETURNING, без второго SELECT.

2) Многострочный INSERT ... RETURNING — то же и для многих строк:
ID  CARD_NO
2   BREW-0002
3   BREW-0003
   → одна команда вставила обе карты; RETURNING вернул id каждой.
```

(The demo prints in Russian.) The card with `id=1` came back immediately with its generated `id`, `points=0` (the `DEFAULT` value), and confirmation that `created_at` is set. The multi-row insert returned the `id` of both cards — `2` and `3`. Not a single extra `SELECT`.

## The fence

`RETURNING` returns as much data as you ask for — don't drag back `RETURNING *` when you only need the `id`: the extra columns travel over the wire for nothing. What we simplified: to insert a **variable** number of rows from a Go slice in one command you don't use a multi-row `VALUES` (its arity is fixed) but `INSERT ... SELECT ... FROM unnest($1::bigint[])` — unfolding an array into rows; and for **bulk** loading (tens of thousands of rows and more) an `INSERT` of any shape loses to the `COPY` protocol (`CopyFrom` in pgx), which we'll cover in **09-01**. Here both cards are given explicitly so the output is reproducible. And one more thing: `RETURNING` hands back the rows of the same command in the same transaction — it's not a substitute for an audit trail (triggers, a separate history table) needed when history must be stored independently of which code did the write (we return to this in **03-05** and module 09).

## Takeaways

- `INSERT ... RETURNING` hands back the values of just-written rows — the generated `id`, columns with a `DEFAULT` — in one query, with no second `SELECT`.
- It removes the extra round-trip and a class of "inserted → read the wrong row" races.
- `RETURNING` works for many rows too: a command that wrote N rows returns N result rows (in sqlc — `:many`, in Go — a slice).
- Return only the columns you need; `RETURNING *` drags the whole row over the wire.
- `UPDATE`/`DELETE` have `RETURNING` as well — it's a through-line idiom for the whole module.

Next up — the **03-02 "SELECT: WHERE / ORDER / LIMIT and keyset pagination"** unit: we'll learn to fetch exactly the rows we need in the right order and page through the menu so that deep pages don't turn into a full table scan.
