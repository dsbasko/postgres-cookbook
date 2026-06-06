-- schema.sql — DDL-добавки ЭТОГО юнита поверх канона Brew (schema/brew.sql).
--
-- Scalar/IN/EXISTS-подзапросы показываем на каноне (drinks, order_items,
-- customers). А вот ловушка NOT IN + NULL требует подзапроса, который МОЖЕТ
-- вернуть NULL — в каноне таких nullable-колонок под рукой нет (order_items.
-- drink_id и orders.customer_id оба NOT NULL). Поэтому заводим лабораторную
-- таблицу акций promo, где featured_drink_id НАМЕРЕННО nullable.
--
-- DDL применяется двумя путями: sqlc читает его на `make gen`, а демо встраивает
-- через runtime.Caller и накатывает brew.Apply при db-reset (идемпотентно).

-- promo — акции Brew. featured_drink_id — напиток акции; NULL значит «акция на
-- всё меню» (конкретного напитка нет). Именно этот допустимый NULL ломает
-- NOT IN ниже — и делает наглядным, почему для «нет среди» берут NOT EXISTS.
CREATE TABLE IF NOT EXISTS promo (
    id                 BIGINT  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title              TEXT    NOT NULL,
    featured_drink_id  BIGINT  REFERENCES drinks (id)
);
