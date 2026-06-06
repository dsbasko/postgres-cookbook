-- query.sql — протагонист урока. `make gen` → типизированный pgx-код в
-- internal/db/. Имя после `-- name:` → метод; суффикс — форма результата.
--
-- Тема — INSERT ... RETURNING. Главный вывод: значения, которые присваивает
-- сервер (сгенерированный id, колонки по DEFAULT), приезжают обратно тем же
-- запросом — второй SELECT не нужен. И это работает не только для одной строки:
-- bulk-INSERT через unnest тоже умеет RETURNING.

-- name: TruncateCards :exec
-- Перед демо обнуляем таблицу и счётчик IDENTITY — id всегда стартуют с 1,
-- вывод воспроизводим независимо от числа прогонов.
TRUNCATE loyalty_cards RESTART IDENTITY;

-- name: IssueCard :one
-- Выдаём карту: передаём только customer_id и card_no. id присвоит БД (IDENTITY),
-- points и created_at подставит DEFAULT. RETURNING возвращает их в том же
-- round-trip — без отдельного SELECT за сгенерированным id.
INSERT INTO loyalty_cards (customer_id, card_no)
VALUES ($1, $2)
RETURNING id, points, (created_at IS NOT NULL)::boolean AS created_set;

-- name: IssueCardsBulk :many
-- Многострочный INSERT: одна команда вставляет сразу несколько карт, и RETURNING
-- отдаёт id каждой — по строке на вставленную карту (форма :many). Так RETURNING
-- работает и для одной строки, и для многих. (Для вставки переменного числа
-- строк из Go-среза берут `unnest($1::bigint[])`, а для массовой загрузки —
-- COPY; см. заборчик и forward-ссылку на 09-01.)
INSERT INTO loyalty_cards (customer_id, card_no)
VALUES (sqlc.arg(cust_a), sqlc.arg(card_a)),
       (sqlc.arg(cust_b), sqlc.arg(card_b))
RETURNING id, card_no;
