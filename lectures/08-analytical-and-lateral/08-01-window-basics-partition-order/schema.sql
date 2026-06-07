-- schema.sql — DDL-добавки ЭТОГО юнита поверх канона Brew (schema/brew.sql).
--
-- Тема — основы оконных функций: PARTITION BY, ORDER BY и running total. Канон
-- Brew засеян всего тремя заказами — для running total на клиента этого мало,
-- а главное, нужен чистый ряд «покупка за покупкой», где видно, как накопленная
-- сумма растёт. Поэтому берём СВОЮ лабораторную таблицу purchases_lab с
-- детерминированными данными — канон не трогаем.
--
-- DDL применяется двумя путями: sqlc читает его на `make gen`, а демо встраивает
-- через runtime.Caller и накатывает brew.Apply при db-reset. Поэтому он
-- идемпотентен: DROP TABLE IF EXISTS + CREATE + фиксированный seed → вывод
-- демо воспроизводится дословно при любом прогоне.

DROP TABLE IF EXISTS purchases_lab;

-- purchases_lab — журнал покупок: одна строка = одна покупка клиента в день.
-- cents — сумма покупки в минорных единицах (как drinks.base_price в каноне).
-- Порядок INSERT задаёт id (GENERATED ALWAYS AS IDENTITY) → стабильный tie-break.
CREATE TABLE purchases_lab (
    id       bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    customer text   NOT NULL,
    day      date   NOT NULL,
    cents    bigint NOT NULL
);

INSERT INTO purchases_lab (customer, day, cents) VALUES
    ('Алиса',  '2025-02-01', 300),
    ('Борис',  '2025-02-02', 250),
    ('Алиса',  '2025-02-03', 450),
    ('Борис',  '2025-02-04', 480),
    ('Алиса',  '2025-02-05', 520),
    ('Карина', '2025-02-01', 480),
    ('Карина', '2025-02-06', 300);
