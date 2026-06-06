-- query.sql — протагонист урока. `make gen` → типизированный pgx-код в
-- internal/db/. Имя после `-- name:` → метод; суффикс — форма результата.
--
-- Тема — три «скучных» типа, на которых ломаются приложения: text (держим его,
-- а не char(n) с паддингом), boolean (трёхзначная логика) и NULL. NULL — не
-- значение, а «неизвестно»: сравнение с ним даёт NULL, а не true/false. Полный
-- разбор NULL — в 03-06; здесь только тизер, чтобы ловушка не застала врасплох.

-- name: NullComparison :one
-- Почему нельзя писать WHERE col = NULL: сравнение с NULL даёт не FALSE, а NULL
-- («неизвестно»). Здесь это видно так: (NULL = NULL) — это не TRUE и это NULL.
-- Правильная проверка на отсутствие значения — IS NULL / IS NOT NULL.
SELECT
    ((NULL = NULL) IS NOT TRUE)::boolean  AS eq_null_is_not_true,
    ((NULL = NULL) IS NULL)::boolean      AS eq_null_is_unknown;

-- name: CustomersWithOrders :many
-- LEFT JOIN порождает настоящие NULL: у клиента без заказов order_id = NULL.
-- sqlc видит, что колонка из LEFT JOIN nullable, и типизирует её как pgtype.Int8
-- (Valid=false для NULL) — типобезопасная работа с «отсутствием значения».
SELECT c.id AS customer_id, c.name, o.id AS order_id
FROM customers c
LEFT JOIN orders o ON o.customer_id = c.id::text
ORDER BY c.id, o.id;

-- name: CountStarVsCol :one
-- count(*) считает строки; count(o.id) пропускает строки, где o.id = NULL —
-- клиент без заказов (Карина) в count(o.id) не попадёт.
SELECT
    count(*)     AS rows_total,
    count(o.id)  AS rows_with_order
FROM customers c
LEFT JOIN orders o ON o.customer_id = c.id::text;

-- name: MenuPremiumFlag :many
-- boolean прямо из выражения: предикат base_price > 400 → bool (в Go — bool).
SELECT id, name, (base_price > 400) AS is_premium
FROM drinks
ORDER BY id;

-- name: TextEquality :one
-- text сравнивается по байтам — хвостовой пробел значим ('abc' ≠ 'abc ').
-- char(n) дополняет до длины пробелами, и при сравнении они «съедаются»
-- ('abc' == 'abc  '). Поэтому в курсе мы держим text, а не char(n).
SELECT
    ('abc' = 'abc ')                     AS text_trailing_space_eq,
    ('abc'::char(5) = 'abc  '::char(5))  AS char_padded_eq;
