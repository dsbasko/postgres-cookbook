-- schema/seed.sql — детерминированные демо-данные Brew.
--
-- Цель: после brew.Reset БД всегда в одном и том же состоянии (стабильные id,
-- фиксированные created_at), чтобы вывод демо в README воспроизводился дословно
-- на любой машине и при любом числе прогонов.
--
-- Идемпотентность: сначала TRUNCATE ... RESTART IDENTITY CASCADE обнуляет все
-- таблицы и сбрасывает счётчики IDENTITY/serial, затем идут INSERT'ы. Поэтому
-- повторный прогон не накапливает дубли и не сдвигает id.
--
-- Цены — в минорных единицах (копейках/центах) как BIGINT.

TRUNCATE
    order_items,
    inventory,
    orders,
    outbox,
    processed_outbox_ids,
    drinks,
    articles,
    customers,
    shops
RESTART IDENTITY CASCADE;

-- customers — id BIGINT задаём явно (в каноне нет default для id).
INSERT INTO customers (id, phone, name, email, created_at) VALUES
    (1, '+7-900-000-0001', 'Алиса Иванова',  'alice@brew.example', '2025-01-10 08:00:00+00'),
    (2, '+7-900-000-0002', 'Борис Петров',   'bob@brew.example',   '2025-01-10 08:05:00+00'),
    (3, '+7-900-000-0003', 'Карина Сидорова', 'carol@brew.example', '2025-01-10 08:10:00+00');

-- drinks — меню. id явный (канон, BIGINT PK без default). base_price в центах.
INSERT INTO drinks (id, sku, name, description, category, base_price, stock, created_at, updated_at) VALUES
    (1, 'ESP-01', 'Эспрессо',     'Классический эспрессо, 30 мл',        'coffee', 300, 100, '2025-01-05 07:00:00+00', '2025-01-05 07:00:00+00'),
    (2, 'CAP-01', 'Капучино',     'Эспрессо с молочной пеной',           'coffee', 450,  80, '2025-01-05 07:00:00+00', '2025-01-05 07:00:00+00'),
    (3, 'LAT-01', 'Латте',        'Эспрессо с большим объёмом молока',    'coffee', 480,  75, '2025-01-05 07:00:00+00', '2025-01-05 07:00:00+00'),
    (4, 'CLD-01', 'Колд брю',     'Холодный кофе медленной экстракции',  'cold',   520,  40, '2025-01-05 07:00:00+00', '2025-01-05 07:00:00+00'),
    (5, 'TEA-01', 'Зелёный чай',  'Сенча, листовой',                      'tea',    250,  60, '2025-01-05 07:00:00+00', '2025-01-05 07:00:00+00');

-- articles — блог Brew.
INSERT INTO articles (id, title, body, author, tags, published_at, created_at) VALUES
    (1, 'Почему эспрессо — это база',
        'Эспрессо — фундамент всего кофейного меню: из него собираются капучино, латте и не только.',
        'Алиса Иванова', 'coffee,basics', '2025-01-12 12:00:00+00', '2025-01-11 10:00:00+00'),
    (2, 'Гайд по колд брю',
        'Колд брю — это про время, а не про температуру: 12–16 часов медленной экстракции.',
        'Борис Петров', 'coffee,cold-brew', '2025-01-14 12:00:00+00', '2025-01-13 10:00:00+00');

-- shops — кофейни. id генерируется IDENTITY (после RESTART → 1, 2 по порядку).
INSERT INTO shops (code, name, city, opened_on, created_at) VALUES
    ('BREW-CENTRAL', 'Brew Central', 'Москва',           '2024-09-01', '2024-09-01 09:00:00+00'),
    ('BREW-NORTH',   'Brew North',   'Санкт-Петербург',  '2024-11-15', '2024-11-15 09:00:00+00');

-- orders — заказы. id BIGSERIAL: задаём явно для стабильности, ниже поправим
-- последовательность через setval, чтобы будущие INSERT'ы не словили дубль PK.
-- customer_id — TEXT (канон), кладём id клиента строкой.
INSERT INTO orders (id, customer_id, amount, status, created_at) VALUES
    (1, '1', 10.50, 'paid',    '2025-01-15 09:00:00+00'),
    (2, '2',  3.00, 'created', '2025-01-15 09:30:00+00'),
    (3, '1',  9.60, 'shipped', '2025-01-16 11:00:00+00');

SELECT setval(pg_get_serial_sequence('orders', 'id'), (SELECT MAX(id) FROM orders));

-- order_items — позиции заказов. id генерируется IDENTITY. unit_price берём из
-- drinks.base_price на момент заказа (в реальном меню цена могла измениться).
INSERT INTO order_items (order_id, drink_id, quantity, unit_price) VALUES
    (1, 2, 1, 450),   -- заказ 1: капучино
    (1, 4, 1, 520),   -- заказ 1: колд брю  → итого 970 центов
    (2, 1, 1, 300),   -- заказ 2: эспрессо
    (3, 3, 2, 480);   -- заказ 3: латте ×2  → итого 960 центов

-- inventory — остатки по кофейням. shop_id берём подзапросом по коду, чтобы не
-- завязываться на конкретные значения IDENTITY.
INSERT INTO inventory (shop_id, drink_id, on_hand, updated_at) VALUES
    ((SELECT id FROM shops WHERE code = 'BREW-CENTRAL'), 1, 50, '2025-01-16 08:00:00+00'),
    ((SELECT id FROM shops WHERE code = 'BREW-CENTRAL'), 2, 40, '2025-01-16 08:00:00+00'),
    ((SELECT id FROM shops WHERE code = 'BREW-CENTRAL'), 4, 25, '2025-01-16 08:00:00+00'),
    ((SELECT id FROM shops WHERE code = 'BREW-NORTH'),   1, 30, '2025-01-16 08:00:00+00'),
    ((SELECT id FROM shops WHERE code = 'BREW-NORTH'),   3, 20, '2025-01-16 08:00:00+00');
