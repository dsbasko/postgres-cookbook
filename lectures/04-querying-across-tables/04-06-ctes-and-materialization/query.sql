-- query.sql — протагонист урока. `make gen` → типизированный pgx-код в
-- internal/db/. Имя после `-- name:` → метод; суффикс — форма результата.
--
-- Тема — CTE (Common Table Expression, секция WITH): именованный временный
-- результат, на который ссылается основной запрос. CTE разбивает сложный запрос
-- на читаемые шаги-«кирпичи» и позволяет переиспользовать промежуточный результат.
-- Плюс — материализация: когда Postgres вычисляет CTE отдельно (фенс), а когда
-- встраивает его в основной запрос.

-- name: CustomerSpend :many
-- CTE-конвейер из двух шагов. order_totals: сумма каждого заказа из позиций
-- (order_items). per_customer: сворачиваем заказы по клиенту. Основной запрос
-- подставляет имя клиента. Без CTE это был бы вложенный подзапрос в подзапросе —
-- здесь же читается сверху вниз, шаг за шагом.
WITH order_totals AS (
    SELECT
        o.id                                  AS order_id,
        o.customer_id                         AS customer_id,
        sum(oi.quantity * oi.unit_price)::bigint AS cents
    FROM orders o
    JOIN order_items oi ON oi.order_id = o.id
    GROUP BY o.id, o.customer_id
),
per_customer AS (
    SELECT
        t.customer_id        AS customer_id,
        count(*)             AS orders,
        sum(t.cents)::bigint AS spent
    FROM order_totals t
    GROUP BY t.customer_id
)
SELECT c.name AS customer, p.orders AS orders, p.spent AS spent
FROM per_customer p
JOIN customers c ON c.id::text = p.customer_id
ORDER BY p.spent DESC;

-- name: OrderShareOfTotal :many
-- CTE, использованный ДВАЖДЫ: order_totals читается и в FROM, и в scalar-
-- подзапросе (общий итог). Когда CTE ссылаются более одного раза, Postgres по
-- умолчанию его МАТЕРИАЛИЗУЕТ — вычисляет один раз и переиспользует, а не считает
-- заново на каждую ссылку. Здесь пишем AS MATERIALIZED явно (для одной ссылки
-- умолчание иное — встраивание; противоположный рычаг — NOT MATERIALIZED).
WITH order_totals AS MATERIALIZED (
    SELECT
        o.id                                  AS order_id,
        sum(oi.quantity * oi.unit_price)::bigint AS cents
    FROM orders o
    JOIN order_items oi ON oi.order_id = o.id
    GROUP BY o.id
)
SELECT
    order_id,
    cents,
    round(100.0 * cents / (SELECT sum(cents) FROM order_totals), 1)::text AS pct
FROM order_totals
ORDER BY order_id;
