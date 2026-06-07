-- schema.sql — DDL-добавки ЭТОГО юнита поверх канона Brew (schema/brew.sql).
--
-- Тема — ранжирующие оконные функции (row_number / rank / dense_rank), top-N на
-- группу и ntile. Чтобы показать, чем эти три функции отличаются, нужны ничьи
-- (одинаковые значения в порядке сортировки) и хотя бы три категории — канон с
-- его пятью напитками без продаж для этого скуден. Берём свою лабораторную
-- таблицу drink_sales_lab с продажами по категориям, где ничьи расставлены
-- намеренно. Канон не трогаем.
--
-- Идемпотентно: DROP TABLE IF EXISTS + CREATE + фиксированный seed → вывод
-- демо воспроизводится дословно при любом прогоне.

DROP TABLE IF EXISTS drink_sales_lab;

-- drink_sales_lab — продано штук напитка за период. Внутри coffee специально
-- две ничьи на 120 (Капучино/Эспрессо) — на них видно расхождение rank и
-- dense_rank, а Раф на 90 после ничьи показывает «дырку» в нумерации rank.
CREATE TABLE drink_sales_lab (
    id       bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    category text   NOT NULL,
    drink    text   NOT NULL,
    units    int    NOT NULL
);

INSERT INTO drink_sales_lab (category, drink, units) VALUES
    ('coffee', 'Латте',    150),
    ('coffee', 'Капучино', 120),
    ('coffee', 'Эспрессо', 120),
    ('coffee', 'Раф',       90),
    ('cold',   'Колд брю',  70),
    ('cold',   'Фраппе',    40),
    ('tea',    'Сенча',     50),
    ('tea',    'Матча',     30);
