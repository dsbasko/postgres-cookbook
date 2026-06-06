-- query.sql — протагонист урока. `make gen` → типизированный pgx-код в
-- internal/db/. Имя после `-- name:` → метод; суффикс — форма результата.
--
-- Тема — транзакции и ACID. Перевод денег между счетами — две команды
-- (списать у одного, зачислить другому), которые обязаны примениться вместе
-- или никак: это атомарность (A в ACID). Запросы тут — кирпичики; собирает их
-- в одну транзакцию main.go (BEGIN → Debit → Credit → COMMIT, иначе ROLLBACK).

-- name: TruncateAccounts :exec
-- Перед демо обнуляем таблицу и счётчик IDENTITY — id всегда 1..2, вывод
-- воспроизводим независимо от числа прогонов.
TRUNCATE ledger_accounts RESTART IDENTITY;

-- name: SeedAccount :exec
INSERT INTO ledger_accounts (owner, balance) VALUES ($1, $2);

-- name: ListAccounts :many
SELECT id, owner, balance FROM ledger_accounts ORDER BY id;

-- name: TotalBalance :one
-- Сумма всех счетов — инвариант системы. Перевод денег НЕ должен её менять
-- (C в ACID: консистентность). COALESCE на случай пустой таблицы.
SELECT COALESCE(SUM(balance), 0)::bigint FROM ledger_accounts;

-- name: Debit :execrows
-- Списание. CHECK (balance >= 0) в схеме отвергает уход в минус (SQLSTATE
-- 23514) — это и роняет транзакцию при overdraft. :execrows отдаёт число
-- затронутых строк: 0 значит «счёта с таким id нет».
UPDATE ledger_accounts
SET balance = balance - sqlc.arg(amount)
WHERE id = sqlc.arg(id);

-- name: Credit :execrows
-- Зачисление. :execrows == 0 значит «получателя не существует» — повод
-- откатить весь перевод (см. сценарий 3 в main.go).
UPDATE ledger_accounts
SET balance = balance + sqlc.arg(amount)
WHERE id = sqlc.arg(id);
