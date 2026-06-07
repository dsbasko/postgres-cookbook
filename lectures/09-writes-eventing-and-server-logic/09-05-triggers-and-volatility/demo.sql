-- demo.sql — триггеры и волатильность функций (цель `make run`).
--
-- Escape-hatch-юнит (как 05-02/06-01/08-04): урок про серверную логику —
-- PL/pgSQL-триггеры и классификацию функций — DDL-тяжёлый, sqlc неприменим,
-- ведём psql-скриптом.
--
-- Три части:
--   1) BEFORE-триггер автоматически проставляет updated_at при каждом UPDATE;
--   2) AFTER-триггер пишет аудит со старым (OLD) и новым (NEW) значением,
--      демонстрируя, что в INSERT нет OLD, а в DELETE нет NEW;
--   3) волатильность функций: IMMUTABLE/STABLE/VOLATILE — как их видит
--      планировщик и почему только IMMUTABLE годится в индексное выражение.
--
-- Детерминизм: updated_at печатаем не значением (now() плавает), а булевым
-- «изменился ли»; аудит — на фиксированных данных; волатильность — буквами из
-- каталога (i/s/v) и SQLSTATE отказа. Лабораторные столы и функции создаются и
-- дропаются здесь же — канон Brew не трогаем, вывод воспроизводится дословно.

\set ON_ERROR_STOP on
\set VERBOSITY terse
\pset footer off
SET client_min_messages = warning;   -- глушим NOTICE от DROP ... IF EXISTS

DROP TABLE IF EXISTS touch_lab;
DROP TABLE IF EXISTS audit_lab;
DROP TABLE IF EXISTS priced_lab;
DROP TABLE IF EXISTS vol_lab;
DROP FUNCTION IF EXISTS set_updated_at();
DROP FUNCTION IF EXISTS audit_priced();
DROP FUNCTION IF EXISTS f_imm(int);
DROP FUNCTION IF EXISTS f_stb();
DROP FUNCTION IF EXISTS f_vol();
DROP FUNCTION IF EXISTS f_vol_int(int);

-- ── Часть 1. BEFORE-триггер: автозаполнение updated_at ─────────────────────
-- BEFORE-триггер может ПОМЕНЯТЬ строку до записи, вернув изменённый NEW. Здесь
-- он на каждый UPDATE проставляет updated_at = now() — приложению не надо помнить
-- про это поле, и его нельзя «забыть» обновить.
CREATE TABLE touch_lab (
    id         bigint      PRIMARY KEY,
    name       text        NOT NULL,
    updated_at timestamptz NOT NULL
);
CREATE FUNCTION set_updated_at() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at := now();   -- меняем строку ДО записи
    RETURN NEW;                -- BEFORE-триггер обязан вернуть строку для записи
END;
$$;
CREATE TRIGGER touch_lab_bupd
    BEFORE UPDATE ON touch_lab
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Кладём строку со СТАРОЙ датой (фиксированной), затем обновляем имя — поле
-- updated_at руками НЕ трогаем, его проставит триггер.
INSERT INTO touch_lab (id, name, updated_at) VALUES (1, 'Эспрессо', '2000-01-01 00:00:00+00');
UPDATE touch_lab SET name = 'Эспрессо (1 шот)' WHERE id = 1;

\echo '1) BEFORE-триггер сам проставил updated_at на UPDATE (печатаем факт, не значение):'
SELECT id, name,
       updated_at > '2000-01-01 00:00:00+00' AS updated_at_bumped
FROM touch_lab WHERE id = 1;

-- ── Часть 2. AFTER-триггер: аудит со старым и новым значением ───────────────
-- AFTER-триггер видит уже записанную строку; его возврат игнорируется. Идеален
-- для аудита: по TG_OP пишем, что произошло, и кладём OLD/NEW. В INSERT нет OLD,
-- в DELETE нет NEW — это видно в журнале как ∅.
CREATE TABLE priced_lab (
    id         bigint PRIMARY KEY,
    name       text   NOT NULL,
    price_cents int   NOT NULL
);
CREATE TABLE audit_lab (
    seq       bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    op        text   NOT NULL,
    old_name  text,
    new_name  text,
    old_price int,
    new_price int
);
CREATE FUNCTION audit_priced() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        INSERT INTO audit_lab (op, old_name, new_name, old_price, new_price)
            VALUES ('INSERT', NULL, NEW.name, NULL, NEW.price_cents);
    ELSIF TG_OP = 'UPDATE' THEN
        INSERT INTO audit_lab (op, old_name, new_name, old_price, new_price)
            VALUES ('UPDATE', OLD.name, NEW.name, OLD.price_cents, NEW.price_cents);
    ELSIF TG_OP = 'DELETE' THEN
        INSERT INTO audit_lab (op, old_name, new_name, old_price, new_price)
            VALUES ('DELETE', OLD.name, NULL, OLD.price_cents, NULL);
    END IF;
    RETURN NULL;   -- AFTER-триггер: возврат игнорируется
