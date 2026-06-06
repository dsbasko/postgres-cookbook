-- session-a.sql — СЕССИЯ A. Запусти в ПЕРВОМ терминале:
--   make session-a
--
-- A и B — два бариста, решающие уйти с пола ОДНОВРЕМЕННО, обе под уровнем
-- изоляции SERIALIZABLE. Правило: на полу всегда ≥1 бариста. Каждый видит «на
-- полу двое», считает свой уход безопасным и снимает свой флаг. Под REPEATABLE
-- READ обе бы закоммитили — и на полу осталось бы 0 (write-skew, никем не
-- замечен). Под SERIALIZABLE база обнаружит опасную пару зависимостей и
-- завершит ВТОРОГО коммитера (это будет A) ошибкой 40001 — инвариант спасён.
--
-- ON_ERROR_STOP off: ошибку 40001 на COMMIT мы хотим УВИДЕТЬ, а не упасть на
-- ней. Порядок шагов держит \prompt — сценарий детерминирован, не гонка.

\set ON_ERROR_STOP off
SET client_min_messages = warning;

\echo 'Сессия A — бариста Алиса. Готовим стол и открываем транзакцию SERIALIZABLE.'
DROP TABLE IF EXISTS shift_lab;
CREATE TABLE shift_lab (id int PRIMARY KEY, name text, on_floor boolean NOT NULL);
INSERT INTO shift_lab VALUES (1, 'Алиса', true), (2, 'Борис', true);

BEGIN ISOLATION LEVEL SERIALIZABLE;

\echo ''
\echo 'A1) Алиса смотрит, сколько барист на полу (видит 2 ≥ 1 → уйти можно):'
SELECT count(*) AS "на полу" FROM shift_lab WHERE on_floor;

\echo ''
\echo 'A2) Алиса снимает СВОЙ флаг (id=1). Транзакция пока НЕ закоммичена:'
UPDATE shift_lab SET on_floor = false WHERE id = 1;

\echo ''
\prompt 'Теперь в другом терминале прогони `make session-b` целиком — Борис уйдёт и закоммитит. Затем вернись сюда и нажми Enter... ' _

\echo ''
\echo 'A3) Алиса коммитит ВТОРОЙ. SERIALIZABLE видит: A и B прочитали одно множество,'
\echo '    а сняли РАЗНЫЕ флаги — вместе они нарушили бы «≥1 на полу». COMMIT падает 40001:'
COMMIT;

\echo ''
\echo 'A4) Транзакция A отменена целиком — её UPDATE не применён. На полу всё ещё есть Алиса:'
SELECT id, name, on_floor FROM shift_lab ORDER BY id;

\echo ''
\echo '>>> 40001 — это «повтори транзакцию»: на ретрае A прочитает свежее (на полу уже 1)'
\echo '    и НЕ уйдёт. Сам ретрай — в юните 05-05. Прибраться: make db-reset.'
