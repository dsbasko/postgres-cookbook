-- query.sql — протагонист урока. `make gen` → типизированный pgx-код в
-- internal/db/. Имя после `-- name:` → метод; суффикс — форма результата.
--
-- Тема — UNIQUE и CHECK. Ключевой сюрприз UNIQUE: по умолчанию NULL ≠ NULL,
-- поэтому несколько NULL проходят; NULLS NOT DISTINCT (PG15) это меняет. CHECK
-- проверяет значение и отбивает нарушителя с SQLSTATE 23514.

-- name: ResetUniqDefault :exec
TRUNCATE uniq_default RESTART IDENTITY;

-- name: ResetUniqNND :exec
TRUNCATE uniq_nnd RESTART IDENTITY;

-- name: ResetCheckDrink :exec
TRUNCATE check_drink RESTART IDENTITY;

-- name: InsertUniqDefaultNull :exec
-- NULL в UNIQUE-колонку: по умолчанию проходит сколько угодно раз (NULL ≠ NULL).
INSERT INTO uniq_default (slot) VALUES (NULL);

-- name: InsertUniqDefaultA :exec
-- Непустое значение: второй такой же → SQLSTATE 23505 (unique_violation).
INSERT INTO uniq_default (slot) VALUES ('A');

-- name: CountUniqDefault :one
SELECT count(*)::bigint AS n FROM uniq_default;

-- name: InsertUniqNNDNull :exec
-- NULL под NULLS NOT DISTINCT: второй NULL уже дубль → SQLSTATE 23505.
INSERT INTO uniq_nnd (slot) VALUES (NULL);

-- name: CountUniqNND :one
SELECT count(*)::bigint AS n FROM uniq_nnd;

-- name: InsertCheckDrink :exec
-- price > 0 и size IN (...) — нарушение любого CHECK → SQLSTATE 23514.
INSERT INTO check_drink (name, price, size) VALUES ($1, $2, $3);
