-- demo.sql — детерминированное демо уровней изоляции (цель `make run`).
--
-- Это escape-hatch-юнит: аномалии изоляции по природе конкурентны, sqlc тут не
-- помощник. Этот файл показывает то, что детерминированно на одной сессии:
-- уровень изоляции по умолчанию, как он задаётся на транзакцию, и ЛОГИКУ
-- аномалии write-skew (два устаревших чтения имитируем последовательно). Живой
-- конфликт двух транзакций под SERIALIZABLE (ошибка 40001) — в session-a.sql /
-- session-b.sql.
--
-- Лабораторный стол shift_lab; DROP в конце оставляет песочницу нетронутой.

\set ON_ERROR_STOP on
\pset null '∅'
SET client_min_messages = warning;

DROP TABLE IF EXISTS shift_lab;
CREATE TABLE shift_lab (id int PRIMARY KEY, name text, on_floor boolean NOT NULL);
INSERT INTO shift_lab VALUES (1, 'Алиса', true), (2, 'Борис', true);

-- ── Уровень изоляции по умолчанию ────────────────────────────────────────────
-- В Postgres дефолт — READ COMMITTED: каждая КОМАНДА видит свежий снимок, то
-- есть внутри одной транзакции два SELECT могут вернуть разное, если между ними
-- кто-то закоммитил. REPEATABLE READ фиксирует снимок на всю транзакцию (см.
-- 05-02), SERIALIZABLE добавляет защиту от аномалий вроде write-skew.
\echo '── Уровень изоляции по умолчанию (дефолт Postgres) ──'
SHOW transaction_isolation;

-- ── Уровень задаётся НА ТРАНЗАКЦИЮ ───────────────────────────────────────────
\echo ''
\echo '── Уровень задаётся на транзакцию через BEGIN ISOLATION LEVEL ... ──'
BEGIN ISOLATION LEVEL REPEATABLE READ;
SELECT current_setting('transaction_isolation') AS "внутри BEGIN REPEATABLE READ";
COMMIT;
BEGIN ISOLATION LEVEL SERIALIZABLE;
SELECT current_setting('transaction_isolation') AS "внутри BEGIN SERIALIZABLE";
COMMIT;

-- ── Логика write-skew ────────────────────────────────────────────────────────
-- Правило Brew: на полу всегда должен быть хотя бы один бариста. Сейчас их два.
-- Двое решают уйти на перерыв ОДНОВРЕМЕННО. Каждый смотрит «сколько на полу»,
-- видит 2 ≥ 1 и думает «я могу уйти, один останется». Оба уходят — на полу 0.
-- Каждый по отдельности рассуждал верно; вместе они нарушили инвариант. Это
-- write-skew: транзакции читают одно множество, а пишут в РАЗНЫЕ строки, и ни
-- блокировка строк (05-03), ни REPEATABLE READ это не ловят.
\echo ''
\echo '── Write-skew: правило «на полу всегда ≥1 бариста». На полу сейчас:'
SELECT count(*) AS "на полу" FROM shift_lab WHERE on_floor;

\echo ''
\echo 'Алиса смотрит «сколько на полу» (видит 2 ≥ 1 → решает уйти):'
SELECT count(*) AS "Алиса видит на полу" FROM shift_lab WHERE on_floor;
\echo 'Борис смотрит ОДНОВРЕМЕННО, по своему снимку (тоже видит 2 → тоже решает уйти):'
SELECT count(*) AS "Борис видит на полу" FROM shift_lab WHERE on_floor;
-- оба приводят решение в действие — каждый снимает СВОЙ флаг:
UPDATE shift_lab SET on_floor = false WHERE id = 1;   -- Алиса ушла
UPDATE shift_lab SET on_floor = false WHERE id = 2;   -- Борис ушёл

\echo ''
\echo 'Итог — на полу не осталось никого, хотя каждый «оставлял одного»:'
SELECT count(*) AS "на полу" FROM shift_lab WHERE on_floor;
\echo '→ инвариант сломан. READ COMMITTED и REPEATABLE READ это пропускают;'
\echo '  ловит только SERIALIZABLE — он завершит одну из транзакций ошибкой 40001 (см. сессии).'

DROP TABLE shift_lab;
