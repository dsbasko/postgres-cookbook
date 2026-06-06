-- demo.sql — мышление миграций: какой ALTER мгновенен, а какой переписывает
-- таблицу (цель `make run`).
--
-- Это escape-hatch-юнит: урок про DDL и его физическую цену — наблюдаемую через
-- relfilenode (физический «файл» таблицы). relfilenode меняется ТОЛЬКО когда
-- Postgres переписывает таблицу целиком; если ALTER лишь правит метаданные,
-- файл прежний. Это и есть наш индикатор «мгновенно vs переписывание».
--
-- Работаем на лабораторном столе (DROP+CREATE → идемпотентно), канон не трогаем.
-- ON_ERROR_STOP включён: тут всё должно проходить — мы измеряем цену операций,
-- а не ловим ошибки.

\set ON_ERROR_STOP on
\pset footer off
-- Глушим NOTICE от DROP ... IF EXISTS (чистый вывод).
SET client_min_messages = warning;

DROP TABLE IF EXISTS alter_lab CASCADE;
CREATE TABLE alter_lab (
    id    BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    n     INT    NOT NULL,
    name  TEXT   NOT NULL
);
INSERT INTO alter_lab (n, name) SELECT g, 'row ' || g FROM generate_series(1, 1000) g;

\echo '== 1) ADD COLUMN с константным DEFAULT — мгновенно (только метаданные) =='
SELECT pg_relation_filenode('alter_lab') AS fn \gset before1_
ALTER TABLE alter_lab ADD COLUMN status TEXT NOT NULL DEFAULT 'active';
SELECT pg_relation_filenode('alter_lab') AS fn \gset after1_
SELECT (:before1_fn = :after1_fn) AS filenode_unchanged;

\echo ''
\echo '== 2) ALTER COLUMN ... TYPE int -> bigint — таблица ПЕРЕПИСана (новый relfilenode) =='
SELECT pg_relation_filenode('alter_lab') AS fn \gset before2_
ALTER TABLE alter_lab ALTER COLUMN n TYPE bigint;
SELECT pg_relation_filenode('alter_lab') AS fn \gset after2_
SELECT (:before2_fn = :after2_fn) AS filenode_unchanged;

\echo ''
\echo '== 3) ADD CONSTRAINT CHECK ... NOT VALID — мгновенно (старые строки не сканируются) =='
ALTER TABLE alter_lab ADD CONSTRAINT n_positive CHECK (n > 0) NOT VALID;
SELECT convalidated AS validated_after_not_valid FROM pg_constraint WHERE conname = 'n_positive';

\echo ''
\echo '== 4) VALIDATE CONSTRAINT — отдельный шаг (не блокирует запись) =='
ALTER TABLE alter_lab VALIDATE CONSTRAINT n_positive;
SELECT convalidated AS validated_after_validate FROM pg_constraint WHERE conname = 'n_positive';

DROP TABLE IF EXISTS alter_lab CASCADE;
