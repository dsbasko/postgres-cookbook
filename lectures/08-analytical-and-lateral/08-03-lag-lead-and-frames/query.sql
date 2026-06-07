-- query.sql — протагонист урока. `make gen` → типизированный pgx-код в
-- internal/db/. Имя после `-- name:` → метод; суффикс — форма результата.
--
-- Тема — две вещи. (1) lag/lead заглядывают на N строк назад/вперёд внутри
-- окна — отсюда «сколько вчера», «дельта день-к-дню». (2) Оконный ФРЕЙМ задаёт,
-- какие строки попадают в агрегат относительно текущей. ROWS BETWEEN считает
-- ФИЗИЧЕСКИЕ строки (2 предыдущие), RANGE BETWEEN считает по ЗНАЧЕНИЮ ORDER BY
-- (даты в пределах 2 дней). Если в ряду есть пропуск дня — окна расходятся.

-- name: DayOverDay :many
-- lag(cents) — выручка предыдущей строки, lead(cents) — следующей. Дельта
-- день-к-дню = cents - lag(cents). У первой строки нет предыдущей (lag = NULL),
-- у последней нет следующей (lead = NULL) — приводим к text и подменяем '—'
-- через coalesce, чтобы тип в Go был чистый string, а не nullable interface{}.
SELECT
    day::text AS day,
    cents,
    coalesce((lag(cents) OVER (ORDER BY day))::text, '—')            AS prev,
    coalesce((cents - lag(cents) OVER (ORDER BY day))::text, '—')    AS delta,
    coalesce((lead(cents) OVER (ORDER BY day))::text, '—')           AS next
FROM daily_revenue_lab
ORDER BY day;

-- name: MovingAverage :many
-- Скользящее среднее за «текущий день и два предыдущих» двумя способами.
-- ma_rows: ROWS BETWEEN 2 PRECEDING AND CURRENT ROW — ровно три ФИЗИЧЕСКИЕ
-- строки, разрыв в датах игнорируется. ma_range: RANGE BETWEEN INTERVAL '2 days'
-- PRECEDING — все строки, чья дата попадает в [день-2, день]; пропущенный день
-- просто отсутствует, поэтому окно у 06 и 07 февраля СУЖАЕТСЯ — и средние
-- расходятся с ma_rows. round(...,2) даёт детерминированный текст.
SELECT
    day::text AS day,
    cents,
    round((avg(cents) OVER (ORDER BY day ROWS  BETWEEN 2 PRECEDING AND CURRENT ROW))::numeric, 2)::text AS ma_rows,
    round((avg(cents) OVER (ORDER BY day RANGE BETWEEN INTERVAL '2 days' PRECEDING AND CURRENT ROW))::numeric, 2)::text AS ma_range
FROM daily_revenue_lab
ORDER BY day;
