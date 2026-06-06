# 01-03 — Date, time, and timestamptz

The Brew app expanded to a second city, and odd things started immediately. An order placed at 12:00 Moscow time showed as 12:00 in the St. Petersburg export, but as 09:00 in the server-side analytics (UTC), and nobody could tell which time was "real." Digging in, they found a column of type `timestamp` (without a zone) in one table. It stored "wall-clock" time with no zone attached, and every service interpreted it its own way.

The goal of this unit is to pick the right type for time once and for all: **always `timestamptz`**. It sounds paradoxical, but `timestamptz` doesn't store a time zone — it stores a **moment in time** (an instant, essentially UTC), and the zone is applied only on display. That's what makes it safe: the moment is one, and how to show it is the client's concern. This is session-level behavior, not query-level, so the unit is an escape hatch: we drive it with a psql script (`demo.sql`), not via `query.sql` + sqlc.

## timestamptz stores a moment, not a zone

Despite the name, `timestamptz` ("timestamp with time zone") doesn't pack a zone into the value. It normalizes the value to UTC and stores a single instant. When you read it, Postgres shows that instant in the session's current time zone (`SET TIME ZONE` / the `TimeZone` parameter). So the same moment `2025-01-15 09:00:00+00` shows as `12:00+03` in Moscow and as `04:00-05` in New York — but it's the same point on the timeline.

## timestamp without a zone is a trap

`timestamp` (without `tz`) stores wall-clock date-time **without** zone information. Under `SET TIME ZONE` it doesn't shift — because it doesn't know which zone it was written in. For an event (when something happened) that's almost always a mistake: two services in different zones will read the same `09:00` as different moments. `timestamp` is appropriate only where the zone genuinely isn't needed (for example, "alarm time 08:00" as a local rule), and there are few such places in an ordinary application.

## What our code shows

`demo.sql` takes one real instant from the canon — `orders.created_at` of order #1 (`2025-01-15 09:00:00+00`) — and reads it under three zones, changing only `SET TIME ZONE`:

```sql
SET TIME ZONE 'UTC';            SELECT created_at FROM orders WHERE id = 1;
SET TIME ZONE 'Europe/Moscow';  SELECT created_at FROM orders WHERE id = 1;
SET TIME ZONE 'America/New_York'; SELECT created_at FROM orders WHERE id = 1;
```

The value in the database doesn't change — only its display does. Then `demo.sql` shows the trap: under the same `SET TIME ZONE 'Europe/Moscow'`, a `timestamp` literal (without a zone) stays `09:00`, while `timestamptz` shifts to `12:00+03`.

In Go (via pgx) `timestamptz` arrives as `time.Time` — also an instant. Formatting it into local time is the job of the presentation layer (UI/report), not storage. That's why we don't need sqlc here: the lesson is about the session command `SET TIME ZONE`, not a typed query.

## Running it

Bring up the sandbox (from the repo root) and apply the canon:

```sh
docker compose up -d
make lecture L=01-data-types/01-03-date-time-timestamptz T=db-reset
make lecture L=01-data-types/01-03-date-time-timestamptz
```

(`T=run` is the default: it's `psql -f demo.sql`. `run` is an alias for the main demo, as in any escape-hatch unit.)

Output:

```
== Один инстант orders.created_at = 2025-01-15 09:00:00+00 под разными зонами ==

-- SET TIME ZONE 'UTC' :
 id |       created_at       
----+------------------------
  1 | 2025-01-15 09:00:00+00


-- SET TIME ZONE 'Europe/Moscow' (+03):
 id |       created_at       
----+------------------------
  1 | 2025-01-15 12:00:00+03


-- SET TIME ZONE 'America/New_York' (зимой -05):
 id |       created_at       
----+------------------------
  1 | 2025-01-15 04:00:00-05


== Ловушка: timestamp БЕЗ зоны не сдвигается, timestamptz — сдвигается ==
-- при той же SET TIME ZONE 'Europe/Moscow':
  wall_clock_no_tz   |       instant_tz       
---------------------+------------------------
 2025-01-15 09:00:00 | 2025-01-15 12:00:00+03
```

(The demo prints in Russian.) `09:00+00`, `12:00+03`, `04:00-05` are three renderings of **one** moment. And at the bottom you see the type difference: `timestamp` without a zone is stuck at `09:00` (it doesn't know its zone), while `timestamptz` honestly shifted by `+03`. The "12:00 everywhere" St. Petersburg export broke precisely because time was stored without a zone.

## The fence

What we simplified: the zones here are chosen with a fixed winter offset (Moscow `+03` year-round since 2014, New York `-05` in winter) so the output reproduces verbatim. In reality there's daylight saving: in New York in summer it would be `-04`, and the same UTC instant would show an hour differently — Postgres accounts for this by the zone name (`America/New_York`), which is exactly why you should **store the zone name, not a numeric offset**, if the local date of a future event matters to you. And: in production, formatting time for the user lives in the presentation layer (their zone comes from the profile/`Accept-Language`/settings), while the DB and backend operate on UTC instants. We twist `SET TIME ZONE` by hand here only to see the mechanics — in an application you almost never do that.

## Takeaways

- For events store **`timestamptz`** — it holds a moment (UTC); the zone is applied on display.
- `timestamp` without a zone stores wall-clock time with no zone and doesn't shift under `SET TIME ZONE` — a trap for events.
- One instant looks different under different zones — that's normal; the value in the database is one.
- In Go `timestamptz` → `time.Time` (an instant). Format into local time in the presentation layer, not in storage.

Next up — the **01-04 "uuid and uuidv7"** unit: which key to choose — auto-increment, random `gen_random_uuid()` (v4), or PG18 `uuidv7()` with embedded time — and why v7 works as a time-sortable primary key.
