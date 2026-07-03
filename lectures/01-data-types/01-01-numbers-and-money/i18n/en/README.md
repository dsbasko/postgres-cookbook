# 01-01 — Numbers and money

The first revenue report went out to Viktor in the morning. Before lunch he comes up to the dev floor — Brew's founder, once a barista himself. He's carrying two printouts.

> **Viktor:** Two reports — two numbers. The register says one thing, your numbers are a couple of kopeks short, and it's like that on every line. Where's my money?
>
> **You:** Maybe the register is rounding?
>
> **Viktor:** The register rings up what the guest paid. Guests pay exact amounts.

Marat scans the totals column, pulls a napkin and writes one line: `0.1 + 0.2 ≠ 0.3`.

> **You:** Every addition?
>
> **Marat:** Every one. Nobody stole anything. Summed in float.
>
> **Viktor:** I don't care what it's called. Make the kopeks add up.

Viktor heads back down to the guests; the printouts stay with you.

The export script predates you: it sums in `float`. This unit closes that class of bug up front: understand why `float` is wrong for money, and pick a representation that adds up exactly and maps cleanly into Go. Postgres has `numeric` (exact, arbitrary precision), but in an application money is usually kept simpler — as an integer count of cents: that's how the Brew base schema is built, `drinks.base_price` is a `BIGINT` in cents.

## Why float breaks money

`float8` (a.k.a. `double precision`) stores numbers in binary floating point. Decimal fractions like `0.1` are repeating in binary — they have to be rounded, and when you add them the rounding errors surface. The classic demonstration: `0.1 + 0.2` gives `0.30000000000000004`, not `0.3`. The comparison `0.1 + 0.2 = 0.3` returns `false`. On a single receipt that's the seventeenth digit after the decimal point; over a month of orders it's the very kopek-sized hole on Viktor's printouts.

In `numeric` the same numbers have no error: it's a decimal type with an exact representation, and `0.1 + 0.2 = 0.3` is `true` there. You pay for it in speed, and in the fact that in Go `numeric` arrives not as a plain number but as `pgtype.Numeric` (which you have to unwrap).

## Money as BIGINT cents

The third path — and usually the best one for an application — is to not store a fraction at all. A price of `3.00` is `300` cents, an integer. Addition, multiplying by quantity, summing over an order — all of these are integer operations: exact, fast, no surprises. In Go `BIGINT` is `int64`, a native type with no wrappers. You unfold it into rubles-and-kopeks only at the output boundary: `price/100` and `price%100`.

The Brew base schema keeps all prices this way: `drinks.base_price`, `order_items.unit_price` — `BIGINT` in cents. The report that didn't reconcile would be fixed by replacing the `float` sum with a `sum()` over integer cents.

## Three representations: which to pick

|  | Exactness | Speed | In Go | When to pick |
|---|---|---|---|---|
| `float8` | fractions inexact (`0.1+0.2≠0.3`) | fast | `float64` | measurements where error doesn't matter; for money — never |
| `numeric` | exact, arbitrary precision | slower | `pgtype.Numeric` (unwrap) | fractional per-unit prices, taxes, intermediate math |
| `BIGINT` cents | exact (integer) | fast | `int64`, native | money in an app: store, add, sum |

Folded into one decision card: **money at the boundary — `BIGINT` in cents; a fractional per-gram price — `numeric`; `float` — never.**

## What our code shows

Three queries in `query.sql`. The first is that very trap, on literals:

```sql
-- name: FloatVsNumeric :one
SELECT
    (0.1::float8 + 0.2::float8)::float8           AS float_sum,
    (0.1::numeric + 0.2::numeric)::text           AS numeric_sum,
    (0.1::float8 + 0.2::float8 = 0.3::float8)      AS float_eq_03,
    (0.1::numeric + 0.2::numeric = 0.3::numeric)  AS numeric_eq_03;
```

