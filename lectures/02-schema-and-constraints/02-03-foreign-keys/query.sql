-- query.sql — протагонист урока. `make gen` → типизированный pgx-код в
-- internal/db/. Имя после `-- name:` → метод; суффикс — форма результата.
--
-- Тема — ссылочная целостность: FK не даёт сослаться на несуществующего
-- родителя (23503), а ON DELETE решает судьбу детей при удалении родителя:
-- CASCADE — удалить, SET NULL — обнулить ссылку, дефолт NO ACTION — запретить.

-- name: ResetFK :exec
-- CASCADE в TRUNCATE — это про сами таблицы (очистить и связанные), не путать с
-- ON DELETE CASCADE внешнего ключа.
TRUNCATE fk_orderitem, fk_review, fk_order, fk_drink, fk_customer RESTART IDENTITY CASCADE;

-- name: InsertCustomer :one
INSERT INTO fk_customer (name) VALUES ($1) RETURNING id;

-- name: InsertOrder :exec
-- customer_id с несуществующим значением → SQLSTATE 23503 (FK не даёт повиснуть).
INSERT INTO fk_order (customer_id, note) VALUES ($1, $2);

-- name: InsertReview :exec
INSERT INTO fk_review (customer_id, stars) VALUES ($1, $2);

-- name: InsertDrink :one
INSERT INTO fk_drink (name) VALUES ($1) RETURNING id;

-- name: InsertOrderItem :exec
INSERT INTO fk_orderitem (drink_id, qty) VALUES ($1, $2);

-- name: DeleteCustomer :exec
-- Запускает ON DELETE-политики детей: CASCADE удалит заказы, SET NULL обнулит
-- ссылку в отзывах.
DELETE FROM fk_customer WHERE id = $1;

-- name: DeleteDrink :exec
-- Дефолтный FK fk_orderitem → fk_drink (NO ACTION): пока есть ссылка → 23503.
DELETE FROM fk_drink WHERE id = $1;

-- name: CountOrders :one
SELECT count(*)::bigint AS n FROM fk_order;

-- name: CountReviews :one
SELECT
    count(*)::bigint                                   AS total,
    count(*) FILTER (WHERE customer_id IS NULL)::bigint AS null_customer
FROM fk_review;
