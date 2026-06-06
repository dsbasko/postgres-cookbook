-- query.sql — протагонист урока. `make gen` → типизированный pgx-код в
-- internal/db/. Имя после `-- name:` → метод; суффикс — форма результата.
--
-- Тема — PRIMARY KEY (= NOT NULL + UNIQUE) и выбор ключа. Показываем, что PK
-- отвергает и NULL, и дубль, что NOT NULL ловит пустую обязательную колонку, и
-- ключевой контраст: при переименовании бизнес-кода натуральный ключ «уезжает»,
-- а суррогатный id остаётся.

-- name: ResetNatural :exec
TRUNCATE shop_natural;

-- name: ResetSurrogate :exec
TRUNCATE shop_surrogate RESTART IDENTITY;

-- name: InsertNatural :exec
INSERT INTO shop_natural (code, name) VALUES ($1, $2);

-- name: InsertNaturalNullCode :exec
-- NULL в PK-колонку code: PRIMARY KEY навязал NOT NULL → SQLSTATE 23502.
INSERT INTO shop_natural (code, name) VALUES (NULL, $1);

-- name: InsertNaturalNullName :exec
-- NULL в обычную NOT NULL колонку name → SQLSTATE 23502.
INSERT INTO shop_natural (code, name) VALUES ($1, NULL);

-- name: InsertSurrogate :one
INSERT INTO shop_surrogate (code, name) VALUES ($1, $2)
RETURNING id;

-- name: RenameNaturalCode :exec
-- Переименование натурального ключа = смена самого значения PK.
UPDATE shop_natural SET code = $2 WHERE code = $1;

-- name: RenameSurrogateCode :exec
-- Переименование атрибута code; id (identity строки) не трогаем.
UPDATE shop_surrogate SET code = $2 WHERE code = $1;

-- name: NaturalCodeExists :one
SELECT EXISTS (SELECT 1 FROM shop_natural WHERE code = $1)::boolean AS present;

-- name: SurrogateIDByCode :one
SELECT id FROM shop_surrogate WHERE code = $1;
