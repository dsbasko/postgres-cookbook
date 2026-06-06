-- demo.sql — время в Postgres глазами разработчика (цель `make run`).
--
-- Это escape-hatch-юнит: урок про SET TIME ZONE — команду уровня СЕССИИ, которая
-- меняет, как клиент ОТОБРАЖАЕТ инстант. Сам момент времени в базе один и тот же;
-- меняется только его текстовое представление. Это поведение psql-сессии, а не
-- SQL-запроса, поэтому урок ведётся psql-скриптом напрямую, без query.sql + sqlc.
--
-- Демо только читает канон Brew (orders.created_at) и литералы — данные не меняет,
-- поэтому идемпотентно и вывод воспроизводится дословно. Часовые пояса выбраны с
-- фиксированным зимним смещением (Москва +03 круглый год, Нью-Йорк зимой -05),
-- чтобы вывод не зависел от текущей даты.

\set ON_ERROR_STOP on
\pset footer off

\echo '== Один инстант orders.created_at = 2025-01-15 09:00:00+00 под разными зонами =='
\echo ''
\echo '-- SET TIME ZONE ''UTC'' :'
SET TIME ZONE 'UTC';
SELECT id, created_at FROM orders WHERE id = 1;

\echo ''
\echo '-- SET TIME ZONE ''Europe/Moscow'' (+03):'
SET TIME ZONE 'Europe/Moscow';
SELECT id, created_at FROM orders WHERE id = 1;

\echo ''
\echo '-- SET TIME ZONE ''America/New_York'' (зимой -05):'
SET TIME ZONE 'America/New_York';
SELECT id, created_at FROM orders WHERE id = 1;

\echo ''
\echo '== Ловушка: timestamp БЕЗ зоны не сдвигается, timestamptz — сдвигается =='
\echo '-- при той же SET TIME ZONE ''Europe/Moscow'':'
SET TIME ZONE 'Europe/Moscow';
SELECT
    '2025-01-15 09:00:00'::timestamp      AS wall_clock_no_tz,
    '2025-01-15 09:00:00+00'::timestamptz AS instant_tz;
