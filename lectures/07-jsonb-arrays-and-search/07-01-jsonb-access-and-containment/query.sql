-- query.sql — протагонист урока. `make gen` → типизированный pgx-код в
-- internal/db/. Имя после `-- name:` → метод; суффикс — форма результата.
--
-- Тема — четыре оператора доступа к jsonb: -> (достать как jsonb), ->> (достать
-- как text), #>> (достать по пути как text) и @> (containment, «содержит»),
-- плюс ? (есть ли ключ). Это рабочая лошадка любого приложения, кладущего в БД
-- полуструктурированные данные. Containment @> ускоряет GIN-индекс (см. 06-05).

-- name: AccessOps :many
-- Доступ к полям. Ключевой контраст: -> оставляет значение как jsonb (молоко
-- приедет в кавычках: "oat"), а ->> отдаёт чистый text (oat). #>> идёт по пути:
-- '{extras,0}' — нулевой элемент массива extras. coalesce подменяет отсутствующее
-- значение на '∅' (у Егора нет ключа milk, у Бориса — массива extras) и заодно
-- даёт sqlc конкретный тип string вместо nullable interface{}.
SELECT
    customer,
    coalesce((options -> 'milk')::text, '∅')   AS milk_jsonb,
    coalesce(options ->> 'milk', '∅')           AS milk_text,
    coalesce(options ->> 'size', '∅')           AS size,
    coalesce(options ->> 'shots', '∅')          AS shots,
    coalesce(options #>> '{extras,0}', '∅')     AS first_extra
FROM order_options_lab
ORDER BY id;

-- name: OatMilkOrders :many
-- Containment @>: строка слева «содержит» json справа. '{"milk":"oat"}' матчит
-- любую строку, где есть ключ milk со значением oat — независимо от прочих полей.
-- Это НЕ равенство всего документа, а «есть ли в нём такая пара».
SELECT customer, options ->> 'size' AS size
FROM order_options_lab
WHERE options @> '{"milk":"oat"}'
ORDER BY id;

-- name: HoneyInExtras :many
-- Containment умеет заглядывать и в массивы: '{"extras":["honey"]}' матчит строки,
-- где массив extras содержит элемент honey. Глубокая структура — один оператор.
SELECT customer
FROM order_options_lab
WHERE options @> '{"extras":["honey"]}'
ORDER BY id;

-- name: HasExtrasKey :many
-- Оператор ? — «есть ли ключ верхнего уровня». Находим заказы, где вообще
-- указали extras (даже пустой массив у Дины — ключ-то есть). Ср. с @>: ? про
-- НАЛИЧИЕ ключа, @> — про наличие пары ключ-значение.
SELECT customer
FROM order_options_lab
WHERE options ? 'extras'
ORDER BY id;
