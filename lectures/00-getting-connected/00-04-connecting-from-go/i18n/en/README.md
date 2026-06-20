# 00-04 — Connecting from Go

The first feature is yours: search drinks by category. The query is built by gluing a string: `"... WHERE category = '" + input + "'"`. In the demo it all worked. Today is the review: Dmitry silently types `' OR 1=1 --` into the search field and turns the screen toward you. The storefront returns **the entire menu** — bypassing any filter.

> **You:** But it worked in the demo!
>
> **Dmitry:** It worked. The field got `coffee` — not a quote with a tail. Your input became the text of the query.
>
> **Botyr:** Same story with me in my first month. It flew in testing with ten clients — but in production the search field gets more than coffee. Ever since, anything from outside travels only through `$1`, separately from the query text.
>
> **Dmitry:** No dressing-down. Let's work out together what goes to the server.

That's SQL injection — the number-one mistake from every list of web vulnerabilities. The goal of this unit is to make the first query to Postgres from Go correctly: a `pgxpool` pool, a `$1` parameter — and on one and the same input see the difference between string gluing and parameter binding. This is a raw-pgx unit: we deliberately write `rows.Scan` by hand so that in the next unit (00-05) we can see exactly what sqlc takes off our plate.

## The connection pool: `pgxpool`, not a single connection

From 00-02 we know: to do anything with data you need a connection. But opening a new TCP connection per query is expensive — the handshake, authentication, and session setup cost milliseconds that turn into a bottleneck under load. So the `pgx` driver works through a **pool**: a set of already-open connections that get reused. Request a connection → run the query → return it to the pool without closing it.

In this course the pool is created with a single `internal/pg.NewPool` call — it reads `DATABASE_URL` and returns a ready `*pgxpool.Pool` with sandbox defaults:

```go
pool, err := pg.NewPool(ctx)   // connection pool to the sandbox
defer pool.Close()             // return all connections on exit
if err := pool.Ping(ctx); err != nil { /* DB unreachable */ }
```

The pool is **lazy**: there's no real connection right after `NewPool` — it's established on the first query. `Ping` is a way to check the DB's availability explicitly: if the sandbox isn't up, the error arrives here, not in the middle of business logic. (The lifecycle of connections in the pool is a separate topic — 00-06 is dedicated to it.)

## `$1` parameters: not about convenience, about safety

A query with a parameter looks like this:

```go
rows, err := pool.Query(ctx, "SELECT ... FROM drinks WHERE category = $1", "coffee")
```

`$1` is a **placeholder**, and `"coffee"` travels as a separate argument. The key point is how this goes to the server: the SQL text and the parameter values are sent **separately**, in different fields of the protocol. The server parses the SQL with placeholders apart from the values, and `$1` arrives in its own field and reaches the executor as a parameter — it isn't pasted into the query text and never passes through the SQL parser at all. The value of `$1` physically cannot become part of the command.

Compare with string gluing:

```go
// ❌ NEVER do this
sql := "SELECT ... FROM drinks WHERE category = '" + input + "'"
```

Here `input` becomes the **text of the query**. For an honest `coffee` you get correct SQL. But for `' OR 1=1 --` you get:

```sql
SELECT ... FROM drinks WHERE category = '' OR 1=1 --'
```

The quote closed early, `OR 1=1` made the condition always true, and `--` commented out the tail. The filter is bypassed, the table leaks. With the `$1` parameter, the same input is simply the string category value `' OR 1=1 --`, which isn't in the menu: zero rows. That's the whole mechanism: keep code and data separate.

Drawn as it goes to the server:

```
   string gluing — ONE envelope, the data is pasted into the query text:

     "… WHERE category = '" + input + "'"   ─▶  one SQL text  ─▶  parser
        input = ' OR 1=1 --  becomes part of the query ──────────────┘  (became code)

   the $1 parameter — TWO envelopes, the value travels past the parser:

     envelope 1 (text):   "… WHERE category = $1"   ─▶  parser ─▶ parsed query with $1
     envelope 2 (value):  "coffee"  ──────────────────────────▶ parameter to the executor
                                                                 (never goes through the parser)
```

`pgx` nudges you onto the right path by design — it has no API for "run this glued string with the data inside it", only query-with-placeholders + arguments. To shoot yourself in the foot, you have to assemble the injection by hand deliberately (which is what we do in the anti-demo — on a safe, read-only sandbox).

