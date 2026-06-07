-- schema.sql — DDL-добавки ЭТОГО юнита поверх канона Brew (schema/brew.sql).
--
-- Тема — субитоги и общий итог за один проход: GROUPING SETS, ROLLUP, CUBE.
-- Нужна маленькая «таблица фактов» продаж в разрезах магазин × категория, по
-- которой удобно считать подытоги. Канон для этого не приспособлен — берём свою
-- лабораторную таблицу sales_fact_lab с фиксированными числами. Канон не трогаем.
--
-- Идемпотентно: DROP TABLE IF EXISTS + CREATE + фиксированный seed → вывод
-- демо воспроизводится дословно при любом прогоне.

DROP TABLE IF EXISTS sales_fact_lab;

-- sales_fact_lab — выручка (в центах) в разрезе «магазин × категория». Четыре
-- факта (2 магазина × 2 категории) — ровно столько, чтобы подытоги считались
-- в уме и было видно, какие строки добавляет ROLLUP, CUBE и GROUPING SETS.
CREATE TABLE sales_fact_lab (
    id       bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    shop     text   NOT NULL,
    category text   NOT NULL,
    cents    bigint NOT NULL
);

INSERT INTO sales_fact_lab (shop, category, cents) VALUES
    ('Central', 'coffee', 1000),
    ('Central', 'tea',     300),
    ('North',   'coffee',  700),
    ('North',   'tea',     200);
