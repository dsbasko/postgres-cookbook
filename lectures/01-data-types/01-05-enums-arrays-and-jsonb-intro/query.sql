-- query.sql — протагонист урока. `make gen` → типизированный pgx-код в
-- internal/db/. Имя после `-- name:` → метод; суффикс — форма результата.
--
-- Тема — три «контейнерных» типа: enum (упорядочен по объявлению), массивы
-- (text[] с операторами @> / = ANY) и jsonb (полуструктурированные данные). Это
-- ВВЕДЕНИЕ: глубокий разбор jsonb, GIN-индексов и полнотекстового поиска — в
-- модуле 07; здесь только знакомство и когда какой контейнер уместен.

-- name: EnumOrder :one
-- enum сравнивается и сортируется по порядку ОБЪЯВЛЕНИЯ значений
-- (small < medium < large), а НЕ по алфавиту (иначе было бы large < medium <
-- small). Это и есть смысл enum: упорядоченный конечный набор.
SELECT
    ('small'::drink_size < 'large'::drink_size)  AS small_lt_large,
    ('large'::drink_size < 'small'::drink_size)  AS large_lt_small;

-- name: TagsAsArray :many
-- В каноне tags хранится строкой 'coffee,basics' (байт-совместимость с kafka-
-- cookbook). string_to_array разворачивает её в text[] — в Go это []string
-- (явный ::text[] подсказывает sqlc конкретный тип элемента).
SELECT id, title, string_to_array(tags, ',')::text[] AS tag_list
FROM articles
ORDER BY id;

-- name: ArticlesTaggedCoffee :many
-- Оператор @> — «массив содержит». Находим статьи, у которых среди тегов есть
-- 'coffee'. На больших объёмах такой поиск ускоряет GIN-индекс (модуль 06/07).
SELECT id, title
FROM articles
WHERE string_to_array(tags, ',') @> ARRAY['coffee']
ORDER BY id;

-- name: JSONBIntro :one
-- jsonb-литерал и базовые операторы. Ключевой контраст: ->> достаёт значение
-- как text ('oat'), а -> оставляет его jsonb ('"oat"' — со скобками-кавычками).
-- jsonb_exists (оператор ?) проверяет наличие ключа. coalesce → тип в Go = string.
SELECT
    coalesce('{"size":"L","milk":"oat","shots":2}'::jsonb ->> 'milk', '')        AS milk_text,
    coalesce(('{"size":"L","milk":"oat","shots":2}'::jsonb -> 'milk')::text, '') AS milk_json,
    coalesce('{"size":"L","milk":"oat","shots":2}'::jsonb ->> 'shots', '')       AS shots_text,
    jsonb_exists('{"size":"L","milk":"oat","shots":2}'::jsonb, 'milk')           AS has_milk;