## What our code shows

At the center is `main.go`. We write the query as a string and map the result rows into a struct by hand:

```go
type drink struct {
	id        int64
	sku, name string
	category  string
	basePrice int64
}

rows, err := pool.Query(ctx, "SELECT id, sku, name, category, base_price FROM drinks WHERE category = $1", "coffee")
defer rows.Close()
for rows.Next() {
	var d drink
	if err := rows.Scan(&d.id, &d.sku, &d.name, &d.category, &d.basePrice); err != nil {
		return err
	}
	out = append(out, d)
}
return rows.Err()
```

This loop — `Query → for rows.Next → Scan → rows.Err` — is the "manual" way to read data from Postgres in Go. It works, but it's easy to get wrong: mix up the column order in `Scan`, forget `rows.Err()`, fail to close `rows`. Remember this code — in 00-05 sqlc will generate exactly it from `query.sql`, and the comparison will be vivid.

The anti-demo uses the same `queryDrinks`, but twice on the malicious input: once through the unsafe gluing, once through `$1`. The difference shows in the output.

## Running it

Bring up the sandbox (from the repo root) and apply the Brew base schema:

```sh
docker compose up -d
make lecture L=00-getting-connected/00-04-connecting-from-go T=db-reset
```

Run the demo:

```sh
make lecture L=00-getting-connected/00-04-connecting-from-go
```

(`T=run` is the default. From inside the unit directory it's simply `make db-reset` and `make run`.)

Output:

```
1) Параметризованный поиск: category = $1, значение 'coffee' — штатный путь.
ID  SKU     НАЗВАНИЕ  КАТЕГОРИЯ  ЦЕНА
1   ESP-01  Эспрессо  coffee     3.00
2   CAP-01  Капучино  coffee     4.50
3   LAT-01  Латте     coffee     4.80

2) Злонамеренный ввод в поле «категория»:  ' OR 1=1 --

   Небезопасно (склейка строкой): запросили одну категорию — сервер вернул 5 строк (вся таблица утекла).
   Безопасно ($1 как параметр): тот же ввод — это литерал категории, совпадений нет, 0 строк.
```

(The demo prints in Russian.) The normal search returned 3 drinks in the `coffee` category. The same malicious input: with gluing, all 5 menu rows leaked; with binding, zero. One and the same text — a different outcome, decided solely by how the value got into the query.

> [!NOTE]
> **Check yourself.** The anti-demo feeds `' OR 1=1 --` into the "category" field.
> Predict: how many rows does the unsafe gluing return, and how many does the `$1`
> parameter? And why exactly that many in each case?

> [!TIP]
> **Answer.** Gluing — 5 rows, the whole menu: the input closed the quote and
> appended `OR 1=1`, making the condition always true, the filter bypassed (as in
> the output above). The `$1` parameter — 0 rows: the same text went in a separate
> field, past the parser, and stayed the string category value `' OR 1=1 --`, which
> isn't in the menu. What decides is not how "dangerous" the input is, but whether
> it landed in the query text or in a parameter.

## The fence

- You **never** glue SQL together from strings — not for user input, not "for config values, those are trusted." The only correct way to pass a value into a query is a parameter (`$1`, `$2`, …). In this unit we write SQL as a string on purpose, to show the mechanics; in real code you won't even be tempted — `pgx` only accepts query-with-placeholders.
- `rows.Scan` by hand is acceptable, but it's boilerplate where it's easy to fail silently (the compiler won't catch a swapped column order). That's why the course default isn't raw pgx but sqlc: the next unit removes this manual mapping, leaving the SQL itself.

## Takeaways

- `pgxpool` is a pool of reusable connections; `pg.NewPool` returns a ready pool, `pool.Ping` checks DB availability. The pool is lazy: the connection opens on the first query.
- A `$1` parameter and its value travel to the server separately: the value never passes through the SQL parser and cannot become code. That shuts down SQL injection at the root.
- Gluing SQL from strings opens injection (`' OR 1=1 --` → the whole table leaks). Pass values only as parameters.
- The manual `Query → Scan → rows.Err` works, but it's boilerplate — sqlc will generate it for us.

Next up — the **00-05 "typed queries via sqlc"** unit: we'll take exactly this manual row mapping and replace it with code generation. We'll write `query.sql` with a `$1` parameter, run `sqlc generate`, and get a typed method where the column order and types are checked against the schema at build time, not at runtime.
