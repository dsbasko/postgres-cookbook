-- query.sql — протагонист урока. `make gen` → типизированный pgx-код в
-- internal/db/. Имя после `-- name:` → метод; суффикс — форма результата.
--
-- Тема — uuid как ключ: случайный gen_random_uuid() (версия 4) против PG18
-- uuidv7() (версия 7, со встроенным временем). Значения uuid случайны, поэтому
-- печатать их нельзя (вывод не воспроизведётся) — вместо этого показываем
-- ПРОВЕРЯЕМЫЕ свойства: номер версии, наличие встроенного времени и монотонность
-- v7 как сортируемого ключа.

-- name: UUIDFacts :one
-- Детерминированные факты о версиях. v4 — случайный, версия 4, без встроенного
-- времени (uuid_extract_timestamp → NULL). v7 — версия 7, со встроенным
-- временем (uuid_extract_timestamp → не NULL).
SELECT
    uuid_extract_version(gen_random_uuid())::int         AS v4_version,
    uuid_extract_version(uuidv7())::int                  AS v7_version,
    (uuid_extract_timestamp(gen_random_uuid()) IS NULL)::boolean     AS v4_has_no_timestamp,
    (uuid_extract_timestamp(uuidv7()) IS NOT NULL)::boolean          AS v7_has_timestamp;

-- name: TruncateSignups :exec
-- Перед демо чистим таблицу, чтобы вставка трёх строк давала ровно три (вывод
-- воспроизводим независимо от числа прогонов).
TRUNCATE loyalty_signups RESTART IDENTITY;

-- name: InsertSignup :one
-- id присваивает БД из DEFAULT uuidv7(); seq — из IDENTITY. Возвращаем оба, но
-- демо их не печатает (id случаен) — нужны только для проверки порядка.
INSERT INTO loyalty_signups (email) VALUES ($1)
RETURNING id, seq;

-- name: SignupsTimeOrdered :one
-- Совпадает ли порядок по uuidv7-ключу (id) с порядком вставки (seq)? Для v7,
-- сгенерированного по возрастанию времени, — да: монотонный, сортируемый ключ.
SELECT
    bool_and(id_rank = seq_rank)::boolean AS ids_match_insertion_order,
    count(*)::bigint                      AS n
FROM (
    SELECT row_number() OVER (ORDER BY id)  AS id_rank,
           row_number() OVER (ORDER BY seq) AS seq_rank
    FROM loyalty_signups
) t;
