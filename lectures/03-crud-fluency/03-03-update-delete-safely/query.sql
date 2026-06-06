-- query.sql — протагонист урока. `make gen` → типизированный pgx-код в
-- internal/db/. Имя после `-- name:` → метод; суффикс — форма результата.
--
-- Тема — менять и удалять строки безопасно. Два инструмента «увидеть масштаб»:
-- RETURNING (какие именно строки задело) и :execrows (сколько строк задело).
-- И главный приём безопасности: рискованную запись делаем внутри транзакции —
-- забытый WHERE тогда откатывается, а не превращается в инцидент на проде.

-- name: TruncatePriceLab :exec
-- Перед демо обнуляем таблицу и счётчик IDENTITY — id всегда 1..5, вывод
-- воспроизводим независимо от числа прогонов.
TRUNCATE price_lab RESTART IDENTITY;

-- name: SeedPriceRow :exec
INSERT INTO price_lab (name, category, price) VALUES ($1, $2, $3);

-- name: ListPriceLab :many
SELECT id, name, category, price FROM price_lab ORDER BY id;

-- name: RaiseCategory :many
-- Целевой UPDATE: меняем цену только в одной категории. RETURNING возвращает
-- ровно затронутые строки — видно, что задето именно то, что хотели.
UPDATE price_lab
SET price = price + sqlc.arg(delta)
WHERE category = sqlc.arg(category)
RETURNING id, name, price;

-- name: RaiseAll :execrows
-- «Забыл WHERE»: UPDATE без условия задевает ВСЮ таблицу. Форма :execrows
-- возвращает число затронутых строк — это и есть масштаб, который надо увидеть
-- ДО коммита (в демо мы исполняем это внутри транзакции и откатываем).
UPDATE price_lab
SET price = price + sqlc.arg(delta);

-- name: DeleteCategory :execrows
-- DELETE с условием. :execrows отдаёт число удалённых строк — тоже масштаб.
DELETE FROM price_lab WHERE category = sqlc.arg(category);
