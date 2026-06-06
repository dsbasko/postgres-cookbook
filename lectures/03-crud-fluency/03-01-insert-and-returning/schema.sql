-- schema.sql — DDL-добавки ЭТОГО юнита поверх канона Brew (schema/brew.sql).
--
-- Тема — INSERT ... RETURNING. Карту лояльности Brew выдают новому клиенту:
-- приложению сразу нужен сгенерированный id карты и значения колонок по
-- DEFAULT (points, created_at) — без второго SELECT. Демонстрируем это на своей
-- лабораторной таблице (канон трогать не нужно: INSERT в loyalty_cards
-- идемпотентен через TRUNCATE ... RESTART IDENTITY в демо).
--
-- DDL применяется двумя путями: sqlc читает его на `make gen`, а демо встраивает
-- через runtime.Caller и накатывает brew.Apply при db-reset. Поэтому он
-- идемпотентен (CREATE TABLE IF NOT EXISTS).

-- loyalty_cards — карта лояльности. id раздаёт БД (GENERATED ALWAYS AS
-- IDENTITY), points и created_at имеют DEFAULT — их и вернём через RETURNING,
-- не делая отдельный запрос.
CREATE TABLE IF NOT EXISTS loyalty_cards (
    id          BIGINT       GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    customer_id BIGINT       NOT NULL,
    card_no     TEXT         NOT NULL,
    points      INT          NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);
