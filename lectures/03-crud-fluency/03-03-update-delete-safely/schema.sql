-- schema.sql — DDL-добавки ЭТОГО юнита поверх канона Brew (schema/brew.sql).
--
-- Тема — безопасные UPDATE/DELETE. Демонстрируем «забытый WHERE» как контролируемую
-- катастрофу, поэтому пишем на СВОЮ лабораторную таблицу price_lab (канон не трогаем),
-- которую демо каждый раз пересоздаёт детерминированно (TRUNCATE + seed).
--
-- DDL применяется двумя путями: sqlc читает его на `make gen`, а демо встраивает
-- через runtime.Caller и накатывает brew.Apply при db-reset. Поэтому он
-- идемпотентен (CREATE TABLE IF NOT EXISTS).

-- price_lab — лабораторный прайс: цена в центах (BIGINT, см. 01-01). На нём
-- безопасно ставить опыты с массовым UPDATE/DELETE.
CREATE TABLE IF NOT EXISTS price_lab (
    id        BIGINT  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name      TEXT    NOT NULL,
    category  TEXT    NOT NULL,
    price     BIGINT  NOT NULL
);
