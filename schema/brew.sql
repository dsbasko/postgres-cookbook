-- schema/brew.sql — канон схемы Brew (общий baseline для всех юнитов курса).
--
-- Две группы таблиц:
--
--   1. CANON (байт-совместимый с kafka-cookbook). Шесть таблиц — orders,
--      outbox, processed_outbox_ids, drinks, articles, customers — определены
--      ДОСЛОВНО так же, как в init.sql соседнего Kafka-курса. Эта совместимость
--      не косметика: capstone 10-05 («CDC seam handoff») публикует ровно эти
--      таблицы в логическую репликацию, и Debezium из kafka-cookbook читает их
--      без переписывания схемы. Любое переименование колонки здесь ломает
--      эстафету — поэтому колонки канона не трогаем (см. CLAUDE.md, правило
--      байт-совместимости).
--
--   2. RICH (наши таблицы). shops, order_items, inventory — добавлены для
--      богатых реляционных примеров (JOIN, LATERAL, оконные функции). Они НЕ
--      участвуют в CDC-handoff, поэтому здесь мы свободны: современные идиомы
--      PG18 (GENERATED ALWAYS AS IDENTITY и т.п.) демонстрируем именно тут, а
--      не на канон-таблицах.
--
-- Весь DDL идемпотентен (IF NOT EXISTS / повторный ALTER REPLICA IDENTITY) —
-- brew.Reset/Apply можно гонять сколько угодно раз.

-- ──────────────────────────────────────────────────────────────────────────
-- CANON (байт-совместимо с kafka-cookbook — НЕ переименовывать колонки)
-- ──────────────────────────────────────────────────────────────────────────

-- orders — заказ Brew. Обрати внимание: customer_id здесь TEXT, а не FK на
-- customers.id BIGINT — так в каноне; в реальном handoff заказы и справочник
-- клиентов едут как независимые потоки CDC.
CREATE TABLE IF NOT EXISTS orders (
    id          BIGSERIAL    PRIMARY KEY,
    customer_id TEXT         NOT NULL,
    amount      NUMERIC      NOT NULL,
    status      TEXT         NOT NULL DEFAULT 'created',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- outbox — transactional outbox: событие пишется в одной транзакции с заказом,
-- relay вычитывает неопубликованные строки и публикует их.
CREATE TABLE IF NOT EXISTS outbox (
    id            BIGSERIAL    PRIMARY KEY,
    aggregate_id  TEXT         NOT NULL,
    topic         TEXT         NOT NULL,
    payload       JSONB        NOT NULL,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    published_at  TIMESTAMPTZ  NULL
);

-- Partial index по неопубликованным — основной запрос relay'я летит по нему.
-- На пусто → быстрый seq scan; на разогретой outbox с миллионом записей —
-- index scan по короткому списку.
CREATE INDEX IF NOT EXISTS outbox_unpublished_idx
    ON outbox (id) WHERE published_at IS NULL;

-- Dedup-таблица на стороне consumer'а. PRIMARY KEY (outbox_id) +
-- INSERT ON CONFLICT DO NOTHING поглощает дубли, прилетевшие после
-- crash'а relay'я между publish и UPDATE published_at.
CREATE TABLE IF NOT EXISTS processed_outbox_ids (
    outbox_id     BIGINT       PRIMARY KEY,
    processed_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- drinks — меню напитков Brew. base_price хранится в минорных единицах
-- (копейках/центах) как BIGINT — деньги целыми, без float. REPLICA IDENTITY
-- FULL нужен Debezium'у, чтобы в before-payload UPDATE/DELETE были все колонки,
-- а не только PK.
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

-- articles — статьи блога Brew о кофе (full-text search по title + body).
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

-- customers — справочник клиентов. id BIGINT (а не uuid) — так в каноне;
-- uuidv7 демонстрируем на новых таблицах, не здесь.
CREATE TABLE IF NOT EXISTS customers (
    id          BIGINT       PRIMARY KEY,
    phone       TEXT         NOT NULL,
    name        TEXT         NOT NULL,
    email       TEXT         NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

ALTER TABLE customers REPLICA IDENTITY FULL;

-- ──────────────────────────────────────────────────────────────────────────
-- RICH (наши таблицы для богатых примеров — вне CDC-канона)
-- ──────────────────────────────────────────────────────────────────────────

-- shops — кофейни сети Brew. GENERATED ALWAYS AS IDENTITY — современная замена
-- serial: колонкой управляет БД, случайный явный INSERT id отвергается.
CREATE TABLE IF NOT EXISTS shops (
    id          BIGINT       GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code        TEXT         NOT NULL UNIQUE,
    name        TEXT         NOT NULL,
    city        TEXT         NOT NULL,
    opened_on   DATE         NOT NULL DEFAULT CURRENT_DATE,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- order_items — позиции заказа: связка orders ↔ drinks (many-to-many). FK на
-- orders с ON DELETE CASCADE (удалили заказ — ушли его позиции), на drinks —
-- RESTRICT по умолчанию (напиток из меню не удалить, пока он в заказах).
CREATE TABLE IF NOT EXISTS order_items (
    id          BIGINT       GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id    BIGINT       NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    drink_id    BIGINT       NOT NULL REFERENCES drinks (id),
    quantity    INT          NOT NULL DEFAULT 1 CHECK (quantity > 0),
    unit_price  BIGINT       NOT NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS order_items_order_id_idx ON order_items (order_id);

-- inventory — остатки напитка в конкретной кофейне. Натуральный составной
-- ключ (shop_id, drink_id): одна строка на пару.
CREATE TABLE IF NOT EXISTS inventory (
    shop_id     BIGINT       NOT NULL REFERENCES shops (id) ON DELETE CASCADE,
    drink_id    BIGINT       NOT NULL REFERENCES drinks (id) ON DELETE CASCADE,
    on_hand     INT          NOT NULL DEFAULT 0 CHECK (on_hand >= 0),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (shop_id, drink_id)
);
