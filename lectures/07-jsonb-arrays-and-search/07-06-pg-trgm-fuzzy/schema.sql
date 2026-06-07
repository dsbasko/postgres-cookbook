-- schema.sql — DDL-добавки ЭТОГО юнита поверх канона Brew (schema/brew.sql).
--
-- Тема — нечёткий (fuzzy) поиск через расширение pg_trgm: схожесть по триграммам.
-- Нужны короткие имена для поиска «с опечаткой» — заводим СВОЙ лабораторный стол
-- menu_search_lab. Имена английские: значения similarity — это сравнение наборов
-- триграмм, оно детерминировано и не зависит от локали машины.
--
-- DDL применяется двумя путями: sqlc читает его на `make gen`, а демо встраивает
-- через runtime.Caller и накатывает brew.Apply при db-reset. Поэтому он
-- идемпотентен: CREATE EXTENSION IF NOT EXISTS + DROP TABLE IF EXISTS + seed.

-- pg_trgm — расширение схожести по триграммам (тройкам символов). Даёт оператор
-- % (схоже?), функцию similarity() и классы операторов под GIN/GiST для
-- ускоренного LIKE/ILIKE. Требует прав на CREATE EXTENSION (в песочнице есть).
CREATE EXTENSION IF NOT EXISTS pg_trgm;

DROP TABLE IF EXISTS menu_search_lab;

-- menu_search_lab — позиции меню для поиска по названию.
CREATE TABLE menu_search_lab (
    id   bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name text NOT NULL
);

-- GIN с классом операторов gin_trgm_ops — то, что отличает trgm-индекс от
-- обычного: он индексирует триграммы, поэтому ускоряет и % (similarity), и
-- LIKE/ILIKE '%подстрока%' (обычный B-tree по подстроке в середине бесполезен).
CREATE INDEX menu_search_lab_name_trgm ON menu_search_lab USING gin (name gin_trgm_ops);

INSERT INTO menu_search_lab (name) VALUES
    ('Cappuccino'), ('Espresso'), ('Latte'), ('Cold Brew'),
    ('Flat White'), ('Americano'), ('Macchiato');
