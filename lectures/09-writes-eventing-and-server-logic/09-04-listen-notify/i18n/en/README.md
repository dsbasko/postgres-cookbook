# 09-04 — LISTEN / NOTIFY: the database pushes a notification

The Brew kitchen wants to see new orders in real time: placed — a card pops up on
the barista's screen at once. You poll the queue from 09-02 for this, a `SELECT`
once a second. It works — but in the morning chat comes Ruslan's report.

> **Ruslan (in chat, 08:05):** Order cards pop up late. The shift calls out what's
> ready by voice, like it's the nineties. The baristas aren't slow — the screen is.

You decide the poll is too rare and bump the frequency in the code.

> **You:** And if we poll more often — will the cards show up in time?

The cards do show up faster. And right after, in the chat — Pavel.

> **Pavel (in chat):** i see your poll. empty selects, every second, per location.
> "nothing new". a database isn't free.

Both extremes of polling are now plain: poll too often and you burn the database,
poll too rarely and the card is late. We want the opposite: let the database push
the signal first, the moment an order is placed, so the app doesn't have to prod
it. That is exactly what the `LISTEN` / `NOTIFY` pair gives — a publish-subscribe
bus built into Postgres.

## The mechanics: a channel, NOTIFY, and a listener

One side subscribes to a named channel: `LISTEN brew_events`. The other sends a
message into it: `NOTIFY brew_events, 'payload'` or, more conveniently from
code/a trigger, the function `pg_notify('brew_events', 'payload')`. Every
connection currently listening on the channel receives the payload instantly — no
polling, no delay.

Most often `NOTIFY` is fired not by hand but from a **trigger**: "as soon as a row
lands in the table — send a notification". In our demo an `AFTER INSERT` trigger
on `notify_lab` calls `pg_notify('brew_events', ...)` with compact json about the
new row. The application doesn't have to remember to send something after every
insert — the database does it.

On the Go side the listener is a **dedicated connection**. `LISTEN` is bound to a
specific backend, and you must read notifications from the same `conn` that ran
`LISTEN`. In `pgx` this is `conn.WaitForNotification(ctx)` — a blocking wait for
the next notification (with a timeout via context). That is exactly why the unit is
raw `pgx` (an escape hatch): `WaitForNotification` lives at the connection level,
and sqlc has no such method.

## Caveat 1: NOTIFY is transactional — it waits for COMMIT

