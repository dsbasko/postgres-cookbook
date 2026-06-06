-- schema.sql — DDL-добавки ЭТОГО юнита поверх канона Brew (schema/brew.sql).
--
-- В отличие от 00-01/00-04, у этого юнита СВОЯ таблица: канон-таблицы байт-
-- совместимы с kafka-cookbook (customers.id — BIGINT, не uuid), поэтому
-- uuidv7-демо живёт на новой таблице, не на каноне (см. CLAUDE.md: RICH-таблицы).
--
-- Этот DDL применяется двумя путями:
--   * sqlc читает его на этапе `make gen` (чтобы типизировать запросы);
--   * демо встраивает его через //go:embed и накатывает brew.Apply при db-reset.
-- Поэтому он идемпотентен (CREATE TABLE IF NOT EXISTS).

-- loyalty_signups — регистрации в программе лояльности Brew. Ключ — uuidv7():
-- генерируется БД, монотонно растёт во времени (в отличие от случайного v4),
-- поэтому годится как сортируемый по времени первичный ключ. seq (IDENTITY) —
-- независимый счётчик порядка вставки, нужен демо, чтобы проверить монотонность.
CREATE TABLE IF NOT EXISTS loyalty_signups (
    id          UUID         NOT NULL DEFAULT uuidv7() PRIMARY KEY,
    seq         BIGINT       GENERATED ALWAYS AS IDENTITY,
    email       TEXT         NOT NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);
