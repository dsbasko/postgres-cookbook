-- query.sql — протагонист урока. `make gen` → типизированный pgx-код в
-- internal/db/. Имя после `-- name:` → метод; суффикс — форма результата.
--
-- Тема — как достать ровно нужные строки в нужном порядке и листать их
-- постранично. WHERE/ORDER/LIMIT — основа; затем два способа пагинации: OFFSET
-- (простой, но дорогой на глубоких страницах) и keyset (по «курсору» из
-- последней строки — летит по индексу).

-- name: FilterMenu :many
-- WHERE/ORDER/LIMIT: отбираем категорию, сортируем по цене, берём не больше N.
-- Полный порядок здесь не критичен (одна категория, печатаем как есть).
SELECT id, name, base_price
FROM drinks
WHERE category = sqlc.arg(category)
ORDER BY base_price
LIMIT sqlc.arg(page_size);

-- name: PageByOffset :many
-- Пагинация через OFFSET: «пропусти offset строк, отдай следующие N». Просто, но
-- сервер всё равно вычисляет и отбрасывает первые offset строк — на глубоких
-- страницах это растёт линейно (см. README, «Заборчик»). ORDER BY обязан быть
-- полным (цена + id как tie-break), иначе строки с равной ценой «плавают».
SELECT id, name, base_price
FROM drinks
ORDER BY base_price DESC, id DESC
LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(skip);

-- name: PageByKeyset :many
-- Keyset-пагинация: вместо «пропусти N» — «дай строки ПОСЛЕ вот этой». Курсор —
-- значения сортировки последней строки прошлой страницы (after_price, after_id).
-- Сравнение кортежей (base_price, id) < (after_price, after_id) ровно повторяет
-- порядок ORDER BY base_price DESC, id DESC. Первую страницу берём с «сторожевым»
-- курсором (заведомо больше любой строки). По индексу (base_price, id) это
-- index range scan без отбрасывания — глубина страницы не дорожает.
SELECT id, name, base_price
FROM drinks
WHERE (base_price, id) < (sqlc.arg(after_price), sqlc.arg(after_id))
ORDER BY base_price DESC, id DESC
LIMIT sqlc.arg(page_size);
