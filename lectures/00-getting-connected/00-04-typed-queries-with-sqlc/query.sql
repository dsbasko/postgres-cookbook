-- query.sql — протагонист урока. SQL пишем руками; `make gen` (sqlc generate)
-- генерирует типизированный pgx-код в internal/db/. Имя после `-- name:` →
-- метод на *db.Queries; суффикс (:one / :many / :exec) — форма результата.
--
-- Главное здесь — параметр $1. В 00-03 мы передавали его в pool.Query и сами
-- разбирали строки через rows.Scan. Тут sqlc выводит ИМЯ и ТИП параметра из
-- схемы: для `WHERE category = $1` он видит, что drinks.category — text, и
-- генерирует метод с аргументом `category string`. Перепутать тип или порядок
-- колонок становится ошибкой компиляции, а не молчаливым багом в рантайме.

-- name: ListDrinksByCategory :many
-- :many → []ListDrinksByCategoryRow. $1 (category) типизирован из схемы как string.
SELECT id, sku, name, category, base_price
FROM drinks
WHERE category = $1
ORDER BY id;

-- name: GetDrinkBySKU :one
-- :one → одна строка (pgx вернёт ErrNoRows, если совпадения нет). $1 = sku.
SELECT id, sku, name, category, base_price
FROM drinks
WHERE sku = $1;

-- name: CountDrinksByCategory :one
-- :one со скаляром → int64. Тот же $1 (category), но результат — одно число.
SELECT count(*)
FROM drinks
WHERE category = $1;
