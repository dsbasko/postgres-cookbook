-- schema.sql — DDL-добавки юнита 02-02 поверх канона Brew.
--
-- Тема — первичный ключ и обязательность колонок: натуральный ключ (бизнес-код
-- как PK) против суррогатного (синтетический id, бизнес-код — отдельный UNIQUE).
-- Две своих таблицы, чтобы канон Brew (байт-совместимый) не трогать.
--
-- DDL идемпотентен (CREATE TABLE IF NOT EXISTS): применяется и на `make gen`
-- (sqlc), и на db-reset (brew.Apply + //schema.sql).

-- shop_natural — ключ натуральный: PRIMARY KEY прямо на бизнес-коде. Обрати
-- внимание: NOT NULL на code мы не писали — PRIMARY KEY навязывает его сам
-- (PK = NOT NULL + UNIQUE). Цена: код И идентифицирует строку, И меняется при
-- ребрендинге — а значит, при переименовании «уезжает» сам ключ.
CREATE TABLE IF NOT EXISTS shop_natural (
    code  TEXT  PRIMARY KEY,
    name  TEXT  NOT NULL
);

-- shop_surrogate — ключ суррогатный: синтетический id владеет identity строки,
-- а бизнес-код живёт отдельной колонкой с UNIQUE. Код можно переименовать, не
-- трогая id: на id ссылаются внешние ключи, и они не ломаются.
CREATE TABLE IF NOT EXISTS shop_surrogate (
    id    BIGINT  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code  TEXT    NOT NULL UNIQUE,
    name  TEXT    NOT NULL
);
