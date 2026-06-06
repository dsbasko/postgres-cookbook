# 02-03 — foreign keys (ON DELETE CASCADE / SET NULL)

A "delete my account" request landed at Brew. A developer ran `DELETE FROM customers WHERE id = 42` — and the app fell apart in three places at once. That customer's orders were left dangling with a reference to a nonexistent owner; the "my reviews" page crashed because a review pointed into the void; and an attempt to also clean up the menu hit an error, because a drink was still referenced by line items of old orders. All three problems are about one thing: what happens to the **referencing** rows when the one they reference disappears.

The goal of this unit is to push those rules into the schema via a `FOREIGN KEY` and its `ON DELETE` policy. An FK does two things. First, it **forbids referencing** a nonexistent parent — a "dangling" reference is rejected with `SQLSTATE 23503`. Second, through `ON DELETE` it sets the fate of children when the parent is deleted: `CASCADE` — delete them along with it, `SET NULL` — null out the reference (the child stays), and the default `NO ACTION` (≈ `RESTRICT`) — forbid the delete entirely while references exist. These are business decisions, and they belong in the DDL.

## The FK as a guardian of referential integrity

`customer_id BIGINT NOT NULL REFERENCES fk_customer(id)` is a promise from the DB: you can't put a value into `customer_id` that doesn't exist in `fk_customer.id`. An attempt to insert an order with `customer_id = 999` when there's no such customer is rejected with `foreign_key_violation` (`23503`). The FK likewise catches deleting a parent that's still referenced (if the policy forbids it). Integrity is guaranteed by the schema — you can't bypass it with a race between "checked that the customer exists" and "inserted the order."

## ON DELETE: CASCADE, SET NULL, RESTRICT

When a parent is deleted, each referencing FK has its own policy:

- `ON DELETE CASCADE` — delete children along with the parent. Fits "weak" entities that make no sense without an owner: an order without a customer is garbage, let it cascade away.
- `ON DELETE SET NULL` — keep the child but null out the reference. Fits when the child is valuable on its own: a coffee review stays (its text is useful), it just becomes anonymous. Requires the FK column to be **NULLABLE** — otherwise `SET NULL` would violate `NOT NULL`.
- the default `NO ACTION` / `RESTRICT` — forbid deleting the parent while it's referenced. This is the default guard: don't accidentally knock a menu drink out from under live orders. Want to delete it — deal with the references first.

## What our code shows

One parent (`fk_customer`) and three children with different policies; plus a `fk_drink` ← `fk_orderitem` pair with a default FK (DDL in `schema.sql`):

```sql
customer_id BIGINT NOT NULL REFERENCES fk_customer (id) ON DELETE CASCADE   -- fk_order
customer_id BIGINT          REFERENCES fk_customer (id) ON DELETE SET NULL  -- fk_review (NULLABLE!)
drink_id    BIGINT NOT NULL REFERENCES fk_drink (id)                        -- fk_orderitem: default NO ACTION
```

`main.go` first tries a dangling reference (`customer_id = 999` → `23503`), then creates a customer with two orders and one review and deletes them — and counts what's left:

```go
queries.DeleteCustomer(ctx, custID)        // triggers the children's policies
orders, _ := queries.CountOrders(ctx)      // CASCADE → 0
rev, _ := queries.CountReviews(ctx)        // SET NULL → review alive, customer_id IS NULL
```

Finally — the default policy: a drink is referenced by a line item, and `DeleteDrink` is rejected with `23503`. All errors are printed as `SQLSTATE` (the code is deterministic, the text is not).

## Running it

```sh
docker compose up -d
make lecture L=02-schema-and-constraints/02-03-foreign-keys T=db-reset
make lecture L=02-schema-and-constraints/02-03-foreign-keys
```

Output:

```
1) FK блокирует «висящую» ссылку:
   заказ с customer_id = 999 (нет такого клиента) → отклонён: SQLSTATE 23503 (foreign_key_violation)
2) Завели клиента id=1: его заказов (ON DELETE CASCADE) = 2, отзывов (ON DELETE SET NULL) = 1
3) DELETE клиента id=1:
   ON DELETE CASCADE → заказы удалены каскадом: осталось 0
   ON DELETE SET NULL → отзыв жив, ссылка обнулена: отзывов 1, из них customer_id IS NULL: 1
4) ON DELETE по умолчанию (NO ACTION / RESTRICT):
   пока на напиток id=1 ссылается позиция заказа, DELETE напитка → отклонён: SQLSTATE 23503
```

(The demo prints in Russian.) The dangling reference was batted away (`23503`). After deleting the customer, their two orders cascaded away (`осталось 0`), while the review stayed — but without an author now (`customer_id IS NULL`). The same `23503` protected the menu: a drink referenced by a live line item can't be deleted. Three lines of DDL replaced three branches of manual logic in the app.

## The fence

What we simplified: `CASCADE` looks convenient — "deleted the customer, everything related cleaned itself up" — but in production it's a double-edged tool your DBA watches closely. A cascade can silently wipe far more than you expected (deleting one row drags tens of thousands in child tables — a long lock and bloated WAL), and it hurts auditing: data vanishes without a trace. Often it's safer to use `RESTRICT` + an explicit "soft delete" (`deleted_at`) in the app, so deletion is deliberate and reversible. With `SET NULL`, remember the NULLABLE column and that the app must now cope with an "orphaned" reference. And: an FK isn't free — it's checked on every insert/delete and needs an index on the referencing side, otherwise deleting a parent triggers a seq scan over the children (indexes under FKs — module 06). The rule: every `ON DELETE` policy is a recorded business decision; choose it deliberately, not by reflexively slapping `CASCADE` everywhere.

## Takeaways

- `FOREIGN KEY ... REFERENCES` forbids referencing a nonexistent parent — a dangling reference → `SQLSTATE 23503`.
- `ON DELETE CASCADE` — delete children with the parent (for entities meaningless without an owner).
- `ON DELETE SET NULL` — keep the child, null the reference (for self-valuable data); the FK column must be NULLABLE.
- the default `NO ACTION`/`RESTRICT` — forbid deleting the parent while references are live; this is the safe default.
- `CASCADE` is convenient but dangerous (silently wipes a lot, breaks auditing) — often `RESTRICT` + soft-delete is better; an FK needs an index on the child side.

Next up — the **02-04 "UNIQUE and CHECK (NULLS NOT DISTINCT)"** unit: two more declarative constraints — uniqueness (and the treachery of `NULL` within it, plus the PG feature `NULLS NOT DISTINCT`) and value checks via `CHECK`.
