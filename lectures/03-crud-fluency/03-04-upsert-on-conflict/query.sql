-- query.sql — протагонист урока. `make gen` → типизированный pgx-код в
-- internal/db/. Имя после `-- name:` → метод; суффикс — форма результата.
--
-- Тема — upsert: «вставь, а если такой ключ уже есть — обнови». INSERT ... ON
-- CONFLICT (...) DO UPDATE делает это одной атомарной командой, безопасной под
-- конкуренцией. EXCLUDED — псевдотаблица с той строкой, которую мы пытались
-- вставить (новые значения). Альтернатива DO NOTHING тихо проглатывает дубль.

-- name: TruncateStock :exec
-- Обнуляем таблицу перед демо — вывод воспроизводим.
TRUNCATE stock_levels;

-- name: UpsertStock :one
-- «Вставь или обнови»: при конфликте по (shop_code, drink_sku) переписываем
-- on_hand значением из EXCLUDED (то, что пытались вставить). RETURNING отдаёт
-- итоговую строку; (xmax <> 0) — известный приём отличить вставку от
-- обновления: у только что вставленной строки xmax = 0, у обновлённой — нет.
INSERT INTO stock_levels (shop_code, drink_sku, on_hand)
VALUES ($1, $2, $3)
ON CONFLICT (shop_code, drink_sku)
DO UPDATE SET on_hand = EXCLUDED.on_hand
RETURNING shop_code, drink_sku, on_hand, (xmax <> 0) AS was_update;

-- name: UpsertIgnore :execrows
-- ON CONFLICT DO NOTHING: при конфликте строка не меняется и не вставляется.
-- :execrows вернёт 1 (вставили) или 0 (конфликт проигнорирован) — это идиома
-- идемпотентной вставки (ср. processed_outbox_ids в каноне: дубли проглатываются).
INSERT INTO stock_levels (shop_code, drink_sku, on_hand)
VALUES ($1, $2, $3)
ON CONFLICT (shop_code, drink_sku) DO NOTHING;

-- name: ListStock :many
SELECT shop_code, drink_sku, on_hand FROM stock_levels
ORDER BY shop_code, drink_sku;
