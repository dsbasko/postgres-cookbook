-- schema.sql — DDL-добавки ЭТОГО юнита поверх канона Brew (schema/brew.sql).
--
-- Тема — upsert через ON CONFLICT. Нужен арбитр конфликта — UNIQUE/PK; делаем
-- его на СВОЕЙ лабораторной таблице stock_levels (остаток напитка в кофейне),
-- чтобы свободно ставить опыты и не трогать канон.
--
-- DDL применяется двумя путями: sqlc читает его на `make gen`, а демо встраивает
-- через runtime.Caller и накатывает brew.Apply при db-reset. Поэтому он
-- идемпотентен (CREATE TABLE IF NOT EXISTS).

-- stock_levels — остаток напитка (drink_sku) в кофейне (shop_code). Натуральный
-- составной ключ (shop_code, drink_sku) — он же арбитр ON CONFLICT: одна строка
-- на пару «кофейня × напиток».
CREATE TABLE IF NOT EXISTS stock_levels (
    shop_code  TEXT  NOT NULL,
    drink_sku  TEXT  NOT NULL,
    on_hand    INT   NOT NULL,
    PRIMARY KEY (shop_code, drink_sku)
);
