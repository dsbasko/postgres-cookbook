-- schema.sql — DDL-добавки юнита 02-04 поверх канона Brew.
--
-- Тема — ещё два декларативных ограничения: UNIQUE (и коварство NULL в нём,
-- плюс PG15-фишка NULLS NOT DISTINCT) и CHECK (проверка значения). Свои
-- таблицы — канон Brew не трогаем.
--
-- DDL идемпотентен: применяется на `make gen` (sqlc) и db-reset (brew.Apply).

-- uniq_default — UNIQUE по умолчанию: два NULL считаются РАЗНЫМИ (в SQL
-- NULL ≠ NULL), поэтому несколько NULL-строк проходят. Непустые значения
-- уникальны как обычно.
CREATE TABLE IF NOT EXISTS uniq_default (
    id    BIGINT  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    slot  TEXT,
    UNIQUE (slot)
);

-- uniq_nnd — UNIQUE NULLS NOT DISTINCT (PG15+): здесь два NULL считаются
-- ОДИНАКОВЫМИ, поэтому второй NULL уже дубль. Это «единственный NULL»-инвариант,
-- который раньше приходилось городить partial unique index'ом.
CREATE TABLE IF NOT EXISTS uniq_nnd (
    id    BIGINT  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    slot  TEXT,
    UNIQUE NULLS NOT DISTINCT (slot)
);

-- check_drink — CHECK как проверка значения прямо в схеме: цена строго
-- положительна, размер — только из набора. Колонки NOT NULL — чтобы demo не
-- путал «NULL прошёл сквозь CHECK» (CHECK пропускает NULL!) с самим CHECK.
CREATE TABLE IF NOT EXISTS check_drink (
    id     BIGINT  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name   TEXT    NOT NULL,
    price  BIGINT  NOT NULL CHECK (price > 0),
    size   TEXT    NOT NULL CHECK (size IN ('small', 'medium', 'large'))
);
