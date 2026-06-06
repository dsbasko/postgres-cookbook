-- demo.sql — B-tree и порядок столбцов в составном индексе (цель `make run`).
--
-- Escape-hatch-юнит: урок про планы, sqlc неприменим — ведём psql-скриптом.
-- На лабораторном столе menu_lab (200 000 строк) с составным индексом
-- (category, price) показываем правило левого префикса: индекс помогает запросу
-- по category и по «category AND price», а запрос по одному price исторически
-- шёл бы Seq Scan'ом — но в PG18 его подхватывает skip-scan (виден по
-- Index Searches > 1).
--
-- Данные сгенерированы так, что category и price НЕзависимы: price = (g%503)+1
-- (503 простое, взаимно просто с 4) → каждый price встречается во всех четырёх
-- категориях. Именно поэтому skip-scan вынужден «перепрыгивать» по категориям.
--
-- Вывод детерминирован: (COSTS OFF, TIMING OFF, BUFFERS OFF) оставляет только
-- форму плана, фактические строки и Index Searches. Параллелизм выключен.
-- Лабораторный стол дропается в конце — канон Brew не трогаем.

\set ON_ERROR_STOP on
\pset footer off
SET client_min_messages = warning;
SET max_parallel_workers_per_gather = 0;

DROP TABLE IF EXISTS menu_lab;
CREATE TABLE menu_lab (
    category text   NOT NULL,    -- низкая кардинальность (4 значения) — лидирующий столбец
    price    bigint NOT NULL,    -- независим от category (см. шапку)
    name     text   NOT NULL
);

INSERT INTO menu_lab (category, price, name)
SELECT (ARRAY['coffee','tea','cold','bakery'])[(g % 4) + 1],
       (g % 503) + 1,
       'item ' || g
FROM generate_series(1, 200000) g;

CREATE INDEX menu_lab_cat_price_idx ON menu_lab (category, price);
ANALYZE menu_lab;

\echo '== Q1) фильтр по ОБОИМ столбцам (category=tea AND price=250) — оба в Index Cond =='
EXPLAIN (ANALYZE, COSTS OFF, TIMING OFF, SUMMARY OFF, BUFFERS OFF)
SELECT * FROM menu_lab WHERE category = 'tea' AND price = 250;

\echo ''
\echo '== Q2) левый префикс: только лидирующий столбец (category=tea) — индекс работает =='
EXPLAIN (ANALYZE, COSTS OFF, TIMING OFF, SUMMARY OFF, BUFFERS OFF)
SELECT * FROM menu_lab WHERE category = 'tea';

\echo ''
\echo '== Q3) только ВТОРОЙ столбец (price=250): до PG18 — Seq Scan, в PG18 — skip-scan =='
EXPLAIN (ANALYZE, COSTS OFF, TIMING OFF, SUMMARY OFF, BUFFERS OFF)
SELECT * FROM menu_lab WHERE price = 250;

DROP TABLE menu_lab;
