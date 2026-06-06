-- query.sql — протагонист урока. `make gen` → типизированный pgx-код в
-- internal/db/. Имя после `-- name:` → метод; суффикс — форма результата.
--
-- Тема — трезвая семантика NULL (расплата за тизер из 01-02). В Postgres NULL —
-- это «неизвестно», и логика трёхзначная: сравнение с NULL даёт не true/false, а
-- NULL (UNKNOWN). Отсюда знаменитая ловушка NOT IN со списком, где есть NULL: он
-- молча возвращает «ничего». Плюс три инструмента безопасной работы с NULL:
-- COALESCE, NULLIF, IS DISTINCT FROM.

-- name: NullLogic :one
-- Четыре факта на литералах. Каждое выражение детерминированно и не-NULL (внешний
-- IS NULL / IS NOT DISTINCT / COALESCE сворачивает результат к bool/int).
SELECT
    ((NULL = NULL) IS NULL)                  AS eq_is_null,         -- (=) с NULL → NULL, не true
    (NULL IS NOT DISTINCT FROM NULL)         AS is_not_distinct,    -- NULL-безопасное равенство
    (NULLIF(100, 100) IS NULL)               AS nullif_eq_is_null,  -- NULLIF(a,a) → NULL
    COALESCE(NULL::int, NULL, 42)            AS coalesce_val;       -- первое не-NULL

-- name: TruncateUnavailable :exec
TRUNCATE unavailable;

-- name: SeedUnavailable :exec
-- Список недоступных: напиток #4 (колд брю) и затесавшийся NULL. Этого одного
-- NULL достаточно, чтобы сломать NOT IN ниже.
INSERT INTO unavailable (drink_id) VALUES (4), (NULL);

-- name: CountAvailableNotIn :one
-- Ловушка: id NOT IN (4, NULL). Для любого id это NOT (id=4 OR id=NULL) =
-- NOT (… OR NULL). Если id<>4, получается NOT (NULL) = NULL — строка НЕ проходит
-- фильтр. Итог — 0, хотя доступных напитков явно больше.
SELECT count(*) FROM drinks
WHERE id NOT IN (SELECT drink_id FROM unavailable);

-- name: CountAvailableNotExists :one
-- Правильный способ: NOT EXISTS. Он спрашивает «нет ли совпадающей строки», и
-- NULL в unavailable никого не исключает (NULL ни с чем не совпадает). Итог — 4
-- (пять напитков минус колд брю #4).
SELECT count(*) FROM drinks d
WHERE NOT EXISTS (
    SELECT 1 FROM unavailable u WHERE u.drink_id = d.id
);
