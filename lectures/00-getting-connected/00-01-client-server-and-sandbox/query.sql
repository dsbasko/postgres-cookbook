-- query.sql — протагонист урока. SQL пишем руками; `make gen` (sqlc generate)
-- генерирует типизированный pgx-код в internal/db/. Имя после `-- name:` →
-- метод на *db.Queries; суффикс (:one / :many) — форма результата.

-- name: ServerVersion :one
-- Первый запрос к серверу: какая версия Postgres отвечает на том конце сокета.
SELECT version();

-- name: CountDrinks :one
-- Сколько напитков в меню Brew — sanity-check, что seed-данные накатились.
SELECT count(*) FROM drinks;

-- name: ListDrinks :many
-- Меню Brew: сервер хранит строки, мы запрашиваем их по своему запросу.
-- base_price — в минорных единицах (центах), BIGINT.
SELECT id, sku, name, category, base_price
FROM drinks
ORDER BY id;