END;
$$;
CREATE TRIGGER priced_lab_audit
    AFTER INSERT OR UPDATE OR DELETE ON priced_lab
    FOR EACH ROW EXECUTE FUNCTION audit_priced();

INSERT INTO priced_lab (id, name, price_cents) VALUES (1, 'Латте', 480);
UPDATE priced_lab SET price_cents = 500 WHERE id = 1;
DELETE FROM priced_lab WHERE id = 1;

\echo ''
\echo '2) AFTER-триггер записал аудит (∅ = значения нет: OLD в INSERT, NEW в DELETE):'
SELECT op,
       coalesce(old_name, '∅')         AS old_name,
       coalesce(new_name, '∅')         AS new_name,
       coalesce(old_price::text, '∅')  AS old_price,
       coalesce(new_price::text, '∅')  AS new_price
FROM audit_lab ORDER BY seq;

-- ── Часть 3. Волатильность функций: IMMUTABLE / STABLE / VOLATILE ───────────
-- Метка волатильности — это ОБЕЩАНИЕ планировщику, насколько функция «стабильна»:
--   IMMUTABLE — на одни входы всегда один выход (чистая арифметика);
--   STABLE    — не меняется В ПРЕДЕЛАХ одного запроса (now(), чтение таблиц);
--   VOLATILE  — может меняться на каждый вызов (random(), запись) — это ДЕФОЛТ.
-- На метке планировщик строит оптимизации (свёртка констант, кэширование).
CREATE FUNCTION f_imm(int) RETURNS int              LANGUAGE sql IMMUTABLE AS $$ SELECT $1 * 2 $$;
CREATE FUNCTION f_stb()    RETURNS timestamptz       LANGUAGE sql STABLE    AS $$ SELECT now() $$;
CREATE FUNCTION f_vol()    RETURNS double precision  LANGUAGE sql VOLATILE  AS $$ SELECT random() $$;

\echo ''
\echo '3) Как Postgres классифицировал наши функции (provolatile из каталога):'
SELECT proname,
       CASE provolatile
           WHEN 'i' THEN 'IMMUTABLE'
           WHEN 's' THEN 'STABLE'
           WHEN 'v' THEN 'VOLATILE'
       END AS volatility
FROM pg_proc
WHERE proname IN ('f_imm', 'f_stb', 'f_vol')
ORDER BY proname;

-- Практическое следствие: в индексное выражение пускают ТОЛЬКО IMMUTABLE —
-- иначе индекс «протух» бы при первом же изменении значения функции.
CREATE TABLE vol_lab (id bigint PRIMARY KEY, n int NOT NULL);
INSERT INTO vol_lab (id, n) VALUES (1, 10), (2, 20), (3, 30);
-- f_vol_int — тело как у f_imm, но помечена VOLATILE: дело в МЕТКЕ, а не в коде.
-- Язык — plpgsql НАМЕРЕННО: тривиальную SQL-функцию планировщик «встроил» бы в
-- выражение, и метка VOLATILE потерялась бы (индекс собрался бы); plpgsql-функции
-- не встраиваются, поэтому метка остаётся в силе и индекс будет отклонён.
CREATE FUNCTION f_vol_int(int) RETURNS int LANGUAGE plpgsql VOLATILE AS $$ BEGIN RETURN $1 * 2; END; $$;

\echo ''
\echo '   f_imm (IMMUTABLE) в индексном выражении — можно:'
CREATE INDEX vol_imm_idx ON vol_lab (f_imm(n));
SELECT 'индекс по f_imm(n) создан' AS result;

\echo '   f_vol_int (VOLATILE) в индексном выражении — нельзя (сырой текст ошибки в stderr):'
\set ON_ERROR_STOP off
CREATE INDEX vol_vol_idx ON vol_lab (f_vol_int(n));
\echo 'SQLSTATE =' :LAST_ERROR_SQLSTATE '(functions in index expression must be marked IMMUTABLE)'
\set ON_ERROR_STOP on

-- Уборка: лабораторные столы и функции. Канон Brew не трогали.
DROP TABLE touch_lab;
DROP TABLE audit_lab;
DROP TABLE priced_lab;
DROP TABLE vol_lab;
DROP FUNCTION set_updated_at();
DROP FUNCTION audit_priced();
DROP FUNCTION f_imm(int);
DROP FUNCTION f_stb();
DROP FUNCTION f_vol();
DROP FUNCTION f_vol_int(int);
