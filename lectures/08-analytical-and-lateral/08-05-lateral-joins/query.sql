-- query.sql — протагонист урока. `make gen` → типизированный pgx-код в
-- internal/db/. Имя после `-- name:` → метод; суффикс — форма результата.
--
-- Тема — LATERAL. Обычный подзапрос в FROM вычисляется НЕЗАВИСИМО и не видит
-- соседние таблицы из того же FROM. LATERAL снимает этот запрет: подзапрос
-- справа может ссылаться на колонки таблицы слева — как тело цикла «для каждой
-- строки слева». Это даёт top-N на группу одним запросом (без N+1 из приложения)
-- и обобщает DISTINCT ON (04-04): LIMIT 1 → «последний/лучший», LIMIT 3 → top-3.

-- name: TopOrdersPerCustomer :many
-- Top-3 заказа на клиента. Подзапрос t ссылается на c.id (это и есть LATERAL):
-- для каждого клиента он берёт его заказы, нумерует по убыванию суммы и
-- оставляет три. LEFT JOIN LATERAL ... ON true сохраняет клиента без заказов
-- (Карина) — её строки t.* будут NULL, поэтому coalesce(...,'—') для чистого
-- типа string в Go.
SELECT
    c.name,
    coalesce(t.rn::text, '—')    AS rn,
    coalesce(t.cents::text, '—') AS cents,
    coalesce(t.day::text, '—')   AS day
FROM lat_customers_lab c
LEFT JOIN LATERAL (
    SELECT row_number() OVER (ORDER BY o.cents DESC, o.id) AS rn, o.cents, o.day
    FROM lat_orders_lab o
    WHERE o.customer_id = c.id
    ORDER BY o.cents DESC, o.id
    LIMIT 3
) t ON true
ORDER BY c.id, t.rn;

-- name: BiggestOrderPerCustomer :many
-- Тот же приём с LIMIT 1 — самый крупный заказ клиента. Это в точности случай
-- DISTINCT ON (04-04), но через LATERAL он естественно расширяется до top-N
-- (поменяй LIMIT). Снова LEFT JOIN, чтобы Карина не выпала.
SELECT
    c.name,
    coalesce(t.cents::text, '—') AS cents,
    coalesce(t.day::text, '—')   AS day
FROM lat_customers_lab c
LEFT JOIN LATERAL (
    SELECT o.cents, o.day
    FROM lat_orders_lab o
    WHERE o.customer_id = c.id
    ORDER BY o.cents DESC, o.id
    LIMIT 1
) t ON true
ORDER BY c.id;
