-- schema.sql — DDL-добавки ЭТОГО юнита поверх канона Brew (schema/brew.sql).
--
-- Тема — два способа хранить «много значений на строку»: нативный массив text[]
-- и нормализованная таблица-связка (junction). Моделируем ОДНИ И ТЕ ЖЕ теги
-- напитков двумя способами и сравниваем запросы. Свои лабораторные столы — чтобы
-- не трогать канон (в каноне теги статей лежат строкой, см. 01-05).
--
-- DDL применяется двумя путями: sqlc читает его на `make gen`, а демо встраивает
-- через runtime.Caller и накатывает brew.Apply при db-reset. Поэтому он
-- идемпотентен: DROP TABLE IF EXISTS + CREATE + детерминированный seed.

DROP TABLE IF EXISTS drink_tags_arr;
DROP TABLE IF EXISTS drink_tags;

-- Денормализованная модель: теги напитка одним массивом text[]. Читается одной
-- строкой, индексируется GIN (для @> / = ANY) — но тег не может нести своих
-- атрибутов и не сослаться внешним ключом на справочник.
CREATE TABLE drink_tags_arr (
    drink_sku text   PRIMARY KEY,
    tags      text[] NOT NULL
);

-- GIN по массиву — чтобы @> / = ANY на большой таблице шли индексом (06-05).
-- На наших четырёх строках планировщик выберет Seq Scan, но индекс — часть модели.
CREATE INDEX drink_tags_arr_gin ON drink_tags_arr USING gin (tags);

INSERT INTO drink_tags_arr (drink_sku, tags) VALUES
    ('ESP-01', ARRAY['coffee','hot','classic']),
    ('CAP-01', ARRAY['coffee','hot','milk']),
    ('CLD-01', ARRAY['coffee','cold','limited']),
    ('TEA-01', ARRAY['tea','hot']);

-- Нормализованная модель: одна строка на пару (напиток, тег). Больше строк и
-- нужен JOIN — зато составной PK гарантирует уникальность пары, можно повесить FK
-- на справочник тегов, добавить тегу свои колонки и тривиально считать частоту.
CREATE TABLE drink_tags (
    drink_sku text NOT NULL,
    tag       text NOT NULL,
    PRIMARY KEY (drink_sku, tag)
);

INSERT INTO drink_tags (drink_sku, tag) VALUES
    ('ESP-01','coffee'), ('ESP-01','hot'), ('ESP-01','classic'),
    ('CAP-01','coffee'), ('CAP-01','hot'), ('CAP-01','milk'),
    ('CLD-01','coffee'), ('CLD-01','cold'), ('CLD-01','limited'),
    ('TEA-01','tea'),    ('TEA-01','hot');
