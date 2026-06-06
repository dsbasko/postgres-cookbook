-- demo.sql — когда индексы не помогают и при чём тут индекс по выражению
-- (цель `make run`).
--
-- Escape-hatch-юнит: урок про планы, sqlc неприменим — ведём psql-скриптом.
-- На лабораторном столе accounts_lab (200 000 аккаунтов с e-mail в смешанном
-- регистре) показываем классическую ловушку: обычный индекс по email НЕ
-- помогает запросу `WHERE lower(email) = ...` — функция поверх столбца делает
-- условие non-sargable, и план срывается в Seq Scan. Чинит это индекс по
-- ВЫРАЖЕНИЮ lower(email): запрос обязан совпасть с выражением индекса дословно.
--
-- Вывод детерминирован: (COSTS OFF, TIMING OFF, BUFFERS OFF) оставляет форму
-- плана и фактические строки. Параллелизм выключен. Стол дропается в конце —
-- канон Brew не трогаем.

\set ON_ERROR_STOP on
\pset footer off
SET client_min_messages = warning;
SET max_parallel_workers_per_gather = 0;

DROP TABLE IF EXISTS accounts_lab;
CREATE TABLE accounts_lab (
    email text NOT NULL,
    name  text NOT NULL
);

-- E-mail в СМЕШАННОМ регистре ('User<g>@Brew.example') — как их вводят люди.
-- Логин нормализует регистр через lower() → и натыкается на ловушку ниже.
INSERT INTO accounts_lab (email, name)
SELECT 'User' || g || '@Brew.example', 'name ' || g
FROM generate_series(1, 200000) g;

CREATE INDEX accounts_lab_email_idx ON accounts_lab (email);
ANALYZE accounts_lab;

\echo '== Q1) точное равенство email = ... — обычный индекс работает (Index Scan) =='
EXPLAIN (ANALYZE, COSTS OFF, TIMING OFF, SUMMARY OFF, BUFFERS OFF)
SELECT * FROM accounts_lab WHERE email = 'User150000@Brew.example';

\echo ''
\echo '== Q2) lower(email) = ... с тем же индексом — Seq Scan (условие non-sargable) =='
EXPLAIN (ANALYZE, COSTS OFF, TIMING OFF, SUMMARY OFF, BUFFERS OFF)
SELECT * FROM accounts_lab WHERE lower(email) = 'user150000@brew.example';

\echo ''
\echo '== создаём индекс по ВЫРАЖЕНИЮ lower(email) =='
CREATE INDEX accounts_lab_lower_email_idx ON accounts_lab (lower(email));
ANALYZE accounts_lab;

\echo ''
\echo '== Q3) тот же lower(email) = ... — теперь Index Scan по индексу-выражению =='
EXPLAIN (ANALYZE, COSTS OFF, TIMING OFF, SUMMARY OFF, BUFFERS OFF)
SELECT * FROM accounts_lab WHERE lower(email) = 'user150000@brew.example';

DROP TABLE accounts_lab;
