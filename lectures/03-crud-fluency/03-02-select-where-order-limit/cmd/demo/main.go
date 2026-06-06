// Команда demo юнита 03-02: SELECT с WHERE/ORDER/LIMIT и две пагинации —
// OFFSET против keyset.
//
// Два режима:
//
//	demo          — фильтр меню + проход по страницам keyset'ом + та же страница OFFSET'ом;
//	demo -reset   — накатить канон Brew (schema + seed) и выйти (db-reset).
//
// Запрос read-only по каноническим drinks — вывод детерминирован (5 напитков
// seed'а). Логи — в stderr, stdout — только результат (для вставки в README).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/dsbasko/postgres-cookbook/lectures/03-crud-fluency/03-02-select-where-order-limit/internal/db"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

// sentinel — «сторожевой» курсор для первой keyset-страницы: заведомо больше
// любой реальной (base_price, id), поэтому условие (price, id) < (sentinel,
// sentinel) пропускает все строки. По убыванию это просто «с самого начала».
const sentinel = int64(1) << 62

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
	const pageSize = 2

	// 1) WHERE/ORDER/LIMIT: кофе, по возрастанию цены, не больше pageSize.
	coffee, err := queries.FilterMenu(ctx, db.FilterMenuParams{Category: "coffee", PageSize: pageSize})
	if err != nil {
		return fmt.Errorf("FilterMenu: %w", err)
	}
	fmt.Printf("1) WHERE/ORDER/LIMIT — category='coffee', по возрастанию цены, LIMIT %d:\n", pageSize)
	for _, d := range coffee {
		fmt.Printf("   %s\n", line(d.ID, d.Name, d.BasePrice))
	}

	// 2) Keyset: листаем ВСЁ меню по убыванию цены. Курсор — (цена, id) последней
	// строки предыдущей страницы; первую берём со сторожевым курсором.
	fmt.Printf("\n2) Keyset-пагинация по всему меню (по убыванию цены, page_size=%d):\n", pageSize)
	afterPrice, afterID := sentinel, sentinel
	for page := 1; ; page++ {
		rows, err := queries.PageByKeyset(ctx, db.PageByKeysetParams{
			AfterPrice: afterPrice,
			AfterID:    afterID,
			PageSize:   pageSize,
		})
		if err != nil {
			return fmt.Errorf("PageByKeyset: %w", err)
		}
		if len(rows) == 0 {
			break
		}
		cursor := "∞ (с начала)"
		if page > 1 {
			cursor = fmt.Sprintf("после %d.%02d / #%d", afterPrice/100, afterPrice%100, afterID)
		}
		fmt.Printf("   страница %d (%s): %s\n", page, cursor, joinKeyset(rows))
		last := rows[len(rows)-1]
		afterPrice, afterID = last.BasePrice, last.ID
		if len(rows) < pageSize {
			break
		}
	}

	// 3) OFFSET: та же «страница 2» (пропустить pageSize, взять pageSize).
	off, err := queries.PageByOffset(ctx, db.PageByOffsetParams{PageSize: pageSize, Skip: pageSize})
	if err != nil {
		return fmt.Errorf("PageByOffset: %w", err)
	}
	fmt.Printf("\n3) OFFSET — та же страница 2 через LIMIT %d OFFSET %d:\n", pageSize, pageSize)
	fmt.Printf("   %s\n", joinOffset(off))
	fmt.Println("   → результат тот же, но сервер вычислил и отбросил первые 2 строки; keyset — нет.")

	return nil
}

// line форматирует одну строку меню: «#id Название Ц.КК».
func line(id int64, name string, cents int64) string {
	return fmt.Sprintf("#%d %s %d.%02d", id, name, cents/100, cents%100)
}

func joinKeyset(rows []db.PageByKeysetRow) string {
	parts := make([]string, 0, len(rows))
	for _, r := range rows {
		parts = append(parts, line(r.ID, r.Name, r.BasePrice))
	}
	return strings.Join(parts, " | ")
}

func joinOffset(rows []db.PageByOffsetRow) string {
	parts := make([]string, 0, len(rows))
	for _, r := range rows {
		parts = append(parts, line(r.ID, r.Name, r.BasePrice))
	}
	return strings.Join(parts, " | ")
}
