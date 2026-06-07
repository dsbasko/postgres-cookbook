-- query.sql — протагонист урока. `make gen` → типизированный pgx-код в
-- internal/db/. Имя после `-- name:` → метод; суффикс — форма результата.
--
-- Тема — transactional outbox. Проблема, которую он решает: надо записать
-- бизнес-факт (заказ) И событие о нём (для рассылки/CDC) так, чтобы они либо
-- появились ВМЕСТЕ, либо никак. Если писать заказ в БД, а событие — отдельно в
-- брокер, между двумя записями возможен сбой: заказ есть, события нет (или
-- наоборот). Outbox убирает зазор: заказ и строку-событие пишем в ОДНОЙ
-- транзакции в одну базу — атомарность даёт сам Postgres. Отдельный процесс
-- (relay) потом вычитывает неопубликованные события и доставляет их.

-- name: InsertOrder :one
-- Бизнес-факт. customer_id здесь TEXT (так в каноне), amount приходит строкой и
-- кастуется в numeric. Именованные параметры (@amount) дают аккуратное имя поля
-- в сгенерированной структуре. Вызывается ВНУТРИ транзакции вместе с InsertOutbox.
INSERT INTO orders (customer_id, amount, status)
VALUES (@customer_id, @amount::numeric, @status)
RETURNING id;

-- name: InsertOutbox :one
-- Событие об этом факте — в той же транзакции, что и заказ. aggregate_id
-- связывает событие с заказом, topic — куда его потом доставить, payload —
-- тело события (jsonb). published_at остаётся NULL: «ещё не доставлено».
INSERT INTO outbox (aggregate_id, topic, payload)
VALUES ($1, $2, $3)
RETURNING id;

-- name: ClaimUnpublished :many
-- Запрос relay'я. Берёт пачку неопубликованных событий ПОД блокировкой строк с
-- SKIP LOCKED (как очередь из 09-02) — чтобы несколько relay-воркеров могли
-- работать параллельно, не доставляя одно событие дважды и не блокируя друг
-- друга. Идёт по partial-индексу outbox_unpublished_idx (WHERE published_at IS NULL).
SELECT id, aggregate_id, topic, payload
FROM outbox
WHERE published_at IS NULL
ORDER BY id
FOR UPDATE SKIP LOCKED
LIMIT $1;

-- name: MarkPublished :exec
-- relay доставил событие → помечаем опубликованным. Делается в той же
-- транзакции, что и ClaimUnpublished: доставили и отметили атомарно.
UPDATE outbox SET published_at = now() WHERE id = $1;

-- name: CountUnpublished :one
-- Сколько событий ещё ждут доставки — для наглядности в демо.
SELECT count(*) FROM outbox WHERE published_at IS NULL;

-- name: CountOrders :one
-- Сколько заказов сейчас в таблице — для проверки атомарности (откат тянет за
-- собой и заказ, и событие).
SELECT count(*) FROM orders;
