-- schema.sql — DDL-добавки ЭТОГО юнита поверх канона Brew (schema/brew.sql).
--
-- Многотабличный JOIN (чек заказа из 4 таблиц) идёт по каноническим orders ↔
-- customers ↔ order_items ↔ drinks. Self-join — это таблица, соединённая сама с
-- собой; самый наглядный его случай — иерархия со ссылкой на «своего же»: строка
-- ссылается на другую строку той же таблицы. В каноне такой таблицы нет, поэтому
-- заводим маленький лабораторный штат кофейни.
--
-- DDL применяется двумя путями: sqlc читает его на `make gen`, а демо встраивает
-- через runtime.Caller и накатывает brew.Apply при db-reset (идемпотентно).

-- staff — штат кофейни. manager_id — ссылка на id ЭТОЙ же таблицы (бариста
-- подчинён менеджеру), то есть таблица ссылается сама на себя. У самого старшего
-- менеджера руководителя нет → manager_id NULL. id задаём явно (не IDENTITY),
-- чтобы в seed сослаться на конкретный manager_id.
CREATE TABLE IF NOT EXISTS staff (
    id          BIGINT  PRIMARY KEY,
    name        TEXT    NOT NULL,
    role        TEXT    NOT NULL,
    manager_id  BIGINT  REFERENCES staff (id)
);
