// Команда demo юнита 05-01: транзакции и ACID — перевод денег между счетами
// «вместе или никак».
//
// Два режима:
//
//	demo          — успешный перевод в транзакции + неудачный с ROLLBACK;
//	demo -reset   — накатить канон Brew + таблицу ledger_accounts и выйти (db-reset).
//
// ledger_accounts засевается в начале демо (TRUNCATE + 2 счёта) → вывод
// детерминирован и идемпотентен. Логи — в stderr, stdout — только результат
// (для дословной вставки в README).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dsbasko/postgres-cookbook/lectures/05-transactions-and-mvcc/05-01-transactions-and-acid/internal/db"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

func main() {
	logger := log.New()

	reset := flag.Bool("reset", false, "накатить канон Brew + ledger_accounts и выйти")
	flag.Parse()

	ctx, cancel := runctx.New()
	defer cancel()

	if err := run(ctx, *reset); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("demo failed", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, reset bool) error {
	pool, err := pg.NewPool(ctx)
	if err != nil {
		return fmt.Errorf("pg.NewPool: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping (песочница поднята? `docker compose up -d`): %w", err)
	}

	if reset {
		ddl, err := schemaDDL()
		if err != nil {
			return err
		}
		if err := brew.Apply(ctx, pool, ddl); err != nil {
			return fmt.Errorf("brew.Apply: %w", err)
		}
		fmt.Println("Канон Brew + таблица ledger_accounts накатаны.")
		return nil
	}

	queries := db.New(pool)

	// Засеваем два кассовых счёта детерминированно (id 1, 2).
	if err := queries.TruncateAccounts(ctx); err != nil {
		return fmt.Errorf("TruncateAccounts: %w", err)
	}
	if err := queries.SeedAccount(ctx, db.SeedAccountParams{Owner: "Касса Brew Central", Balance: 10000}); err != nil {
		return fmt.Errorf("SeedAccount: %w", err)
	}
	if err := queries.SeedAccount(ctx, db.SeedAccountParams{Owner: "Касса Brew North", Balance: 5000}); err != nil {
		return fmt.Errorf("SeedAccount: %w", err)
	}
	fmt.Println("1) Два кассовых счёта засеяны:")
	if err := dump(ctx, queries); err != nil {
		return err
	}

	// 2) Успешный перевод 30.00 со счёта #1 на #2 — обе команды в одной
	// транзакции. Сумма по системе не меняется (инвариант C в ACID).
	fmt.Println("\n2) Перевод 30.00 со счёта #1 на #2 (BEGIN → списать → зачислить → COMMIT):")
	if err := transfer(ctx, queries, pool, 1, 2, 3000); err != nil {
		return fmt.Errorf("успешный перевод неожиданно упал: %w", err)
	}
	fmt.Println("   COMMIT. Состояние:")
	if err := dump(ctx, queries); err != nil {
		return err
	}

	// 3) Неудачный перевод: списываем 20.00 со счёта #1 (получится), но
	// зачисляем на несуществующий счёт #999 — Credit задевает 0 строк. Видим
	// это по RowsAffected и откатываем весь перевод. Списание #1 (реальная
	// частичная работа внутри транзакции) откатывается вместе с ним.
	fmt.Println("\n3) Перевод 20.00 со счёта #1 на НЕсуществующий #999 — должен откатиться целиком:")
	err = transfer(ctx, queries, pool, 1, 999, 2000)
	if err != nil {
		fmt.Printf("   перевод отклонён: %v\n", err)
	}
	fmt.Println("   ROLLBACK. Состояние (как в шаге 2 — списание #1 откатилось вместе с переводом):")
	if err := dump(ctx, queries); err != nil {
		return err
	}

	// Итог: после неудачного перевода сумма по системе та же, что и была —
	// деньги не исчезли и не появились. Это и есть атомарность + консистентность.
	total, err := queries.TotalBalance(ctx)
	if err != nil {
		return fmt.Errorf("TotalBalance: %w", err)
	}
	fmt.Printf("\n4) Сумма по всем счетам: %d.%02d — неизменна с самого начала (ничего не потеряно, ничего не создано).\n",
		total/100, total%100)

	return nil
}

// transfer переводит amount центов со счёта from на счёт to внутри ОДНОЙ
// транзакции: списать (Debit) → зачислить (Credit) → COMMIT. Любая осечка
// (overdraft роняет Debit по CHECK; несуществующий получатель → Credit задел 0
// строк) приводит к ROLLBACK, и ни одна из двух команд не доходит до диска.
func transfer(ctx context.Context, q *db.Queries, pool *pgxpool.Pool, from, to, amount int64) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("Begin: %w", err)
	}
	defer tx.Rollback(ctx) // страховка от раннего выхода; явный Commit ниже решает судьбу

	qtx := q.WithTx(tx)

	// Списание. CHECK (balance >= 0) отвергнет уход в минус (ошибка вернётся
	// сюда, и defer-Rollback откатит транзакцию).
	if _, err := qtx.Debit(ctx, db.DebitParams{Amount: amount, ID: from}); err != nil {
		return fmt.Errorf("списание со счёта #%d: %w", from, err)
	}

	// Зачисление. Если получателя нет — задето 0 строк; деньги ушли бы «в
	// никуда», поэтому откатываем весь перевод.
	n, err := qtx.Credit(ctx, db.CreditParams{Amount: amount, ID: to})
	if err != nil {
		return fmt.Errorf("зачисление на счёт #%d: %w", to, err)
	}
	if n == 0 {
		return fmt.Errorf("счёта-получателя #%d не существует", to)
	}

	return tx.Commit(ctx)
}

// dump печатает все счета по порядку id (баланс в рублях.копейках).
func dump(ctx context.Context, q *db.Queries) error {
	rows, err := q.ListAccounts(ctx)
	if err != nil {
		return fmt.Errorf("ListAccounts: %w", err)
	}
	for _, r := range rows {
		fmt.Printf("   #%d %-20s %d.%02d\n", r.ID, r.Owner, r.Balance/100, r.Balance%100)
	}
	return nil
}

// schemaDDL читает schema.sql юнита (DDL поверх канона: таблица ledger_accounts).
// Путь резолвится через runtime.Caller относительно этого исходника (go:embed не
// дотянется: файл лежит на два уровня выше cmd/demo/).
func schemaDDL() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("runtime.Caller: не удалось определить путь к исходнику")
	}
	path := filepath.Join(filepath.Dir(thisFile), "..", "..", "schema.sql")
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read schema.sql: %w", err)
	}
	return string(b), nil
}
