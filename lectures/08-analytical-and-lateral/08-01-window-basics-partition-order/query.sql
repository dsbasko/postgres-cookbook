-- query.sql — протагонист урока. `make gen` → типизированный pgx-код в
-- internal/db/. Имя после `-- name:` → метод; суффикс — форма результата.
--
-- Тема — что такое оконная функция и чем она отличается от агрегата. Агрегат с
-- GROUP BY СХЛОПЫВАЕТ группу строк в одну. Оконная функция (та же sum/avg/count,
-- но с OVER (...)) считает по «окну» строк, НО оставляет каждую исходную строку
-- на месте и доклеивает результат отдельной колонкой. PARTITION BY режет таблицу
-- на окна, ORDER BY внутри окна превращает sum в накопленный итог (running total).

-- name: CustomerTotals :many
-- Обычный агрегат для контраста: GROUP BY схлопывает покупки клиента в ОДНУ
-- строку — сами покупки после этого не достать.
SELECT customer,
       count(*)          AS purchases,
       (sum(cents))::bigint AS total
FROM purchases_lab
GROUP BY customer
ORDER BY customer;

-- name: WindowTotals :many
-- Та же sum, но как ОКОННАЯ функция: каждая из 7 покупок остаётся на месте, а
-- рядом появляются две колонки. sum(cents) OVER (PARTITION BY customer) — сумма
-- по клиенту (повторяется в каждой его строке); sum(cents) OVER () — общий итог
-- по всей таблице (окно без PARTITION = все строки). Строки НЕ схлопнулись.
SELECT customer,
       day::text AS day,
       cents,
       (sum(cents) OVER (PARTITION BY customer))::bigint AS customer_total,
       (sum(cents) OVER ())::bigint                      AS grand_total
FROM purchases_lab
ORDER BY customer, day, id;

-- name: RunningTotal :many
-- Добавляем ORDER BY ВНУТРИ окна — и sum превращается в накопленный итог: для
-- каждой строки это сумма cents от начала окна до текущей строки включительно.
-- PARTITION BY customer обнуляет накопление на границе клиента; ORDER BY day, id
-- задаёт порядок накопления (id — детерминированный tie-break внутри дня).
SELECT customer,
       day::text AS day,
       cents,
       (sum(cents) OVER (PARTITION BY customer ORDER BY day, id))::bigint AS running
FROM purchases_lab
ORDER BY customer, day, id;
