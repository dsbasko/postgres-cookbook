-- query.sql — протагонист урока. `make gen` → типизированный pgx-код в
-- internal/db/. Имя после `-- name:` → метод; суффикс — форма результата.
--
-- Тема — ранжирование. Три функции нумеруют строки внутри окна по ORDER BY, но
-- по-разному ведут себя на ничьих: row_number() даёт строго уникальный номер;
-- rank() даёт одинаковый номер ничьим и ПРОПУСКАЕТ следующие (1,2,2,4);
-- dense_rank() — одинаковый номер ничьим, но БЕЗ пропуска (1,2,2,3). Плюс
-- top-N на группу (row_number() = 1) и ntile() — раскладка строк по корзинам.

-- name: RankFunctions :many
-- Три ранга бок о бок внутри одной категории (coffee). Тонкость: «ничья» для
-- rank/dense_rank — это равенство по ВСЕМ колонкам ORDER BY окна. Поэтому
-- row_number считаем по окну wu (units DESC, drink — drink даёт уникальный
-- tie-break и строгую нумерацию 1,2,3,4), а rank/dense_rank — по окну wt
-- (только units DESC, чтобы 120/120 были настоящими пирами). На ничьей видно:
-- row_number 2,3, а rank/dense_rank обе 2; на Рафе (90) rank прыгает на 4
-- (пропуск числа), dense_rank идёт ровно 3 (без пропуска).
SELECT
    drink,
    units,
    row_number() OVER wu AS rn,
    rank()       OVER wt AS rnk,
    dense_rank() OVER wt AS dns
FROM drink_sales_lab
WHERE category = 'coffee'
WINDOW wu AS (ORDER BY units DESC, drink),
       wt AS (ORDER BY units DESC)
ORDER BY units DESC, drink;

-- name: TopPerCategory :many
-- Top-1 на группу — классический приём: пронумеровать строки внутри каждой
-- категории (PARTITION BY category) по убыванию продаж, затем во ВНЕШНЕМ запросе
-- оставить только rn = 1. WHERE по оконной функции напрямую нельзя (она ещё не
-- посчитана на этапе WHERE) — поэтому нумерация прячется в CTE.
WITH ranked AS (
    SELECT category, drink, units,
           row_number() OVER (PARTITION BY category ORDER BY units DESC, drink) AS rn
    FROM drink_sales_lab
)
SELECT category, drink, units
FROM ranked
WHERE rn = 1
ORDER BY category;

-- name: Quartiles :many
-- ntile(4) раскладывает все 8 напитков на 4 равные корзины по продажам (по 2 в
-- каждой). Корзина 1 — лидеры, корзина 4 — аутсайдеры. Удобно для «раздели на
-- квартили/децили», когда конкретный ранг не нужен, а нужна группа.
SELECT
    drink,
    units,
    ntile(4) OVER (ORDER BY units DESC, drink) AS quartile
FROM drink_sales_lab
ORDER BY units DESC, drink;
