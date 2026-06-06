-- demo.sql — частичные, покрывающие и уникальные индексы (цель `make run`).
--
-- Escape-hatch-юнит: урок про планы, sqlc неприменим — ведём psql-скриптом.
-- На лабораторном столе orders_lab (200 000 заказов, 1% в статусе pending)
-- показываем два индекса, которые экономят больше обычного:
--
--   A) ЧАСТИЧНЫЙ индекс (... WHERE status = 'pending') индексирует только нужные
--      строки → он крошечный (64 kB против 4408 kB у полного) и обслуживает
--      горячий запрос «разгрести pending по порядку».
--   B) ПОКРЫВАЮЩИЙ индекс ((customer_id) INCLUDE (total)) держит и total прямо в
--      индексе → запрос получает всё из индекса и НЕ ходит в таблицу:
--      Index Only Scan, Heap Fetches: 0 (после VACUUM, который выставляет карту
--      видимости).
--
-- Уникальный индекс разбираем в README (UNIQUE-констрейнт = уникальный индекс).
--
-- Вывод детерминирован: (COSTS OFF, TIMING OFF, BUFFERS OFF) оставляет форму
-- плана и строки; размеры индексов фиксированы данными. VACUUM перед частью B
-- делает Heap Fetches: 0 воспроизводимым. Стол дропается в конце — канон не трогаем.

\set ON_ERROR_STOP on
\pset footer off
SET client_min_messages = warning;
SET max_parallel_workers_per_gather = 0;

DROP TABLE IF EXISTS orders_lab;
CREATE TABLE orders_lab (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,   -- даёт orders_lab_pkey (полный индекс по id)
    customer_id bigint NOT NULL,
    status      text   NOT NULL,
    total       bigint NOT NULL
);

-- 200K заказов: 1000 клиентов, статус почти всегда 'done', 1% — 'pending'.
INSERT INTO orders_lab (customer_id, status, total)
SELECT (g % 1000) + 1,
       CASE WHEN g % 100 = 0 THEN 'pending' ELSE 'done' END,
       (g % 500) * 13
FROM generate_series(1, 200000) g;

-- Частичный индекс: только строки в статусе 'pending'.
CREATE INDEX orders_lab_pending_idx ON orders_lab (id) WHERE status = 'pending';
ANALYZE orders_lab;

\echo '== A1) частичный индекс много меньше полного (PK по всем строкам) =='
SELECT pg_size_pretty(pg_relation_size('orders_lab_pkey'))         AS full_pk_idx,
       pg_size_pretty(pg_relation_size('orders_lab_pending_idx'))  AS partial_idx,
       (pg_relation_size('orders_lab_pkey') > pg_relation_size('orders_lab_pending_idx')) AS partial_is_smaller;

\echo ''
\echo '== A2) частичный индекс обслуживает "разгрести pending по порядку id" =='
EXPLAIN (ANALYZE, COSTS OFF, TIMING OFF, SUMMARY OFF, BUFFERS OFF)
SELECT id, total FROM orders_lab WHERE status = 'pending' ORDER BY id LIMIT 5;

-- Покрывающий индекс: ключ customer_id + INCLUDE (total) лежит прямо в индексе.
CREATE INDEX orders_lab_cust_cover_idx ON orders_lab (customer_id) INCLUDE (total);
VACUUM (ANALYZE) orders_lab;   -- карта видимости all-visible → Heap Fetches: 0

\echo ''
\echo '== B) покрывающий индекс INCLUDE → Index Only Scan, Heap Fetches: 0 =='
EXPLAIN (ANALYZE, COSTS OFF, TIMING OFF, SUMMARY OFF, BUFFERS OFF)
SELECT customer_id, total FROM orders_lab WHERE customer_id = 777;

DROP TABLE orders_lab;
