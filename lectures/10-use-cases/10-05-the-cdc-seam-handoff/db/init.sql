-- db/init.sql — артефакт CDC-эстафеты. Это тот самый init.sql, который мы
-- отдаём на сторону kafka-cookbook: он БАЙТ-СОВМЕСТИМ с её
-- 09-use-cases/04-pg-to-elasticsearch/db/init.sql. Совместимость не косметика —
-- благодаря ей Debezium из Kafka-курса читает наши drinks/articles/customers
-- без переписывания схемы. Менять имена/типы колонок здесь нельзя (см. правило
-- байт-совместимости канона в CLAUDE.md; защищено тестом).
--
-- Источник CDC: три таблицы, которые поедут в Elasticsearch как индексы.
-- drinks — меню напитков Brew (full-text search по названию + описанию),
-- articles — статьи блога Brew о кофе (поиск по title + body), customers —
-- справочник клиентов для join'ов (Sink не делает join, мы его не делаем —
-- это место для отдельной search-experience-лекции). REPLICA IDENTITY FULL
-- нужен Debezium'у для UPDATE/DELETE, иначе before-payload обрезается до PK
-- и Sink не видит изменения.

CREATE TABLE IF NOT EXISTS drinks (
    id          BIGINT       PRIMARY KEY,
    sku         TEXT         NOT NULL,
    name        TEXT         NOT NULL,
    description TEXT         NOT NULL,
    category    TEXT         NOT NULL,
    base_price  BIGINT       NOT NULL,
    stock       INT          NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

ALTER TABLE drinks REPLICA IDENTITY FULL;

CREATE TABLE IF NOT EXISTS articles (
    id          BIGINT       PRIMARY KEY,
    title       TEXT         NOT NULL,
    body        TEXT         NOT NULL,
    author      TEXT         NOT NULL,
    tags        TEXT         NOT NULL DEFAULT '',
    published_at TIMESTAMPTZ NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

ALTER TABLE articles REPLICA IDENTITY FULL;

CREATE TABLE IF NOT EXISTS customers (
    id          BIGINT       PRIMARY KEY,
    phone       TEXT         NOT NULL,
    name        TEXT         NOT NULL,
    email       TEXT         NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

ALTER TABLE customers REPLICA IDENTITY FULL;

-- Publication заранее: явный список таблиц вместо publication.autocreate.
-- Так видно, что именно стримится. Удалить таблицу из publication
-- = `ALTER PUBLICATION dbz_publication DROP TABLE <name>`.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_publication WHERE pubname = 'dbz_publication') THEN
        CREATE PUBLICATION dbz_publication FOR TABLE drinks, articles, customers;
    END IF;
END $$;
