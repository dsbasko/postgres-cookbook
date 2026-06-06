-- schema.sql — DDL-добавки ЭТОГО юнита поверх канона Brew (schema/brew.sql).
--
-- inner/left/right показываем на каноне (customers ↔ orders): у клиента Карина
-- заказов нет — она и есть «несовпавшая» строка. Но FULL JOIN раскрывается
-- только когда несовпадения есть с ОБЕИХ сторон, а в каноне такого нет (каждый
-- заказ ссылается на существующего клиента). Поэтому FULL демонстрируем на паре
-- лабораторных «листов пересчёта» остатков — утренний и вечерний счёт, где
-- какие-то напитки попали только в один из листов.
--
-- DDL применяется двумя путями: sqlc читает его на `make gen`, а демо встраивает
-- через runtime.Caller и накатывает brew.Apply при db-reset (идемпотентно).

-- count_floor / count_storage — два листа пересчёта остатков (зал и склад).
-- Один напиток может быть посчитан только в одном листе — это и есть
-- несовпадения, ради которых нужен FULL JOIN.
CREATE TABLE IF NOT EXISTS count_floor (
    drink_id  BIGINT  PRIMARY KEY REFERENCES drinks (id),
    qty       INT     NOT NULL
);

CREATE TABLE IF NOT EXISTS count_storage (
    drink_id  BIGINT  PRIMARY KEY REFERENCES drinks (id),
    qty       INT     NOT NULL
);
