-- session-b.sql — СЕССИЯ B (воркер 2 очереди). Запусти во ВТОРОМ терминале по
-- подсказке из session-a:
--   make session-b
--
-- B исполняет ТОТ ЖЕ claim-запрос, что и A. Задача #1 залочена транзакцией A,
-- поэтому SKIP LOCKED её ПРОПУСКАЕТ — B без ожидания получает #2. С обычным
-- FOR UPDATE (без SKIP LOCKED) B заблокировался бы на #1 и простаивал, пока A
-- не закоммитит. Здесь два воркера работают параллельно и не наступают друг
-- другу на пятки.

\set ON_ERROR_STOP on

\echo 'Сессия B — воркер 2. Тот же claim-запрос FOR UPDATE SKIP LOCKED.'
BEGIN;

\echo ''
\echo 'B1) Задача #1 залочена сессией A → SKIP LOCKED её пропускает, берём следующую (#2):'
SELECT id, payload FROM job_queue
WHERE status = 'pending'
ORDER BY id
FOR UPDATE SKIP LOCKED
LIMIT 1;

\echo ''
\echo 'B2) Завершаем взятую задачу и коммитим:'
UPDATE job_queue SET status = 'done' WHERE id = 2;
COMMIT;

\echo ''
\echo '>>> B забрала #2 (не #1!) без ожидания. Вернись в терминал A и нажми Enter.'
