-- schema.sql — DDL-добавки ЭТОГО юнита поверх канона Brew (schema/brew.sql).
--
-- Тема — транзакции и ACID на классическом примере: перевод денег между
-- счетами. Чтобы безопасно ронять транзакции (overdraft, несуществующий
-- получатель), работаем на СВОЕЙ лабораторной таблице ledger_accounts —
-- канон не трогаем. Демо засевает её детерминированно в начале каждого прогона.
--
-- DDL применяется двумя путями: sqlc читает его на `make gen`, а демо встраивает
-- через runtime.Caller и накатывает brew.Apply при db-reset. Поэтому он
-- идемпотентен (CREATE TABLE IF NOT EXISTS).

-- ledger_accounts — кассовые счета кофеен Brew. Баланс в центах (BIGINT, см.
-- 01-01). CHECK (balance >= 0) — инвариант «нельзя уйти в минус»: именно он
-- роняет транзакцию при попытке списать больше, чем есть, и даёт нам наблюдать
-- атомарность (частичный перевод не доходит до диска).
CREATE TABLE IF NOT EXISTS ledger_accounts (
    id       BIGINT  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    owner    TEXT    NOT NULL,
    balance  BIGINT  NOT NULL CHECK (balance >= 0)
);
