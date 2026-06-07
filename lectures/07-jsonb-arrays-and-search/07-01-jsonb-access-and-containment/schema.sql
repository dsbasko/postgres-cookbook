-- schema.sql — DDL-добавки ЭТОГО юнита поверх канона Brew (schema/brew.sql).
--
-- Тема — доступ к jsonb и containment. Канонический outbox.payload — тоже jsonb,
-- но он пуст в seed и форма его событий фиксирована. Нам нужна разношёрстная,
-- разреженная структура (у одних заказов есть extras, у других нет ключа milk) —
-- ровно тот случай, ради которого берут jsonb. Делаем СВОЮ лабораторную таблицу
-- order_options_lab, чтобы свободно ставить опыты и не трогать канон.
--
-- DDL применяется двумя путями: sqlc читает его на `make gen`, а демо встраивает
-- через runtime.Caller и накатывает brew.Apply при db-reset. Поэтому он
-- идемпотентен: DROP TABLE IF EXISTS + CREATE + детерминированный seed (фиксиро-
-- ванные id и значения → вывод демо воспроизводится дословно при любом прогоне).

DROP TABLE IF EXISTS order_options_lab;

-- order_options_lab — кастомизация напитка в заказе как jsonb. options хранит
-- то, что по природе бесформенно: размер, молоко, число шотов, список добавок.
-- Заметь: у строки 5 вообще нет ключа milk — jsonb это позволяет, и именно
-- поэтому фильтры по такому полю требуют аккуратности (см. 03-06 про NULL).
CREATE TABLE order_options_lab (
    id       bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    customer text   NOT NULL,
    options  jsonb  NOT NULL
);

INSERT INTO order_options_lab (customer, options) VALUES
    ('Алиса',  '{"size":"L","milk":"oat","shots":2,"extras":["cinnamon","syrup"]}'),
    ('Борис',  '{"size":"M","milk":"cow","shots":1}'),
    ('Карина', '{"size":"S","milk":"oat","shots":1,"extras":["honey"]}'),
    ('Дина',   '{"size":"L","milk":"soy","shots":3,"extras":[]}'),
    ('Егор',   '{"size":"M","shots":2}');
