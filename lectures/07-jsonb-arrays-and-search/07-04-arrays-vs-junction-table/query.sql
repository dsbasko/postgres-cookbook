-- query.sql — протагонист урока. `make gen` → типизированный pgx-код в
-- internal/db/. Имя после `-- name:` → метод; суффикс — форма результата.
--
-- Тема — массив против таблицы-связки. Один и тот же вопрос («напитки с тегом
-- coffee») задаём обеим моделям и видим, что результат совпадает, а вот цена
-- разных вопросов разная: containment по массиву — один оператор, а частота
-- тегов тривиальна на junction (GROUP BY) и неуклюжа на массиве (нужен unnest).

-- name: ArrayTaggedCoffee :many
-- Массив, оператор @> («содержит»): теги напитка включают coffee. На большой
-- таблице это ускоряет GIN-индекс по tags (см. 06-05).
SELECT drink_sku
FROM drink_tags_arr
WHERE tags @> ARRAY['coffee']
ORDER BY drink_sku;

-- name: ArrayHasTag :many
-- Массив, оператор = ANY: проверка принадлежности одного значения. Параметр
-- $1 sqlc типизирует из схемы (tags — text[] → элемент text → Go string).
SELECT drink_sku
FROM drink_tags_arr
WHERE sqlc.arg(tag)::text = ANY(tags)
ORDER BY drink_sku;

-- name: JunctionTaggedCoffee :many
-- Та же выборка на нормализованной модели — обычный фильтр по строке-связке.
-- Результат совпадает с ArrayTaggedCoffee: модели эквивалентны по данным.
SELECT drink_sku
FROM drink_tags
WHERE tag = 'coffee'
ORDER BY drink_sku;

-- name: TagPopularity :many
-- Козырь нормализации: «сколько напитков у каждого тега» — это просто GROUP BY.
-- На массиве пришлось бы разворачивать unnest(tags) и группировать — лишний шаг.
SELECT tag, count(*)::bigint AS used
FROM drink_tags
GROUP BY tag
ORDER BY used DESC, tag;

-- name: TagsFromJunction :many
-- Мост между моделями: array_agg сворачивает строки-связки обратно в массив.
-- ORDER BY внутри агрегата → стабильный порядок элементов. ::text[] → Go []string.
SELECT drink_sku, array_agg(tag ORDER BY tag)::text[] AS tags
FROM drink_tags
GROUP BY drink_sku
ORDER BY drink_sku;
