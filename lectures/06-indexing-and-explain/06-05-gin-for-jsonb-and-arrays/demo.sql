-- demo.sql — GIN-индекс для jsonb и массивов (цель `make run`).
--
-- Escape-hatch-юнит: урок про планы, sqlc неприменим — ведём psql-скриптом.
-- На лабораторном столе drink_specs_lab (200 000 спецификаций напитка: jsonb
-- attrs + text[] tags) показываем, чего НЕ умеет B-tree и что берёт на себя GIN.
--
-- B-tree индексирует значение ЦЕЛИКОМ (для =, <, >, диапазонов). Он не отвечает
-- на «содержит ли этот jsonb ключ gift» или «есть ли в массиве элемент limited»
-- — для этого надо смотреть ВНУТРЬ значения. Это и делает GIN (инвертированный
-- индекс): строит список вхождений по каждому элементу/ключу. Оператор
-- containment @> летит по нему.
--
-- Вывод детерминирован: (COSTS OFF, TIMING OFF, BUFFERS OFF) оставляет форму
-- плана и строки. Параллелизм выключен. Стол дропается в конце — канон не трогаем.

\set ON_ERROR_STOP on
\pset footer off
SET client_min_messages = warning;
SET max_parallel_workers_per_gather = 0;

DROP TABLE IF EXISTS drink_specs_lab;
CREATE TABLE drink_specs_lab (
    id    bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    attrs jsonb  NOT NULL,    -- {"milk": ..., "size": ..., возможно "gift": true}
    tags  text[] NOT NULL     -- {coffee} либо {coffee, limited}
);

-- 200K строк; редкий признак (gift / limited) у 0.5% — на нём containment селективен.
INSERT INTO drink_specs_lab (attrs, tags)
SELECT
    jsonb_build_object('milk', (ARRAY['oat','soy','cow','none'])[(g % 4) + 1],
                       'size', (ARRAY['S','M','L'])[(g % 3) + 1])
      || CASE WHEN g % 200 = 0 THEN '{"gift": true}'::jsonb ELSE '{}'::jsonb END,
    CASE WHEN g % 200 = 0 THEN ARRAY['coffee','limited'] ELSE ARRAY['coffee'] END
FROM generate_series(1, 200000) g;
ANALYZE drink_specs_lab;

\echo '== 1) jsonb @> БЕЗ индекса — Seq Scan (B-tree тут не помощник) =='
EXPLAIN (ANALYZE, COSTS OFF, TIMING OFF, SUMMARY OFF, BUFFERS OFF)
SELECT id FROM drink_specs_lab WHERE attrs @> '{"gift": true}';

\echo ''
\echo '== 2) массив @> БЕЗ индекса — тоже Seq Scan =='
EXPLAIN (ANALYZE, COSTS OFF, TIMING OFF, SUMMARY OFF, BUFFERS OFF)
SELECT id FROM drink_specs_lab WHERE tags @> ARRAY['limited'];

\echo ''
\echo '== создаём два GIN-индекса: по attrs (jsonb) и по tags (массив) =='
CREATE INDEX drink_specs_lab_attrs_gin ON drink_specs_lab USING gin (attrs);
CREATE INDEX drink_specs_lab_tags_gin  ON drink_specs_lab USING gin (tags);
ANALYZE drink_specs_lab;

\echo ''
\echo '== 3) jsonb @> С GIN — Bitmap Index Scan по GIN =='
EXPLAIN (ANALYZE, COSTS OFF, TIMING OFF, SUMMARY OFF, BUFFERS OFF)
SELECT id FROM drink_specs_lab WHERE attrs @> '{"gift": true}';

\echo ''
\echo '== 4) массив @> С GIN — Bitmap Index Scan по GIN =='
EXPLAIN (ANALYZE, COSTS OFF, TIMING OFF, SUMMARY OFF, BUFFERS OFF)
SELECT id FROM drink_specs_lab WHERE tags @> ARRAY['limited'];

DROP TABLE drink_specs_lab;
