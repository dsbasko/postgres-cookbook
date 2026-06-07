-- query.sql — протагонист урока. `make gen` → типизированный pgx-код в
-- internal/db/. Имя после `-- name:` → метод; суффикс — форма результата.
--
-- Тема — SQL/JSON path (язык jsonpath: $.a.b, [*], фильтры ? (@.x > N)) и сборка
-- jsonb (jsonb_set, jsonb_build_object, jsonb_agg). jsonpath — стандартный
-- способ доставать данные из вложенных документов, куда обычные -> / #> лезут
-- неуклюже.
--
-- Заметка по версиям: JSON_TABLE (развернуть jsonb в реляционную таблицу прямо
-- в FROM) появился в PG17, не в PG18 — здесь его НЕ используем; нужные нам
-- jsonb_path_* и сборочные функции есть с PG12.

-- name: PathQueries :one
-- jsonpath на рецепте Латте (id=1). $.ingredients[*].name — имена всех
-- ингредиентов; фильтр ? (@.grams > 100) оставляет только тяжёлые (по граммам);
-- jsonb_path_query_first берёт первое совпадение. _array собирает совпадения в
-- jsonb-массив, ::text делает его печатаемым. coalesce → конкретный тип string.
SELECT
    coalesce(jsonb_path_query_array(recipe, '$.ingredients[*].name')::text, '[]')                   AS all_names,
    coalesce(jsonb_path_query_array(recipe, '$.ingredients[*] ? (@.grams > 100).name')::text, '[]') AS heavy_names,
    coalesce(jsonb_path_query_first(recipe, '$.ingredients[0].name')::text, 'null')                 AS first_name
FROM drink_recipe_lab
WHERE id = 1;

-- name: PathPredicates :many
-- Предикаты пути: @? («существует ли совпадение») и @@ («истинно ли условие»).
-- @? '$.ingredients[*] ? (@.name == "milk")' — есть ли ингредиент molk;
-- @@ '$.kcal > 100' — калорийный ли напиток. Оба возвращают boolean.
SELECT
    name,
    (recipe @? '$.ingredients[*] ? (@.name == "milk")') AS has_milk,
    (recipe @@ '$.kcal > 100')                          AS over_100_kcal
FROM drink_recipe_lab
ORDER BY id;

-- name: SetField :one
-- jsonb_set точечно правит поле и возвращает НОВЫЙ документ — хранимая строка не
-- меняется (это чистая функция, ср. write-amplification из 07-02: «правка» =
-- сборка нового значения). Показываем kcal до и после правки на 130.
SELECT
    recipe ->> 'kcal'                             AS kcal_before,
    jsonb_set(recipe, '{kcal}', '130') ->> 'kcal' AS kcal_after
FROM drink_recipe_lab
WHERE id = 1;

-- name: BuildMenu :one
-- Сборка в обратную сторону: из строк канона drinks собираем ОДИН jsonb-массив
-- объектов. jsonb_build_object лепит объект из пар, jsonb_agg(... ORDER BY id)
-- агрегирует строки в массив (ORDER BY внутри агрегата → стабильный порядок).
SELECT coalesce(
    jsonb_agg(jsonb_build_object('sku', sku, 'price_cents', base_price) ORDER BY id)::text,
    '[]'
) AS menu_json
FROM drinks;
