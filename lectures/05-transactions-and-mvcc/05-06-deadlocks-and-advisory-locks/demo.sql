-- demo.sql — детерминированное демо advisory-локов (цель `make run`).
--
-- Это escape-hatch-юнит: дедлок по природе конкурентен (нужны две сессии), а
-- advisory-локи показываются на одной сессии детерминированно. Этот файл —
-- «основной демо»: API прикладных блокировок (pg_advisory_lock и компания).
-- Живой дедлок с ошибкой 40P01 — в session-a.sql / session-b.sql.
--
-- Лабораторного стола тут не нужно: advisory-локи не привязаны к строкам, это
-- именованные «защёлки» по произвольному числовому ключу.

\set ON_ERROR_STOP on
\pset null '∅'
SET client_min_messages = warning;  -- глушим NOTICE про усечение длинных алиасов

-- ── Advisory-лок: прикладная блокировка по числовому ключу ───────────────────
-- В отличие от блокировок строк (05-03), advisory-лок НЕ связан с данными.
-- Ключ — любое 64-битное число, смысл которого знает только приложение
-- («лок на пересчёт остатков кофейни #7»). pg_try_advisory_lock берёт лок и
-- возвращает сразу: t — взяли, f — занято кем-то другим (без ожидания).
\echo '── Берём session-level advisory-лок по ключу 42 ──'
SELECT pg_try_advisory_lock(42) AS got_42;

-- Advisory-локи РЕЕНТРАБЕЛЬНЫ: та же сессия берёт тот же ключ повторно (счётчик
-- становится 2). Значит, и отпускать надо столько же раз.
\echo ''
\echo '── Та же сессия берёт ключ 42 повторно (реентрабельно) → счётчик = 2 ──'
SELECT pg_try_advisory_lock(42) AS got_42_again;

\echo ''
\echo '── Отпускаем дважды (t, t); третий unlock → f (лок уже не наш; +WARNING в stderr) ──'
SELECT pg_advisory_unlock(42) AS unlock_1;
SELECT pg_advisory_unlock(42) AS unlock_2;
SELECT pg_advisory_unlock(42) AS unlock_3;

-- ── Транзакционный advisory-лок: освобождается сам на COMMIT/ROLLBACK ─────────
-- pg_advisory_xact_lock держится до конца транзакции — забыть отпустить нельзя.
-- Это безопаснее session-level варианта (где утечка лока живёт до конца
-- коннекта). После COMMIT ключ свободен — берём его снова без проблем.
\echo ''
\echo '── Транзакционный лок по ключу 7: живёт до COMMIT, освобождается сам ──'
BEGIN;
SELECT pg_advisory_xact_lock(7);
SELECT count(*) AS held_now FROM pg_locks WHERE locktype = 'advisory' AND objid = 7;
COMMIT;
SELECT count(*) AS held_after_commit FROM pg_locks WHERE locktype = 'advisory' AND objid = 7;
\echo '→ held_now=1 (внутри tx), held_after_commit=0 (COMMIT снял лок автоматически).'
