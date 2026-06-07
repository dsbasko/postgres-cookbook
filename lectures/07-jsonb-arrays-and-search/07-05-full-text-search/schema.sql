-- schema.sql — DDL-добавки ЭТОГО юнита поверх канона Brew (schema/brew.sql).
--
-- Тема — полнотекстовый поиск. Нужен генерируемый tsvector-столбец с весами
-- (заголовок важнее тела) и GIN под него — заводим СВОЙ лабораторный стол
-- kb_articles (база знаний Brew). Контент — английский: конфигурация 'english'
-- встроена, делает стемминг (Porter) и убирает стоп-слова детерминированно, без
-- зависимости от локали машины (важно для воспроизводимого вывода).
--
-- DDL применяется двумя путями: sqlc читает его на `make gen`, а демо встраивает
-- через runtime.Caller и накатывает brew.Apply при db-reset. Поэтому он
-- идемпотентен: DROP TABLE IF EXISTS + CREATE + детерминированный seed.

DROP TABLE IF EXISTS kb_articles;

-- kb_articles — статьи базы знаний. tsv — ГЕНЕРИРУЕМЫЙ столбец: БД сама держит
-- его в синхроне с title/body (не надо триггера). setweight('A') заголовку и
-- ('B') телу: совпадение в заголовке весит больше при ранжировании. || склеивает
-- два tsvector в один. GIN-индекс по tsv делает поиск index-доступным (06-05).
CREATE TABLE kb_articles (
    id    bigint   GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title text     NOT NULL,
    body  text     NOT NULL,
    tsv   tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('english', title), 'A') ||
        setweight(to_tsvector('english', body),  'B')
    ) STORED
);

CREATE INDEX kb_articles_tsv_gin ON kb_articles USING gin (tsv);

INSERT INTO kb_articles (title, body) VALUES
    ('Espresso basics', 'Espresso is the base for brewing milk drinks like cappuccino and latte.'),
    ('Cold brew guide', 'Cold brew is about time, not temperature: brewing for sixteen hours.'),
    ('Milk steaming',   'Steaming milk creates microfoam for a cappuccino.'),
    ('Tea selection',   'Green tea and herbal infusions for non-coffee guests.');
