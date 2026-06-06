-- query.sql — протагонист урока. SQL пишем руками; `make gen` (sqlc generate)
-- генерирует типизированный pgx-код в internal/db/. Имя после `-- name:` →
-- метод на *db.Queries; суффикс (:one / :many) — форма результата.
--
-- Тема урока — числа и деньги. Главный вывод: float для денег НЕ годится
-- (0.1 + 0.2 ≠ 0.3), а в приложении деньги удобнее всего держать целыми в
-- минорных единицах (центах) как BIGINT — точно и ложится в Go int64.

-- name: FloatVsNumeric :one
-- Классическая ловушка: в double precision 0.1 + 0.2 не равно 0.3 (двоичное
-- представление десятичных дробей неточно). В numeric — равно. float_sum
-- остаётся float8 (в Go это float64 — увидишь «хвост»), numeric_sum приводим к
-- text только ради аккуратной печати (в Go numeric — это pgtype.Numeric).
SELECT
    (0.1::float8 + 0.2::float8)::float8           AS float_sum,
    (0.1::numeric + 0.2::numeric)::text           AS numeric_sum,
    (0.1::float8 + 0.2::float8 = 0.3::float8)      AS float_eq_03,
    (0.1::numeric + 0.2::numeric = 0.3::numeric)  AS numeric_eq_03;

-- name: MenuPriced :many
-- Меню Brew. base_price — BIGINT в центах: целое число, без float-погрешностей.
-- В рубли.копейки разворачиваем уже в приложении (base_price/100 и %100).
SELECT id, name, base_price
FROM drinks
ORDER BY id;

-- name: OrderTotalCents :one
-- Итог заказа целиком в центах: sum по позициям. COALESCE на случай пустого
-- заказа, ::bigint — потому что sum() над bigint возвращает numeric.
SELECT coalesce(sum(quantity * unit_price), 0)::bigint AS total_cents
FROM order_items
WHERE order_id = $1;
