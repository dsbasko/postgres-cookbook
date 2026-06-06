// Команда demo юнита 01-02: text, boolean и тизер NULL.
//
// Два режима:
//
//	demo          — NULL-логика, count(*) vs count(col), boolean, text vs char(n);
//	demo -reset   — накатить канон Brew (schema + seed) и выйти (цель db-reset).
//
// Главный вывод: NULL — это «неизвестно», а не «пусто»; сравнение с ним даёт
// NULL, а не true/false, поэтому проверяем через IS NULL. Логи — в stderr,
// stdout — только результат (для дословной вставки в README).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/dsbasko/postgres-cookbook/lectures/01-data-types/01-02-text-boolean-and-null-teaser/internal/db"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

func main() {
	logger := log.New()

	reset := flag.Bool("reset", false, "накатить канон Brew (schema + seed) и выйти")
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
		if err := brew.Reset(ctx, pool); err != nil {
			return fmt.Errorf("brew.Reset: %w", err)
		}
		fmt.Println("Канон Brew накатан: схема + seed-данные на месте.")
		return nil
	}

	queries := db.New(pool)

	// 1) NULL в сравнении: (NULL = NULL) — не TRUE и не FALSE, а NULL.
	nc, err := queries.NullComparison(ctx)
	if err != nil {
		return fmt.Errorf("NullComparison: %w", err)
	}
	fmt.Println("1) (NULL = NULL) — это не TRUE и не FALSE, а NULL («неизвестно»):")
	fmt.Printf("   (NULL = NULL) IS NOT TRUE = %v;  IS NULL = %v\n", nc.EqNullIsNotTrue, nc.EqNullIsUnknown)
	fmt.Println("   → отсутствие значения проверяем через IS NULL, не через = NULL.")

	// 2) LEFT JOIN даёт настоящие NULL: у клиента без заказов order_id отсутствует.
	rows, err := queries.CustomersWithOrders(ctx)
	if err != nil {
		return fmt.Errorf("CustomersWithOrders: %w", err)
	}
	fmt.Println("\n2) LEFT JOIN customers ↔ orders — order_id у клиента без заказов = NULL:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "CUSTOMER_ID\tИМЯ\tORDER_ID")
	for _, r := range rows {
		orderID := "NULL"
		if r.OrderID.Valid {
			orderID = fmt.Sprintf("%d", r.OrderID.Int64)
		}
		fmt.Fprintf(w, "%d\t%s\t%s\n", r.CustomerID, r.Name, orderID)
	}
	if err := w.Flush(); err != nil {
		return err
	}

	// 3) count(*) vs count(col): count(o.id) пропускает NULL-строку.
	cnt, err := queries.CountStarVsCol(ctx)
	if err != nil {
		return fmt.Errorf("CountStarVsCol: %w", err)
	}
	fmt.Printf("\n3) count(*) = %d (все строки), count(o.id) = %d (без NULL-заказа Карины)\n",
		cnt.RowsTotal, cnt.RowsWithOrder)

	// 4) boolean прямо из предиката base_price > 400.
	menu, err := queries.MenuPremiumFlag(ctx)
	if err != nil {
		return fmt.Errorf("MenuPremiumFlag: %w", err)
	}
	fmt.Println("\n4) boolean из выражения base_price > 400 (в Go это bool):")
	w2 := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w2, "ID\tНАЗВАНИЕ\tIS_PREMIUM")
	for _, d := range menu {
		fmt.Fprintf(w2, "%d\t%s\t%v\n", d.ID, d.Name, d.IsPremium)
	}
	if err := w2.Flush(); err != nil {
		return err
	}

	// 5) text vs char(n): хвостовой пробел значим в text, но «съедается» в char(n).
	te, err := queries.TextEquality(ctx)
	if err != nil {
		return fmt.Errorf("TextEquality: %w", err)
	}
	fmt.Println("\n5) text сравнивает по байтам, char(n) дополняет пробелами:")
	fmt.Printf("   'abc' = 'abc '           → %v  (text: пробел значим)\n", te.TextTrailingSpaceEq)
	fmt.Printf("   'abc'::char(5) = 'abc  ' → %v   (char(n): паддинг съел пробелы)\n", te.CharPaddedEq)

	return nil
}
