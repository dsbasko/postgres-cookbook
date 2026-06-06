-- demo.sql — детерминированное демо механики MVCC (цель `make run`).
--
-- Это escape-hatch-юнит: sqlc здесь неприменим (нам нужны системные колонки
-- ctid/xmin/xmax и наблюдение за версиями строки), поэтому урок ведётся
-- psql-скриптами, а не query.sql + кодоген. Этот файл — «основной демо»,
-- на который ссылается цель run; вторая половина урока (снапшот-изоляция между
-- двумя транзакциями) — в session-a.sql / session-b.sql.
--
-- Работаем на отдельном лабораторном столе mvcc_lab, а не на каноне Brew:
-- свежая таблица даёт чистые ctid/xmin (никакая прошлая транзакция их не
-- «запачкала»), а DROP в конце оставляет песочницу нетронутой — демо
-- идемпотентно и вывод воспроизводится дословно.

\set ON_ERROR_STOP on
\pset null '∅'
SET client_min_messages = warning;  -- глушим NOTICE от DROP ... IF EXISTS

DROP TABLE IF EXISTS mvcc_lab;
CREATE TABLE mvcc_lab (id int PRIMARY KEY, price int);
INSERT INTO mvcc_lab VALUES (2, 450);

-- ctid — физический адрес версии строки (страница, смещение). xmax = 0 значит
-- «эту версию ещё никто не сменил и не залочил».
\echo '── Свежая строка: одна версия, ctid = физический адрес ───────────────'
SELECT id, price, ctid, (xmax <> '0'::xid) AS superseded FROM mvcc_lab WHERE id = 2;

BEGIN;
-- Запоминаем физический адрес и создавшую версию транзакцию ДО изменения.
-- ctid/xmin — системные колонки, поэтому при копировании их переименовываем
-- (иначе имя столбца конфликтует с системным).
CREATE TEMP TABLE _before ON COMMIT DROP AS
  SELECT ctid AS c, xmin AS x FROM mvcc_lab WHERE id = 2;

-- UPDATE в MVCC не переписывает строку на месте: он помечает старую версию
-- мёртвой (ставит ей xmax = id этой транзакции) и пишет НОВУЮ версию.
UPDATE mvcc_lab SET price = price + 50 WHERE id = 2;

CREATE TEMP TABLE _after ON COMMIT DROP AS
  SELECT ctid AS c, xmin AS x FROM mvcc_lab WHERE id = 2;

\echo ''
\echo '── UPDATE написал НОВУЮ версию строки (внутри той же транзакции) ──────'
SELECT (b.c <> a.c) AS ctid_changed, (b.x <> a.x) AS xmin_changed
FROM _before b, _after a;
COMMIT;

-- Старая версия (price=450) ещё физически лежит на странице как «мёртвый
-- кортеж» — её уберёт VACUUM. Видимая нам строка — уже новая версия.
\echo ''
\echo '── Итог: одна логическая строка id=2 — но уже вторая физическая версия '
SELECT id, price, ctid, (xmax <> '0'::xid) AS superseded FROM mvcc_lab WHERE id = 2;

DROP TABLE mvcc_lab;
