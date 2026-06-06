-- query.sql — протагонист урока. `make gen` → типизированный pgx-код в
-- internal/db/. Имя после `-- name:` → метод; суффикс — форма результата.
--
-- Тема — подзапросы и выбор между IN и EXISTS. Три формы: scalar (подзапрос как
-- одно значение), IN (значение в наборе), EXISTS (есть ли совпадающая строка). И
-- кульминация — почему «нет среди» пишут через NOT EXISTS, а не NOT IN: ловушка
-- NULL в списке (мы встречали её в 03-06 как урок о NULL; здесь это довод за
-- EXISTS).

-- name: AbovePriceAvg :many
-- Scalar-подзапрос: (SELECT avg(base_price) FROM drinks) возвращает ОДНО число
-- (среднюю цену = 400), и оно подставляется в WHERE как обычное значение.
-- Получаем напитки дороже среднего.
SELECT id, name, base_price
FROM drinks
WHERE base_price > (SELECT avg(base_price) FROM drinks)
ORDER BY base_price;

-- name: DrinksOrdered :many
-- IN-подзапрос: id IN (набор из подзапроса). «Напитки, которые хоть раз
-- заказывали» — те, чей id есть среди order_items.drink_id. Зелёный чай (#5) не
-- заказывали ни разу → его тут не будет.
SELECT id, name
FROM drinks
WHERE id IN (SELECT drink_id FROM order_items)
ORDER BY id;

-- name: CountCustomersWithOrders :one
-- EXISTS — коррелированный подзапрос: для каждого клиента спрашиваем «есть ли
-- ХОТЯ БЫ ОДИН его заказ». EXISTS останавливается на первой найденной строке и
-- не тащит данные наружу — ему важен факт наличия, а не значения. Карина без
-- заказов не пройдёт.
SELECT count(*)
FROM customers c
WHERE EXISTS (
    SELECT 1 FROM orders o WHERE o.customer_id = c.id::text
);

-- name: TruncatePromo :exec
TRUNCATE promo;

-- name: SeedPromo :exec
-- Две акции: на эспрессо (#1) и на всё меню (featured_drink_id = NULL). Этого
-- одного NULL достаточно, чтобы сломать NOT IN ниже.
INSERT INTO promo (title, featured_drink_id) VALUES
    ('Эспрессо дня', 1),
    ('Скидка на всё', NULL);

-- name: CountNotFeaturedNotIn :one
-- Ловушка: id NOT IN (1, NULL). Для напитка с id<>1 это NOT (id=1 OR id=NULL) =
-- NOT (false OR NULL) = NOT (NULL) = NULL → строка НЕ проходит фильтр. Итог 0,
-- хотя «не на акции» напитков явно больше. NULL в списке обнуляет весь NOT IN.
SELECT count(*)
FROM drinks
WHERE id NOT IN (SELECT featured_drink_id FROM promo);

-- name: CountNotFeaturedNotExists :one
-- Правильно: NOT EXISTS. Спрашиваем «нет ли акции на ЭТОТ напиток». Строка promo
-- с featured_drink_id = NULL ни с каким d.id не совпадёт (NULL ни с чем не
-- равен), поэтому никого лишнего не исключает. Итог 4 (пять напитков минус
-- эспрессо #1, который на акции).
SELECT count(*)
FROM drinks d
WHERE NOT EXISTS (
    SELECT 1 FROM promo p WHERE p.featured_drink_id = d.id
);
