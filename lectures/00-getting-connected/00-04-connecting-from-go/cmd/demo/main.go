// Команда demo юнита 00-03: первый запрос из Go и почему параметры — не опция.
//
// Два режима:
//
//	demo          — поиск напитков по категории, штатно ($1) и на анти-демо инъекции;
//	demo -reset   — накатить канон Brew (schema + seed) и выйти (цель db-reset).
//
// Это raw-pgx юнит (escape-hatch до sqlc): запрос пишем строкой и руками
// разбираем строки через rows.Scan — чтобы в 00-04 увидеть, какой именно
// boilerplate за нас сгенерирует sqlc. Логи идут в stderr (internal/log), в
// stdout — только результат, чтобы вывод дословно лёг в README.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dsbasko/postgres-cookbook/lectures/internal/brew"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/log"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/pg"
	"github.com/dsbasko/postgres-cookbook/lectures/internal/runctx"
)

// drink — одна строка меню. В raw-pgx мы сами описываем структуру под результат
// и сами раскладываем колонки в её поля (см. queryDrinks). В 00-04 эту структуру
// и Scan сгенерирует sqlc из схемы.
type drink struct {
	id        int64
	sku       string
	name      string
	category  string
	basePrice int64
}

// baseSelect — общая часть запроса к меню. Ниже мы добавим к ней WHERE двумя
// способами: безопасно (параметр $1) и небезопасно (склейка строкой) — чтобы
// увидеть разницу на одном и том же вводе.
const baseSelect = "SELECT id, sku, name, category, base_price FROM drinks"

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
	// Пул ленивый: соединения тут ещё нет — оно установится при Ping/запросе.
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

	// 1) Штатный путь: параметризованный запрос. Плейсхолдер $1 в тексте SQL,
	// значение — отдельным аргументом. Драйвер отправляет их раздельно, и
	// сервер НИКОГДА не парсит значение как часть SQL.
	const safeSQL = baseSelect + " WHERE category = $1 ORDER BY id"
	coffee, err := queryDrinks(ctx, pool, safeSQL, "coffee")
	if err != nil {
		return fmt.Errorf("параметризованный поиск: %w", err)
	}

	fmt.Println("1) Параметризованный поиск: category = $1, значение 'coffee' — штатный путь.")
	printDrinks(coffee)

	// 2) Анти-демо инъекции. Классический злонамеренный ввод: закрыть кавычку и
	// дописать своё условие. Если такой текст склеить в SQL строкой, он станет
	// частью запроса.
	const malicious = "' OR 1=1 --"
	fmt.Printf("\n2) Злонамеренный ввод в поле «категория»:  %s\n\n", malicious)

	// Небезопасно: значение склеено в текст SQL. Получаем
	//   ... WHERE category = '' OR 1=1 --' ...
	// — условие category всегда истинно, остаток запроса закомментирован.
	unsafeSQL := baseSelect + " WHERE category = '" + malicious + "' ORDER BY id"
	leaked, err := queryDrinks(ctx, pool, unsafeSQL)
	if err != nil {
		return fmt.Errorf("небезопасный запрос: %w", err)
	}
	fmt.Printf("   Небезопасно (склейка строкой): запросили одну категорию — сервер вернул %d строк (вся таблица утекла).\n", len(leaked))

	// Безопасно: тот же ввод, но как параметр $1. Сервер трактует его как
	// литеральное значение категории, а не как SQL — совпадений нет.
	neutralized, err := queryDrinks(ctx, pool, safeSQL, malicious)
	if err != nil {
		return fmt.Errorf("безопасный запрос с тем же вводом: %w", err)
	}
	fmt.Printf("   Безопасно ($1 как параметр): тот же ввод — это литерал категории, совпадений нет, %d строк.\n", len(neutralized))

	return nil
}

// queryDrinks выполняет запрос и руками раскладывает строки в []drink. Это и
// есть тот boilerplate (Query → for rows.Next → Scan → rows.Err), который в
// 00-04 за нас сгенерирует sqlc.
func queryDrinks(ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) ([]drink, error) {
	rows, err := pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []drink
	for rows.Next() {
		var d drink
		if err := rows.Scan(&d.id, &d.sku, &d.name, &d.category, &d.basePrice); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func printDrinks(drinks []drink) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSKU\tНАЗВАНИЕ\tКАТЕГОРИЯ\tЦЕНА")
	for _, d := range drinks {
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%d.%02d\n",
			d.id, d.sku, d.name, d.category, d.basePrice/100, d.basePrice%100)
	}
	_ = w.Flush()
}
