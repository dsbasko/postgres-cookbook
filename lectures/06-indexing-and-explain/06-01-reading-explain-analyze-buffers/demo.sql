-- demo.sql — как читать EXPLAIN ANALYZE (цель `make run`).
--
-- Escape-hatch-юнит: урок про чтение плана запроса, sqlc тут не помощник —
-- ведём psql-скриптом. На лабораторном столе events_lab (1 000 000 строк,
-- журнал событий кассы Brew) показываем тот самый контраст, ради которого
-- существуют индексы: один и тот же запрос сначала идёт Seq Scan'ом по всему
-- миллиону строк, а после CREATE INDEX — Index Scan'ом точно в одну строку.
--
-- Почему вывод детерминирован: EXPLAIN с (COSTS OFF, TIMING OFF, BUFFERS OFF)
-- печатает ТОЛЬКО форму плана и фактические строки — без времени и буферов,
-- которые зависят от железа и прогрева кэша (их разбираем в README отдельно, с
-- оговоркой про машину). Параллелизм выключаем (max_parallel_workers_per_gather
-- = 0), чтобы план читался в одну колонку, а не Gather + воркеры.
--
-- Лабораторный стол создаётся и дропается здесь же — канон Brew не трогаем,
-- демо идемпотентно, вывод воспроизводится дословно при любом числе прогонов.

\set ON_ERROR_STOP on
\pset footer off
SET client_min_messages = warning;          -- глушим NOTICE от DROP ... IF EXISTS
SET max_parallel_workers_per_gather = 0;     -- серийный план — читается чище (см. README)

DROP TABLE IF EXISTS events_lab;
CREATE TABLE events_lab (
    ref_no   bigint NOT NULL,    -- сквозной номер события (ищем по нему)
    shop_id  int    NOT NULL,
    kind     text   NOT NULL,
    amount   bigint NOT NULL
);

-- Миллион строк одной командой через generate_series — детерминированно
-- (ref_no = 1..1000000), без random().
INSERT INTO events_lab (ref_no, shop_id, kind, amount)
SELECT g,
       (g % 50) + 1,
       (ARRAY['sale','refund','void','comp'])[(g % 4) + 1],
       (g % 1000) * 10
FROM generate_series(1, 1000000) g;

ANALYZE events_lab;   -- собрать статистику, чтобы планировщик считал честно

\echo '== 1) БЕЗ индекса: запрос идёт Seq Scan по всему миллиону строк =='
EXPLAIN (ANALYZE, COSTS OFF, TIMING OFF, SUMMARY OFF, BUFFERS OFF)
SELECT * FROM events_lab WHERE ref_no = 762312;

\echo ''
\echo '== создаём индекс по ref_no и пересобираем статистику =='
CREATE INDEX events_lab_ref_no_idx ON events_lab (ref_no);
ANALYZE events_lab;

\echo ''
\echo '== 2) С индексом: тот же запрос — Index Scan точно в одну строку =='
EXPLAIN (ANALYZE, COSTS OFF, TIMING OFF, SUMMARY OFF, BUFFERS OFF)
SELECT * FROM events_lab WHERE ref_no = 762312;

DROP TABLE events_lab;
