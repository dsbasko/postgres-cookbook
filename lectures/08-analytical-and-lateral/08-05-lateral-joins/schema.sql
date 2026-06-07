-- schema.sql — DDL-добавки ЭТОГО юнита поверх канона Brew (schema/brew.sql).
--
-- Тема — LATERAL-join: подзапрос в FROM, который МОЖЕТ ссылаться на строки
-- левой таблицы (обычный подзапрос в FROM так не умеет). Классическое
-- применение — top-N на группу (top-3 заказа на клиента) одним запросом, без
-- N+1. Чтобы было что показывать, нужно несколько заказов на клиента и клиент
-- БЕЗ заказов (для LEFT JOIN LATERAL). Канон с тремя заказами скуден — берём
-- свои лабораторные таблицы. Канон не трогаем.
--
-- Идемпотентно: DROP TABLE IF EXISTS + CREATE + фиксированный seed → вывод
-- демо воспроизводится дословно при любом прогоне.

DROP TABLE IF EXISTS lat_orders_lab;
DROP TABLE IF EXISTS lat_customers_lab;

CREATE TABLE lat_customers_lab (
    id   bigint PRIMARY KEY,
    name text   NOT NULL
);

-- Карина (id 3) — без единого заказа: на ней проверим, что LEFT JOIN LATERAL
-- сохраняет клиента (CROSS JOIN LATERAL бы её уронил).
INSERT INTO lat_customers_lab (id, name) VALUES
    (1, 'Алиса'),
    (2, 'Борис'),
    (3, 'Карина');

CREATE TABLE lat_orders_lab (
    id          bigint PRIMARY KEY,
    customer_id bigint NOT NULL REFERENCES lat_customers_lab (id),
    cents       bigint NOT NULL,
    day         date   NOT NULL
);

-- Алиса — 4 заказа (top-3 отсечёт самый дешёвый, 280), Борис — 2, Карина — 0.
INSERT INTO lat_orders_lab (id, customer_id, cents, day) VALUES
    (1, 1, 300, '2025-03-01'),
    (2, 1, 450, '2025-03-02'),
    (3, 1, 520, '2025-03-03'),
    (4, 1, 280, '2025-03-04'),
    (5, 2, 250, '2025-03-01'),
    (6, 2, 480, '2025-03-02');
