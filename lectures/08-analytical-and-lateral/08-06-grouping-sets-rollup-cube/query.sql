-- query.sql — протагонист урока. `make gen` → типизированный pgx-код в
-- internal/db/. Имя после `-- name:` → метод; суффикс — форма результата.
--
-- Тема — субитоги одним запросом. Обычный GROUP BY (shop, category) даёт только
-- листья. ROLLUP добавляет иерархические подытоги (по магазину + общий), CUBE —
-- ВСЕ комбинации (ещё и по категории поперёк магазинов), GROUPING SETS — ровно
-- те срезы, что перечислишь. Функция grouping(col) = 1, если в этой строке
-- колонка «свёрнута» (NULL-подытог), и 0, если это реальное значение — так
-- отличают строку-итог от настоящего NULL и сортируют итоги в конец.

-- name: RollupByShop :many
-- ROLLUP (shop, category): листья (shop, category) + подытог по каждому магазину
-- (category свёрнута) + общий итог (обе свёрнуты). coalesce(...,'— все —')
-- подписывает свёрнутые уровни; level = grouping(shop)+grouping(category)
-- (0 — данные, 1 — подытог по магазину, 2 — общий итог).
SELECT
    coalesce(shop, '— все —')                  AS shop,
    coalesce(category, '— все —')              AS category,
    (sum(cents))::bigint                       AS cents,
    (grouping(shop) + grouping(category))::int AS level
FROM sales_fact_lab
GROUP BY ROLLUP (shop, category)
ORDER BY grouping(shop), shop, grouping(category), category;

-- name: CubeAllAngles :many
-- CUBE (shop, category): к строкам ROLLUP добавляются ещё подытоги по КАТЕГОРИИ
-- поперёк магазинов (shop свёрнут, category — нет): «сколько всего coffee по
-- всей сети». То есть все 4 комбинации свёрнутости двух колонок.
SELECT
    coalesce(shop, '— все —')                  AS shop,
    coalesce(category, '— все —')              AS category,
    (sum(cents))::bigint                       AS cents,
    (grouping(shop) + grouping(category))::int AS level
FROM sales_fact_lab
GROUP BY CUBE (shop, category)
ORDER BY grouping(shop), shop, grouping(category), category;

-- name: GroupingSetsExplicit :many
-- GROUPING SETS перечисляет нужные срезы вручную: «итоги по магазину», «итоги по
-- категории» и «общий итог» — без листьев (shop, category). Это даёт ровно три
-- среза, которые в дашборде и нужны, без лишних строк CUBE.
SELECT
    coalesce(shop, '— все —')                  AS shop,
    coalesce(category, '— все —')              AS category,
    (sum(cents))::bigint                       AS cents,
    (grouping(shop) + grouping(category))::int AS level
FROM sales_fact_lab
GROUP BY GROUPING SETS ((shop), (category), ())
ORDER BY grouping(shop), shop, grouping(category), category;
