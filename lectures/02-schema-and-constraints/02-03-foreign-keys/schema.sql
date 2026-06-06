-- schema.sql — DDL-добавки юнита 02-03 поверх канона Brew.
--
-- Тема — внешние ключи и их поведение при удалении родителя: CASCADE (удалить
-- детей), SET NULL (обнулить ссылку), и дефолт NO ACTION/RESTRICT (запретить
-- удаление, пока есть ссылки). Свои таблицы — канон Brew не трогаем.
--
-- DDL идемпотентен: применяется на `make gen` (sqlc) и на db-reset (brew.Apply).

-- fk_customer — родитель. id за базой (GENERATED ALWAYS).
CREATE TABLE IF NOT EXISTS fk_customer (
    id    BIGINT  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name  TEXT    NOT NULL
);

-- fk_order — ребёнок с ON DELETE CASCADE: удалили клиента — ушли и его заказы.
-- customer_id NOT NULL: заказ без клиента не имеет смысла.
CREATE TABLE IF NOT EXISTS fk_order (
    id           BIGINT  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    customer_id  BIGINT  NOT NULL REFERENCES fk_customer (id) ON DELETE CASCADE,
    note         TEXT    NOT NULL
);

-- fk_review — ребёнок с ON DELETE SET NULL: удалили клиента — отзыв остаётся
-- (он ценен сам по себе), а ссылка обнуляется. Поэтому customer_id здесь
-- NULLABLE — иначе SET NULL нарушил бы NOT NULL.
CREATE TABLE IF NOT EXISTS fk_review (
    id           BIGINT  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    customer_id  BIGINT  REFERENCES fk_customer (id) ON DELETE SET NULL,
    stars        INT     NOT NULL
);

-- fk_drink — родитель с дефолтным поведением FK (NO ACTION ≈ RESTRICT).
CREATE TABLE IF NOT EXISTS fk_drink (
    id    BIGINT  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name  TEXT    NOT NULL
);

-- fk_orderitem — ссылается на fk_drink БЕЗ ON DELETE: значит дефолт NO ACTION,
-- т.е. удалить напиток, пока на него ссылается позиция, нельзя (23503).
CREATE TABLE IF NOT EXISTS fk_orderitem (
    id        BIGINT  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    drink_id  BIGINT  NOT NULL REFERENCES fk_drink (id),
    qty       INT     NOT NULL
);
