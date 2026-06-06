-- session-b.sql — сессия B: пока сессия A строит индекс CONCURRENTLY на cic_live,
-- пишем в ту же таблицу. INSERT проходит немедленно: CONCURRENTLY держит слабую
-- блокировку (SHARE UPDATE EXCLUSIVE), которая НЕ конфликтует с записью строк.
--
-- С обычным CREATE INDEX (без CONCURRENTLY) этот же INSERT встал бы в очередь и
-- ждал бы конца сборки индекса — в этом вся разница.

\timing on
INSERT INTO cic_live (payload) VALUES ('запись во время конкурентной сборки') RETURNING id;
\echo 'B: INSERT прошёл во время сборки индекса в сессии A — запись не блокировалась.'
