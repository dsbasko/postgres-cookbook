# 07-05 — full-text search

Brew's knowledge base has grown: brewing articles, guides, barista notes. In the morning chat, a report from Ruslan lands:

> **Ruslan (in chat, 09:15):** New hires can't find the guides. They search "brewing" — the article on "brew" stays silent. Substring search, right?

The ticket reaches the team. Botyr reacts first:

> **Botyr:** Should we bring in Elasticsearch? A separate engine, search out of the box.
>
> **Dmitry:** Let's first see what the database can do.

A naive search — `body LIKE '%brew%'` — is almost useless: it won't find `brewing` for the query `brew`, can't tell a title from a body, can't sort by relevance, and on a large table always reads everything. Postgres can do real full-text search right in the database — without a separate engine like Elasticsearch, as long as the volumes are moderate.

The goal of this unit is to assemble a working FTS on built-in types: text → `tsvector` (normalized lexemes), query → `tsquery`, the `@@` operator for matching, and `ts_rank` for ranking. Plus two production techniques: a generated `tsvector` column (the database keeps it in sync itself) and `setweight` (the title matters more than the body).

## tsvector and tsquery: what the search sees

FTS doesn't search raw text — it works on a `tsvector`: text parsed into **lexemes** (normalized words) with positions. The parsing is done by a language configuration: `'english'` reduces words to a stem (stemming: `brewing` and `brew` → one lexeme `brew`, `hours` → `hour`) and drops stop words (`is`, `about`, `for` — noise you don't search by). The query is normalized too — into a `tsquery` — by the same configuration, so `brewing` in the query finds `brew` in the text. The `@@` operator checks whether a `tsvector` satisfies a query. That's the difference from `LIKE`: the search goes by the meaning of words, not by substring.

## A generated column and weights

Running `to_tsvector` on every query is expensive; better to compute it once at write time and index it. We make the `tsvector` a **generated column** (`GENERATED ALWAYS AS (...) STORED`): the database recomputes it on any `INSERT`/`UPDATE` — no triggers. We put a **GIN index** under it, and `@@` flies by index, not by scan (06-05). Then `setweight`: we tag the title's lexemes with weight `A`, the body's with `B`, and concatenate (`||`). When ranking, `ts_rank` weighs a title match more than a body match — relevance out of the box.

## ts_rank: sorting by relevance

`@@` answers "yes/no," but the user needs an order. `ts_rank(tsv, query)` gives a number that grows with how often and how "heavily" (per `setweight`) the query lexemes occur. We sort by it `DESC` — and the most relevant come up top. In the demo the query `brew` finds two articles, but "Cold brew guide" has the word both in the title (weight `A`) and the body (`B`), so its rank is noticeably higher.

## The search pipeline

Both the text and the query go through the same normalization and meet at the `@@` operator:

```
  article body ──to_tsvector('english',…)──▶ tsvector   'brew':2,8 'cold':1 'hour':11 …
                  stemming + stop words           │
                                                  @@   match?
                                                  │
  query 'brewing' ─plainto_tsquery('english')─▶ tsquery   'brew'
                  the same normalization

  rows that pass @@ ──ts_rank(tsvector, tsquery)──▶ a relevance number → ORDER BY DESC
```

That's why `brewing` finds `brew`: both sides are normalized by the same configuration, so one lexeme `brew` ends up in both the index and the query. `LIKE` can't do that — it compares substrings of raw text.

> **Evgeny (in chat):** Guests search for "limited" — seasonal. Let a title with that word weigh more than an article where it just happened to slip into the description.

## What our code shows

A lab table `kb_articles` (an English knowledge base) with a generated `tsvector` + GIN. Four queries:

```sql
SELECT to_tsvector('english', body) FROM kb_articles WHERE id = 2;             -- ShowTsvector (lexemes)

SELECT id, title, ts_rank(tsv, plainto_tsquery('english','brew')) AS rank      -- SearchRanked
FROM kb_articles WHERE tsv @@ plainto_tsquery('english','brew') ORDER BY rank DESC;

SELECT id, title FROM kb_articles WHERE tsv @@ to_tsquery('english','milk & cappuccino');  -- SearchAnd
SELECT id, title FROM kb_articles WHERE tsv @@ plainto_tsquery('english','brewing');       -- StemmingMatch
```

The rank (a float) is rounded with `round(...::numeric, 4)::text` — a stable printable number. The content is English: the `'english'` configuration is built in and does stemming deterministically, with no dependence on the machine locale. The unit adds its own table → `make db-reset` applies it via `brew.Apply`.

## Running it

```sh
docker compose up -d
make lecture L=07-jsonb-arrays-and-search/07-05-full-text-search T=db-reset
make lecture L=07-jsonb-arrays-and-search/07-05-full-text-search
```

Output:

```
1) tsvector тела статьи 2 (стемминг brewing→brew, hours→hour; стоп-слова выброшены):
   'brew':2,8 'cold':1 'hour':11 'sixteen':10 'temperatur':7 'time':5

2) поиск 'brew', ранжирование ts_rank (вес A заголовка > B тела):
ID  ЗАГОЛОВОК        РАНГ
2   Cold brew guide  0.6957
1   Espresso basics  0.2432

3) to_tsquery('milk & cappuccino') — нужны обе лексемы:
   1  Espresso basics
   3  Milk steaming

4) запрос 'brewing' (стем → brew) — морфология, чего не дал бы LIKE:
   1  Espresso basics
   2  Cold brew guide
```

The first block shows what a `tsvector` stores: `brewing` and `brew` merged into `'brew':2,8`, `temperature` → `'temperatur'`, `hours` → `'hour'`, while `is`/`about`/`not`/`for` were dropped as stop words. The `brew` search put "Cold brew guide" higher (`0.6957` vs `0.2432`) — the title weight kicked in. `to_tsquery` with `&` required both lexemes, and the query `brewing`, thanks to stemming, found the same as `brew`.

## The fence

FTS in Postgres is an excellent default as long as volumes are moderate and you have one or two languages. Its limits:

- `ts_rank` is simple frequency ranking: no learning, no built-in synonyms, no typos (fuzziness is `pg_trgm`'s job, see 07-06), and no distributed index;
- the language configuration is chosen deliberately: `'english'` stems for English, Russian needs `'russian'`, and `'simple'` doesn't stem at all;
- without a GIN on `tsv` every `@@` is a `Seq Scan` recomputing `to_tsvector` on the fly (06-05);
- in production you keep synonym dictionaries and thesauri and treat relevance as a product.

Beyond those limits you reach for an external engine — the very handoff into Elasticsearch in the sibling kafka-cookbook.

## Takeaways

- FTS: text → `tsvector` (lexemes, stemmed, stop words removed), query → `tsquery`, match via `@@`.
- Stemming gives morphology for free (`brewing` finds `brew`) — something `LIKE '%...%'` can't do.
- A generated `tsvector` column (`GENERATED ALWAYS AS ... STORED`) + GIN — compute once, search by index; `setweight` lifts the title over the body.
- `ts_rank` sorts by relevance; the limits of FTS (typos, synonyms, scale) are the cue for `pg_trgm` or an external engine.

Next up — **07-06 "Fuzzy search with pg_trgm"**: FTS doesn't forgive typos — we'll add trigram similarity (`similarity`, the `%` operator), accelerated `LIKE`, and a decision matrix: when FTS, when trgm, and when it's time for an external engine.
