// Команда demo юнита 04-02: многотабличный JOIN и self-join.
//
// Два режима:
//
//	demo          — чек заказа из 4 таблиц + иерархия персонала self-join'ом;
//	demo -reset   — накатить канон Brew + таблицу staff и выйти (db-reset).
//
// Чек идёт по каноническим orders↔customers↔order_items↔drinks, иерархия — по
// лабораторной staff. Логи в stderr, stdout — только результат (для README).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/dsbasko/postgres-cookbook/lectures/04-querying-across-tables/04-02-multi-table-and-self-joins/internal/db"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
	"github.com/jackc/pgx/v5/pgtype"
)

func main() {
	logger := log.New()

	reset := flag.Bool("reset", false, "накатить канон Brew + таблицу staff и выйти")
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
		fmt.Println("Канон Brew + таблица staff накатаны.")
		return nil
	}

	queries := db.New(pool)

	// 1) Многотабличный JOIN: собираем чек из четырёх таблиц.
	receipt, err := queries.OrderReceipt(ctx)
	if err != nil {
		return fmt.Errorf("OrderReceipt: %w", err)
	}
	fmt.Println("1) Чек заказа — JOIN по 4 таблицам (orders→customers→order_items→drinks):")
	fmt.Printf("   %-5s %-16s %-12s %3s %8s %8s\n", "заказ", "клиент", "напиток", "кол", "цена", "сумма")
	var total int64
	for _, r := range receipt {
		fmt.Printf("   #%-4d %-16s %-12s %3d %8s %8s\n",
			r.OrderID, r.Customer, r.Drink, r.Quantity, money(r.UnitPrice), money(r.LineTotal))
		total += r.LineTotal
	}
	fmt.Printf("   итого по всем позициям: %s\n", money(total))

	// 2) Self-join: таблица staff соединена сама с собой (e=сотрудник, m=руководитель).
	if err := queries.TruncateStaff(ctx); err != nil {
		return fmt.Errorf("TruncateStaff: %w", err)
	}
	if err := queries.SeedStaff(ctx); err != nil {
		return fmt.Errorf("SeedStaff: %w", err)
	}
	staff, err := queries.StaffWithManager(ctx)
	if err != nil {
		return fmt.Errorf("StaffWithManager: %w", err)
	}
	fmt.Println("\n2) Иерархия персонала — self-join staff (e=сотрудник, m=руководитель):")
	fmt.Printf("   %-8s %-12s %s\n", "сотрудник", "роль", "руководитель")
	for _, r := range staff {
		fmt.Printf("   %-8s %-12s %s\n", r.Employee, r.Role, manager(r.Manager))
	}
	fmt.Println("   → у Анны руководителя нет (manager = NULL) — LEFT JOIN её не выкинул.")

	return nil
}

// money форматирует цену в центах как «Ц.КК».
func money(cents int64) string {
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}

// manager печатает имя руководителя или «— (старший)», если его нет (вершина
// иерархии: manager_id NULL → m.name NULL после LEFT JOIN).
func manager(t pgtype.Text) string {
	if !t.Valid {
		return "— (старший)"
	}
	return t.String
}

// schemaDDL читает schema.sql юнита (DDL поверх канона: таблица staff). Путь
// резолвится через runtime.Caller относительно этого исходника (go:embed не
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
