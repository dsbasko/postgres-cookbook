-- session-a.sql — сессия A: строит индекс CONCURRENTLY на БОЛЬШОЙ таблице
-- (сборка идёт несколько секунд). Пока она идёт — переключись в терминал 2 и
-- запусти `make session-b`: её INSERT пройдёт сразу, не дожидаясь конца сборки.
-- Это и есть смысл CONCURRENTLY: запись во время построения индекса не блокируется.
--
-- ⚠️ Сценарий интерактивный и зависит от тайминга (см. README): запусти A и БЫСТРО
-- переключись на B, пока идёт CREATE INDEX CONCURRENTLY.

\timing on
SET client_min_messages = warning;

DROP TABLE IF EXISTS cic_live;
CREATE TABLE cic_live (
    id      bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    payload text   NOT NULL
);
INSERT INTO cic_live (payload) SELECT 'p' || g FROM generate_series(1, 3000000) g;

\echo ''
\echo '>>> Таблица готова (3 млн строк). БЫСТРО переключись в терминал 2: make session-b <<<'
\echo '>>> Сейчас стартует CREATE INDEX CONCURRENTLY — он займёт несколько секунд <<<'
\echo ''

CREATE INDEX CONCURRENTLY cic_live_payload_idx ON cic_live (payload);

\echo 'A: индекс построен. Записи сессии B в это время НЕ блокировались.'
SELECT count(*) AS rows_now FROM cic_live;   -- больше 3 млн, если B успела вставить
