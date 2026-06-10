-- query.sql — протагонист урока. SQL пишем руками; `make gen` (sqlc generate)
-- генерирует типизированный pgx-код в internal/db/. Имя после `-- name:` →
-- метод на *db.Queries; суффикс (:one / :many) — форма результата.

-- name: ServerVersion :one
-- Первый контакт: какая версия Postgres отвечает на том конце сокета.
SELECT version();

-- name: BrewWorld :many
-- Перепись мира Brew: сколько строк лежит в каждой таблице канона после seed.
-- ord фиксирует порядок (UNION ALL без ORDER BY его не гарантирует).
SELECT entity, n FROM (
    SELECT 1 AS ord, 'customers'::text     AS entity, count(*) AS n FROM customers
    UNION ALL SELECT 2, 'drinks',               count(*) FROM drinks
    UNION ALL SELECT 3, 'articles',             count(*) FROM articles
    UNION ALL SELECT 4, 'orders',               count(*) FROM orders
    UNION ALL SELECT 5, 'outbox',               count(*) FROM outbox
    UNION ALL SELECT 6, 'processed_outbox_ids', count(*) FROM processed_outbox_ids
    UNION ALL SELECT 7, 'shops',                count(*) FROM shops
    UNION ALL SELECT 8, 'order_items',          count(*) FROM order_items
    UNION ALL SELECT 9, 'inventory',            count(*) FROM inventory
) w
ORDER BY ord;
