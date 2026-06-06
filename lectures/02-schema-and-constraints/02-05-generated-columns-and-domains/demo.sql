-- demo.sql — генерируемые столбцы и домены (цель `make run`).
--
-- Это escape-hatch-юнит. Причина конкретная: PG18 `GENERATED ... VIRTUAL` —
-- настолько свежая фича, что её ещё не понимает парсер sqlc (v1.30.0 падает на
-- `syntax error at or near "VIRTUAL"`). А урок именно про неё. Поэтому ведём его
-- psql-скриптом напрямую — сервер PG18 фичу знает, а кодоген тут и не нужен.
--
-- Демо работает на отдельном лабораторном столе (DROP+CREATE в начале →
-- идемпотентно и воспроизводимо), канон Brew не трогает. ON_ERROR_STOP выключен
-- намеренно: две вставки ниже ДОЛЖНЫ упасть — мы показываем сами ошибки, а не
-- падаем на них. VERBOSITY terse делает текст ошибки однострочным и стабильным.

\set ON_ERROR_STOP off
\set VERBOSITY terse
\pset footer off
-- Глушим NOTICE от DROP ... IF EXISTS (чистый вывод); ERROR это не трогает.
SET client_min_messages = warning;

DROP TABLE IF EXISTS gen_lab CASCADE;
DROP TABLE IF EXISTS dom_lab CASCADE;
DROP DOMAIN IF EXISTS positive_cents CASCADE;

-- Генерируемый столбец = значение, которое БД вычисляет из других колонок этой
-- же строки. STORED считается при записи и лежит на диске; VIRTUAL (PG18)
-- считается на лету при чтении и места не занимает. Выражение одно и то же.
CREATE TABLE gen_lab (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    qty           INT    NOT NULL,
    unit_price    BIGINT NOT NULL,
    total_stored  BIGINT GENERATED ALWAYS AS (qty * unit_price) STORED,
    total_virtual BIGINT GENERATED ALWAYS AS (qty * unit_price) VIRTUAL
);

\echo '== 1) Генерируемый столбец считается из других колонок (qty * unit_price) =='
INSERT INTO gen_lab (qty, unit_price) VALUES (3, 450);
SELECT qty, unit_price, total_stored, total_virtual FROM gen_lab;

\echo ''
\echo '== 2) Как столбец хранится (pg_attribute.attgenerated): s = STORED, v = VIRTUAL =='
SELECT attname AS col, attgenerated AS gen
FROM pg_attribute
WHERE attrelid = 'gen_lab'::regclass AND attgenerated <> ''
ORDER BY attnum;

\echo ''
\echo '== 3) Писать в генерируемый столбец напрямую нельзя (как и в GENERATED ALWAYS id) =='
INSERT INTO gen_lab (qty, unit_price, total_stored) VALUES (1, 100, 999);

\echo ''
\echo '== 4) DOMAIN positive_cents = BIGINT + встроенный CHECK (VALUE > 0) =='
CREATE DOMAIN positive_cents AS BIGINT CHECK (VALUE > 0);
CREATE TABLE dom_lab (
    id     BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    price  positive_cents NOT NULL
);
\echo '-- price = 0 (нарушает CHECK домена):'
INSERT INTO dom_lab (price) VALUES (0);
\echo '-- price = 300 (валидно):'
INSERT INTO dom_lab (price) VALUES (300);
SELECT price FROM dom_lab;

-- Прибираемся: лабораторные объекты сносим, канон Brew остаётся как был.
DROP TABLE IF EXISTS gen_lab CASCADE;
DROP TABLE IF EXISTS dom_lab CASCADE;
DROP DOMAIN IF EXISTS positive_cents CASCADE;