`float_sum` stays `float8` (in Go a `float64`, and you'll see the "tail"); we cast `numeric_sum` to `text` only for clean printing (in Go `numeric` is `pgtype.Numeric`). The second and third queries treat money as integer cents:

```sql
SELECT id, name, base_price FROM drinks ORDER BY id;                     -- MenuPriced :many
SELECT coalesce(sum(quantity * unit_price), 0)::bigint AS total_cents    -- OrderTotalCents :one
FROM order_items WHERE order_id = $1;
```

`base_price` and the total are `BIGINT` → `int64`. In `main.go` we unfold cents into `₽.kop` with integer arithmetic, without a single `float`:

```go
fmt.Fprintf(w, "%d\t%s\t%d\t%d.%02d\n", d.ID, d.Name, d.BasePrice, d.BasePrice/100, d.BasePrice%100)
```

## Running it

Bring up the sandbox (from the repo root) and apply the base schema:

```sh
docker compose up -d
make lecture L=01-data-types/01-01-numbers-and-money T=db-reset
make lecture L=01-data-types/01-01-numbers-and-money
```

(`T=run` is the default. From inside the unit directory it's `make db-reset`, `make run`.)

Output:

```
1) 0.1 + 0.2 — float8 (Go float64) против numeric:
   float:    0.30000000000000004   (= 0.3? false)
   numeric:  0.3         (= 0.3? true)

2) Меню Brew — base_price BIGINT в центах, печатаем как ₽.коп:
ID  НАЗВАНИЕ     ЦЕНТЫ  ЦЕНА
1   Эспрессо     300    3.00
2   Капучино     450    4.50
3   Латте        480    4.80
4   Колд брю     520    5.20
5   Зелёный чай  250    2.50

3) Итог заказа #1 — sum в центах:  970  (= 9.70)
```

(The demo prints in Russian.) `float` gave `0.30000000000000004` and `= 0.3` → `false` — that very kopek-sized hole from the printouts: not theft, just the seventeenth digit after the decimal point. `numeric` is exact. And Brew's money lives in integer cents: the menu unfolds into `₽.kop` losslessly, and the total of order #1 is `970` cents = `9.70`.

## The fence

> **Zoya — in review, one line:** Cents — correct. Currency and rounding will be on you in production.

`numeric` is not a "bad" type: for money it's exact, and storing sums in `numeric(12,2)` is perfectly fine. We choose integer cents because they map into Go `int64` without the `pgtype.Numeric` wrapper, and arithmetic over them is faster. What we simplified, and what your billing module adds in production:

- **Currency.** A dollar cent ≠ a ruble kopek — you need a currency code next to the amount, or you'll add things that don't add.
- **Rounding.** Banker's rounding of halves by a fixed rule, not "however it comes out" with `float`.
- **Fractional prices and taxes.** A per-gram price, VAT and intermediate math go through `numeric` — and are folded into cents only at the final step.
- **Scale.** The billing module stores both the currency and the number of decimal places; here we keep a single currency and integer cents so the lesson doesn't drift.

One thing is non-negotiable: **money is never computed in `float`**.

## Takeaways

- `float`/`double precision` is inexact for decimal fractions: `0.1 + 0.2 ≠ 0.3`. For money — never.
- `numeric` is exact (`0.1 + 0.2 = 0.3`), but in Go it's `pgtype.Numeric` and slower than integers.
- In an application, money is most convenient as an integer count of minor units (cents) in a `BIGINT` → Go `int64`; unfold into `₽.kop` only at output. That's how the Brew base schema is built.
- `sum()` over `BIGINT` returns `numeric` — cast the result to `::bigint` if you expect `int64`.

The kopeks add up — Viktor's printouts can go back downstairs. Next up — the **01-02 "text, boolean, and the NULL teaser"** unit: we'll look at three "boring" types that applications actually trip over — why we keep `text` and not `char(n)`, what the three-valued logic of `boolean` is, and why `NULL` is not "empty" but "unknown".
