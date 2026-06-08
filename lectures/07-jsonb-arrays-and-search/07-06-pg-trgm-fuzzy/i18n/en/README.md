# 07-06 — fuzzy search with pg_trgm

The full-text search from 07-05 is strong on morphology but helpless against a typo: a guest types "capucino" into the menu search, and FTS, which searches by normalized lexemes, finds nothing — there's no such lexeme in the index. But the user meant "Cappuccino." You need a search that forgives mistakes and works by spelling similarity rather than by words — and in Postgres it lives in the `pg_trgm` extension.

The goal of this unit is fuzzy search on trigrams: the `similarity` function (how alike two strings are), the `%` operator (alike above a threshold — a ready "did you mean"), and accelerating `LIKE`/`ILIKE` with a mid-string substring via a GIN index. And at the end — a decision matrix: when FTS, when `pg_trgm`, and when it's time for an external engine.

## Trigrams and similarity

`pg_trgm` cuts a string into **trigrams** — runs of three consecutive characters (`Cappuccino` → `cap`, `app`, `ppu`, …). The similarity of two strings is the fraction of shared trigrams: `similarity(a, b)` returns a number from 0 (nothing in common) to 1 (identical). A typo changes only a few trigrams, so `similarity('Cappuccino', 'capucino')` stays high, while for dissimilar names it drops to nearly zero. This is a fundamentally different search from FTS: it knows nothing of words or morphology — it compares spelling character by character, which is exactly why it catches typos.

## A word in trigrams

A word is cut into runs of three consecutive characters (plus space padding at the edges), and similarity is the fraction of shared triples:

```
  Cappuccino → cap app ppu puc ucc cci cin ino
  capucino   → cap apu puc uci cin ino

  shared triples: cap, puc, cin, ino → similarity('Cappuccino','capucino') = 0.538
  dissimilar words share almost none → similarity ≈ 0
```

That's why trgm catches typos and FTS doesn't: the comparison is by spelling, not by dictionary lexemes.

## The `%` operator and the threshold

Computing `similarity` against everything and sorting works, but for a "similar / not similar" filter there's the `%` operator: `name % 'capucino'` is true when similarity exceeds the `pg_trgm.similarity_threshold` parameter (default `0.3`). That's the ready "did-you-mean": keep only what's above the threshold, sort by `similarity DESC`, and show "did you mean…". The threshold is tunable per task: lower means more candidates (and noise), higher is stricter.

## Accelerated LIKE and the decision matrix

A `pg_trgm` bonus is a substring index. A plain B-tree doesn't speed up `name LIKE '%presso%'` (the pattern starts with `%` — nothing to search from), but a GIN with the `gin_trgm_ops` operator class indexes trigrams and therefore accelerates both `%`-similarity and `ILIKE '%...%'` (06-05). Where to use what:

| Task | Tool |
|---|---|
| Word search, morphology, relevance | full-text search (`tsvector`/`@@`, 07-05) |
| Typos, "spelled similarly," `ILIKE '%x%'` | `pg_trgm` (`similarity`/`%` + trgm-GIN) |
| Exact "is a value in a list" | array `@>`/`= ANY` or a junction (07-04) |
| Morphology of complex languages, synonyms, ML relevance, huge scale | external engine (Elasticsearch, etc.) |

## What our code shows

A lab table `menu_search_lab` (menu items) + the `pg_trgm` extension and a trgm-GIN. Three queries around the typo `capucino`:

```sql
SELECT name, similarity(name, 'capucino') FROM menu_search_lab ORDER BY similarity DESC;  -- SimilarityScores
SELECT name FROM menu_search_lab WHERE name % 'capucino' ORDER BY similarity DESC;         -- DidYouMean (threshold 0.3)
SELECT name FROM menu_search_lab WHERE name ILIKE '%presso%';                              -- AcceleratedLike
```

`similarity` returns `real`; we round with `round(...::numeric, 3)::text` for stable output. The names are English: similarity is a comparison of trigram sets, so it's deterministic and independent of locale. The unit adds its own extension and table → `make db-reset` applies them via `brew.Apply`.

## Running it

```sh
docker compose up -d
make lecture L=07-jsonb-arrays-and-search/07-06-pg-trgm-fuzzy T=db-reset
make lecture L=07-jsonb-arrays-and-search/07-06-pg-trgm-fuzzy
```

Output:

```
1) similarity(name, 'capucino') — схожесть по триграммам (опечатка в Cappuccino):
НАЗВАНИЕ    SIMILARITY
Cappuccino  0.538
Cold Brew   0.056
Americano   0.056
Espresso    0.000
Latte       0.000
Flat White  0.000
Macchiato   0.000

2) name % 'capucino' — выше порога 0.3 (did-you-mean):
   Cappuccino (similarity 0.538)

3) name ILIKE '%presso%' — подстрока в середине, ускоряется trgm-GIN:
   Espresso
```

The `similarity` to the typo `capucino` is high only for `Cappuccino` (`0.538`), nearly zero for the rest: few shared trigrams. So the `%` operator (threshold `0.3`) left a single candidate — a ready "did you mean Cappuccino." And `ILIKE '%presso%'` found `Espresso` by a mid-string substring — the case where an ordinary index is powerless and the trgm-GIN saves the day.

## The fence

`pg_trgm` is a point tool, not a replacement for search:

- `similarity` knows nothing of meaning: `Latte` and `Matte` look alike to it, though they're different things; on short strings the noise is higher;
- the `%` threshold has to be calibrated to the data — too low a value floods the results with junk;
- on large tables a trgm-GIN is noticeably heavier than an ordinary index in both writes and size; putting it on "just in case" is a bad idea;
- `pg_trgm` complements FTS rather than replacing it: the typical combo is "FTS by words + trgm for typos/`ILIKE`."

When you need morphology for a dozen languages, synonyms, learned relevance, or search over terabytes, that's no longer "search in the database" but an external engine (the handoff into Elasticsearch in the sibling kafka-cookbook); your DBA will tell you where the line is.

## Takeaways

- `pg_trgm` searches by spelling similarity (trigrams), not by words: `similarity(a,b)` ∈ [0,1] catches typos, which FTS and `LIKE` can't.
- The `%` operator is "similar above a threshold" (`pg_trgm.similarity_threshold`, default `0.3`) — a ready "did-you-mean."
- A GIN `gin_trgm_ops` accelerates both `%` and `ILIKE '%substring%'` (where a plain B-tree is useless).
- The matrix: words/morphology → FTS; typos/`ILIKE` → trgm; exact membership → array/junction; scale/synonyms/ML → external engine.

That closes module 07: from `jsonb` access and its limits — through SQL/JSON path, arrays-vs-junction, and full-text and fuzzy search. Next up — module **08 "Analytics, window functions, and LATERAL"**: we compute running totals, rank top-N per group, build day-over-day and moving averages, walk a category tree with a recursive CTE, and kill N+1 with LATERAL — analytics right inside SQL, without offloading to the application.
