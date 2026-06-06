-- demo.sql — CREATE INDEX CONCURRENTLY: детерминированные факты (цель `make run`).
--
-- Escape-hatch-юнит: урок про DDL и блокировки, sqlc неприменим — ведём
-- psql-скриптом. Обычный CREATE INDEX берёт блокировку, которая ПРЕГРАЖДАЕТ
-- запись в таблицу на всё время сборки; на горячей таблице это простой кассы.
-- CREATE INDEX CONCURRENTLY строит индекс слабой блокировкой
-- (SHARE UPDATE EXCLUSIVE), не мешая INSERT/UPDATE/DELETE — но за это платит
-- особыми правилами.
--
-- Этот файл показывает ДЕТЕРМИНИРОВАННЫЕ свойства CONCURRENTLY (правило «нельзя
-- в транзакции», валидность индекса, поиск битых индексов). ЖИВУЮ незаблокиро-
-- ванность записи во время сборки показывают session-a.sql / session-b.sql
-- (см. README; сценарий двух сессий интерактивен).
--
-- Лабораторный стол создаётся и дропается здесь же — канон Brew не трогаем,
-- демо идемпотентно. Сообщение об ошибке шага 2 уходит в stderr; в stdout
-- остаётся детерминированный SQLSTATE.

\set VERBOSITY terse
\pset footer off
SET client_min_messages = warning;

DROP TABLE IF EXISTS cic_lab;
CREATE TABLE cic_lab (
    id      bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    payload text   NOT NULL
);
INSERT INTO cic_lab (payload) SELECT 'p' || g FROM generate_series(1, 5000) g;

\echo '== 1) обычный CREATE INDEX можно внутри транзакции (он транзакционный) =='
BEGIN;
CREATE INDEX cic_lab_plain_idx ON cic_lab (payload);
COMMIT;
SELECT 'обычный индекс собран внутри BEGIN/COMMIT' AS result;

\echo ''
\echo '== 2) CREATE INDEX CONCURRENTLY ВНУТРИ транзакции запрещён (ошибка в stderr) =='
\set ON_ERROR_STOP off
BEGIN;
CREATE INDEX CONCURRENTLY cic_lab_conc_idx ON cic_lab (payload);
ROLLBACK;
\echo 'SQLSTATE =' :LAST_ERROR_SQLSTATE '(cannot run inside a transaction block)'
\set ON_ERROR_STOP on

\echo ''
\echo '== 3) CREATE INDEX CONCURRENTLY ВНЕ транзакции — успех, индекс валиден =='
CREATE INDEX CONCURRENTLY cic_lab_conc_idx ON cic_lab (payload);
SELECT indexrelid::regclass AS index, indisvalid
FROM pg_index WHERE indexrelid = 'cic_lab_conc_idx'::regclass;

\echo ''
\echo '== 4) проверка на битые индексы (сорванный CONCURRENTLY оставляет indisvalid=false) =='
SELECT count(*) AS invalid_indexes FROM pg_index WHERE NOT indisvalid;

DROP TABLE cic_lab;