The main thing a developer must know: `NOTIFY` **is held until COMMIT**. If the
trigger fired inside a transaction, the notification won't go out until the
transaction commits; if it rolls back, there is no notification at all. This is
good news (no "phantom" signals about things that didn't happen in the database)
but also a trap: the listener will see nothing while the writer keeps the
transaction open. In the demo we insert a row inside a transaction, wait 400 ms —
silence; we `COMMIT` — and only now does the payload arrive. On a timeline:

```
Writer (one transaction)             Listener (LISTEN brew_events)
  BEGIN
  INSERT → trigger pg_notify  ····►   held back, not visible yet
  ... 400 ms ...                      waits... silence
  COMMIT ─────────────────────────►   payload arrives INSTANTLY

  and if a rollback instead of COMMIT:
  ROLLBACK ───────────────────────►   no notification AT ALL
```

## Caveat 2: at-most-once — no listener, lost signal

The second trap matters more than the first. `NOTIFY` **is not stored**. If no one
is listening on the channel at the moment of sending, the notification simply
vanishes — it won't be queued or delivered later. This is **at-most-once**:
subscribe late, miss it. In the demo we `UNLISTEN`, insert a row (the notification
flies into the void), `LISTEN` again, and wait — nothing. The signal is lost for
good.

Hence the direct contrast with the outbox from 09-03. The outbox is rows in a
table; they survive a restart and are delivered at least once. `NOTIFY` is a
fleeting nudge "hey, take a look" that exists only while a listener is present. So
they are often combined: `NOTIFY` wakes a relay/worker immediately, and `outbox`
provides reliability — woken by the signal OR by a timer, it drains everything
that has accumulated.

Two columns side by side — when to use which:

| Axis | `LISTEN`/`NOTIFY` | `outbox` (09-03) |
|---|---|---|
| Storage | not stored — a fleeting nudge | a row in a table, survives a restart |
| Guarantee | at-most-once (no listener → loss) | at-least-once (the relay reads on) |
| Transactionality | waits for `COMMIT`, rollback → no signal | fact and event commit atomically |
| Latency | instant, no polling | one relay-cycle delay |
| Size | payload ≤ 8000 bytes | row / jsonb size |
| Role | "wake up" signal | reliable delivery |

> **Dmitry:** NOTIFY wakes, the outbox guarantees. A signal, not delivery.

## What our code shows

This is a raw `pgx` unit. The listener takes a dedicated connection and
subscribes:

```go
lc, _ := pool.Acquire(ctx)        // a dedicated connection for the listener
defer lc.Release()
listener := lc.Conn()
listener.Exec(ctx, "LISTEN brew_events")
```

The wait with a timeout is encapsulated in `waitNotify`: arrived — returns the
payload; timed out — returns "nothing" (both caveats rest on this):

```go
n, err := conn.WaitForNotification(wctx)
if errors.Is(err, context.DeadlineExceeded) {
    return nil, nil // no notification — a normal outcome
}
```

The trigger that fires `pg_notify` is created in `setupLab` (`AFTER INSERT` →
`pg_notify('brew_events', json_build_object('id', NEW.id, 'name', NEW.name))`).

## Running it

```sh
docker compose up -d
make lecture L=09-writes-eventing-and-server-logic/09-04-listen-notify T=db-reset
make lecture L=09-writes-eventing-and-server-logic/09-04-listen-notify
```

`T=run` is the default and can be omitted. From inside the unit directory it is
shorter: `make db-reset`, then `make run`.

```
1) notify_lab + триггер AFTER INSERT → pg_notify('brew_events', ...) готовы.
2) LISTEN brew_events (выделенное соединение слушает канал).

3) Транзакционность: INSERT внутри транзакции — уведомление ждёт COMMIT.
   до COMMIT: ждём 400ms... уведомления нет (NOTIFY придержан до COMMIT).
   COMMIT.
   после COMMIT: получено уведомление, payload = {"id" : 1, "name" : "Эспрессо"}

4) At-most-once: если никто не слушает в момент NOTIFY — уведомление теряется.
   UNLISTEN brew_events; INSERT 'Латте' (NOTIFY летит в пустоту).
   LISTEN brew_events снова; ждём 400ms... уведомления нет (потеряно, NOTIFY не хранится).
```

Scenario 3 shows transactionality: before `COMMIT` — silence (timeout), after — the
payload arrives instantly. Scenario 4 shows at-most-once: while the channel was
unlistened, the "Латте" notification vanished, and a second `LISTEN` can no longer
retrieve it.

## The fence

- **Payload ≤ 8000 bytes.** Sending the whole object there is a bad idea — send an
  **identifier** ("order #42 changed") and let the subscriber read the body with a
  plain `SELECT`.
- **Under load the bus serializes.** Identical notifications collapse (Postgres
  dedupes identical payloads within a transaction), and in very high-throughput
  scenarios the `NOTIFY` bus itself goes through a shared lock — it is not a
  replacement for a broker.
- **A listener keeps a connection busy** for the entire wait, and under
  transaction pooling (PgBouncer, see 10-04) `LISTEN` breaks outright —
  notifications arrive on the wrong backend. A listener needs a direct session-mode
  connection to the database, not a connection from a transaction pool; how to
  arrange that in production is a question for your connection infrastructure.
- **The rule:** `NOTIFY` is a **"wake up" signal**, not a data-delivery channel.
  Build reliability on `outbox`/a table, and use `NOTIFY` to avoid polling it for
  nothing.

## Takeaways

`LISTEN` / `NOTIFY` is a pub/sub bus built into Postgres: subscribe to a channel,
get a nudge as soon as someone (often a trigger via `pg_notify`) writes into it; in
pgx this is `conn.WaitForNotification` on a dedicated connection. Two caveats define
where it applies: `NOTIFY` is **transactional** (waits for COMMIT, rollback — no
notification) and **at-most-once** (no listener at send time — the signal is lost, it
is not stored). So `NOTIFY` is a lightweight "wake up", and reliable delivery stays
with `outbox` (09-03); the two combine naturally. And mind the 8 KB limit and the
incompatibility with transaction pooling.

Next — more on triggers themselves, and on what server-side logic costs you: a
`BEFORE` trigger for `updated_at`, an `AFTER` audit trigger with the old and new
values, the `IMMUTABLE/STABLE/VOLATILE` classification of functions — and an honest
"when NOT to put logic in the database" section. That is 09-05, the module's finale.
