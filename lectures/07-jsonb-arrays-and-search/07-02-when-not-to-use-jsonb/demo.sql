-- demo.sql — когда НЕ нужен jsonb (цель `make run`).
--
-- Escape-hatch-юнит: урок про физическую и семантическую цену jsonb, sqlc тут
-- ни при чём — ведём psql-скриптом, чтобы показать pg_column_size (физика) и
-- SQLSTATE отбитых ограничений (семантика).
--
-- Две истории на лабораторном столе menu_doc_lab (одна карточка напитка):
--   1) write-amplification — изменить одно поле внутри jsonb = переписать ВЕСЬ
--      документ; обычная колонка пишет 8 байт, тот же número внутри doc — сотни;
--   2) потеря per-field ограничений — колонка с CHECK/типом отбивает мусор
--      (23514 / 22P02), а тот же мусор внутри jsonb проглатывается молча.
--
-- Вывод детерминирован: pg_column_size на фиксированном документе стабилен, мы
-- печатаем SQLSTATE (а не машинозависимый текст ошибки). Стол дропается в конце
-- — канон Brew не трогаем. Текст ошибок уходит в stderr, в stdout — SQLSTATE.

\set VERBOSITY terse
\pset footer off
SET client_min_messages = warning;

DROP TABLE IF EXISTS menu_doc_lab;

-- price_cents — обычная колонка (тип + CHECK). doc — та же карточка целиком в
-- jsonb (так соблазняют сделать, «чтобы гибко»). Сравним два мира на одной строке.
CREATE TABLE menu_doc_lab (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    price_cents bigint NOT NULL CHECK (price_cents > 0),
    doc         jsonb  NOT NULL
);

INSERT INTO menu_doc_lab (price_cents, doc) VALUES
  (450, jsonb_build_object(
     'sku','CAP-01','name','Капучино','category','coffee','price',450,
     'description','Эспрессо с молочной пеной, классическая пропорция 1:1:1',
     'nutrition',    jsonb_build_object('kcal',120,'protein',6,'fat',6,'carbs',9),
     'sizes',        jsonb_build_array('S','M','L'),
     'milk_options', jsonb_build_array('cow','oat','soy','almond','lactose-free'),
     'allergens',    jsonb_build_array('milk'),
     'i18n',         jsonb_build_object('en','Cappuccino','de','Cappuccino','fr','Cappuccino')
  ));

\echo '== 1) write-amplification: байты на одно поле — колонка против jsonb =='
-- 8 байт колонки против сотен байт документа. Чтобы поменять цену внутри doc,
-- jsonb_set отдаёт НОВЫЙ полный документ — переписывается весь объём, не одно поле.
SELECT pg_column_size(price_cents) AS price_column_bytes,
       pg_column_size(doc)         AS doc_bytes
FROM menu_doc_lab WHERE id = 1;

SELECT pg_column_size(jsonb_set(doc, '{price}', '999')) AS doc_after_one_field_change_bytes
FROM menu_doc_lab WHERE id = 1;

\echo ''
\echo '== 2) потеря ограничений: колонка отбивает мусор, jsonb — нет =='
-- Колонка price_cents защищена типом и CHECK: отрицательное → 23514,
-- не-число → 22P02. Печатаем SQLSTATE (текст ошибки — в stderr).
\set ON_ERROR_STOP off
INSERT INTO menu_doc_lab (price_cents, doc) VALUES (-5, '{}');
\echo 'колонка price_cents = -5      → SQLSTATE' :LAST_ERROR_SQLSTATE '(CHECK price_cents > 0)'
INSERT INTO menu_doc_lab (price_cents, doc) VALUES ('banana', '{}');
\echo 'колонка price_cents = banana  → SQLSTATE' :LAST_ERROR_SQLSTATE '(invalid input for bigint)'
\set ON_ERROR_STOP on

\echo ''
\echo '== 3) тот же мусор ВНУТРИ jsonb проходит молча (ни типа, ни CHECK) =='
UPDATE menu_doc_lab SET doc = doc || '{"price": -5}'      WHERE id = 1;
UPDATE menu_doc_lab SET doc = doc || '{"price": "banana"}' WHERE id = 1;
SELECT doc ->> 'price' AS doc_price_now,
       price_cents     AS column_price_still
FROM menu_doc_lab WHERE id = 1;

DROP TABLE menu_doc_lab;
