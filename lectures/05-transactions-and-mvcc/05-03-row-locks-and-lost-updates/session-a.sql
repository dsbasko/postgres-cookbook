-- session-a.sql — СЕССИЯ A (воркер 1 очереди). Запусти в ПЕРВОМ терминале:
--   make session-a
--
-- A готовит очередь задач job_queue и забирает ОДНУ задачу запросом-claim'ом
-- `SELECT ... FOR UPDATE SKIP LOCKED LIMIT 1`. Пока A держит задачу #1 (её
-- транзакция открыта), ты в другом терминале прогонишь session-b — второй
-- воркер. Суть урока: B не будет ЖДАТЬ задачу #1 (как было бы с обычным
-- FOR UPDATE) — SKIP LOCKED заставит B ПРОПУСТИТЬ залоченную строку и без
-- задержки взять следующую. Так N воркеров делят очередь без двойной обработки
-- и без выстраивания в очередь друг за другом.
--
-- Порядок шагов задаёт \prompt — он держит транзакцию A открытой, пока ты не
-- вернёшься. Поэтому сценарий детерминирован, а не гонка.

\set ON_ERROR_STOP on
SET client_min_messages = warning;

\echo 'Сессия A — воркер 1. Готовим очередь из трёх задач.'
DROP TABLE IF EXISTS job_queue;
CREATE TABLE job_queue (
    id      int   PRIMARY KEY,
    payload text  NOT NULL,
    status  text  NOT NULL DEFAULT 'pending'
);
INSERT INTO job_queue (id, payload) VALUES
    (1, 'сварить заказ #1'),
    (2, 'сварить заказ #2'),
    (3, 'сварить заказ #3');

\echo ''
\echo 'A1) Забираем задачу claim-запросом FOR UPDATE SKIP LOCKED — строка залочена до COMMIT:'
BEGIN;
SELECT id, payload FROM job_queue
WHERE status = 'pending'
ORDER BY id
FOR UPDATE SKIP LOCKED
LIMIT 1;

\echo ''
\prompt 'A держит задачу #1. Теперь в другом терминале запусти `make session-b`. Когда B заберёт СВОЮ задачу — нажми Enter здесь... ' _

\echo ''
\echo 'A2) Завершаем задачу #1 и коммитим (освобождаем блокировку):'
UPDATE job_queue SET status = 'done' WHERE id = 1;
COMMIT;

\echo ''
\echo 'A3) Итог очереди — B взял ДРУГУЮ задачу (#2), не дожидаясь #1. Двойной обработки нет:'
SELECT id, payload, status FROM job_queue ORDER BY id;

\echo ''
\echo '>>> Демо завершено. Восстанови канон: `make db-reset` (таблицу job_queue можно DROP вручную).'
