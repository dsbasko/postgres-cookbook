-- query.sql — протагонист урока. `make gen` → типизированный pgx-код в
-- internal/db/. Имя после `-- name:` → метод; суффикс — форма результата.
--
-- Тема — четыре вида JOIN. Сводим клиентов и их заказы и смотрим, какие строки
-- каждый JOIN сохраняет, а какие выкидывает. customers.id — BIGINT, а
-- orders.customer_id — TEXT (так в каноне Brew), поэтому связываем их с явным
-- приведением c.id::text = o.customer_id.

-- name: InnerCustomersOrders :many
-- INNER JOIN: только пары, где совпадение есть с ОБЕИХ сторон. Клиент без
-- заказов (Карина) сюда не попадёт — у неё нет совпадающей строки в orders.
SELECT c.name AS customer, o.id AS order_id, o.status
FROM customers c
JOIN orders o ON o.customer_id = c.id::text
ORDER BY c.id, o.id;

-- name: LeftCustomersOrders :many
-- LEFT JOIN: сохраняем ВСЕ строки левой таблицы (customers), даже без пары
-- справа. У Карины заказов нет → order_id и status придут NULL (sqlc типизирует
-- их как nullable). Это классический «все клиенты и их заказы, если есть».
SELECT c.name AS customer, o.id AS order_id, o.status
FROM customers c
LEFT JOIN orders o ON o.customer_id = c.id::text
ORDER BY c.id, o.id;

-- name: RightOrdersCustomers :many
-- RIGHT JOIN: зеркало LEFT — сохраняем все строки ПРАВОЙ таблицы (customers).
-- Тот же результат, что у LEFT выше, просто таблицы переставлены местами. RIGHT
-- в коде встречается редко именно потому, что любой RIGHT можно записать как
-- LEFT, поменяв порядок таблиц, — и так читается естественнее.
SELECT c.name AS customer, o.id AS order_id, o.status
FROM orders o
RIGHT JOIN customers c ON o.customer_id = c.id::text
ORDER BY c.id, o.id;

-- name: TruncateCounts :exec
-- Перед демо обнуляем оба листа пересчёта — вывод воспроизводим при любом числе
-- прогонов.
TRUNCATE count_floor, count_storage;

-- name: SeedFloor :exec
-- Зал посчитал напитки {1, 2}.
INSERT INTO count_floor (drink_id, qty) VALUES (1, 10), (2, 5);

-- name: SeedStorage :exec
-- Склад посчитал {2, 4}. Пересечение (2) совпадёт, а 1 и 4 останутся каждый в
-- своём листе — ровно те несовпадения, что покажет FULL JOIN.
INSERT INTO count_storage (drink_id, qty) VALUES (2, 3), (4, 8);

-- name: ReconcileFull :many
-- FULL JOIN: сохраняем несовпавшие строки С ОБЕИХ сторон. Напиток только в зале
-- → storage_qty NULL; только на складе → floor_qty NULL; посчитан в обоих →
-- обе цены. Имя берём из drinks по COALESCE ключа (ключ есть хотя бы с одной
-- стороны). Это типичная сверка двух источников: «покажи всё, что есть хоть
-- где-то, и подсветь расхождения».
SELECT d.name AS drink, f.qty AS floor_qty, s.qty AS storage_qty
FROM count_floor f
FULL JOIN count_storage s ON s.drink_id = f.drink_id
JOIN drinks d ON d.id = COALESCE(f.drink_id, s.drink_id)
ORDER BY d.id;
