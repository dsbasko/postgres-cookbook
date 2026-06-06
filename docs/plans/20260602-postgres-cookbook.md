# postgres-cookbook — построение репозитория курса «PostgreSQL 18 для разработчиков»

## Overview

Построить с нуля репозиторий **postgres-cookbook** — практический курс «PostgreSQL 18 для разработчиков» (только сторона разработки, НЕ DBA/DevOps), по образцу соседнего курса `kafka-cookbook`. Идея та же, что в Kafka-курсе: **курс-как-данные на переиспользуемом движке** — каждый юнит самодостаточен, запускается локально и оставляет наблюдаемый след, а сайт рендерит опубликованный npm-движок `@dsbasko/cookbook-engine`.

- **Проблема, которую решаем:** нет связного, практико-ориентированного курса по Postgres именно для разработчиков приложений (а не админов), с запускаемым кодом и единым нарративом.
- **Ключевые выгоды:** переиспользование готовой инфраструктуры сайта/деплоя/Makefile (экономия недель работы); общая вселенная с `kafka-cookbook` (нарратив «Brew»), где капстон postgres-курса буквально передаёт «эстафету» в Kafka-курс через CDC.
- **Интеграция:** репозиторий-сосед к `kafka-cookbook`; тот же движок, та же педагогика, байт-совместимые схемы Brew.

Эталон-источник: `/Users/dsbasko/Develop/dsbasko/kafka-cookbook`. Целевая директория `/Users/dsbasko/Develop/dsbasko/postgres-cookbook` **пуста и не под git** — первый шаг включает `git init`.

## Context (from discovery)

**Файлы/компоненты эталона (kafka-cookbook), проверены на диске:**

- `web/` — 12 route-файлов голых ре-экспортов из `@dsbasko/cookbook-engine`: `app/{layout,page,not-found,icon,opengraph-image}.tsx`, `app/{robots,sitemap}.ts`, `app/[lang]/{layout,page,not-found}.tsx`, `app/[lang]/[module]/page.tsx`, `app/[lang]/[module]/[lesson]/page.tsx`; `next.config.mjs` = `createCookbookConfig()`; `package.json`, `tsconfig.json`.
- `.github/workflows/deploy.yml` — единственный workflow, репо-агностичный.
- `lectures/internal/` — **отдельный Go-модуль** (свой `go.mod`/`go.sum`): `config/env.go`, `log/log.go`, `runctx/runctx.go` (переиспользуем дословно) + `kafka/{client,admin}.go` (заменяем на `pg/`).
- `course.yaml` — манифест с секцией `brand` и `modules[]→lessons[]{slug,title.{ru,en},duration,tags}`.
- Образец юнита `lectures/04-reliability/04-03-outbox-pattern/`: `go.mod`/`go.sum`, `README.md` (заглушка), `i18n/{ru,en}/README.md`, `Makefile`, `cmd/<bin>/main.go`, `db/init.sql`, `docker-compose.override.yml`.

**Байт-совместимый канон Brew (из реальных init.sql эталона):**

- `04-03-outbox-pattern/db/init.sql`: `orders(id BIGSERIAL PK, customer_id TEXT, amount NUMERIC, status TEXT DEFAULT 'created', created_at TIMESTAMPTZ)`; `outbox(id BIGSERIAL PK, aggregate_id TEXT, topic TEXT, payload JSONB, created_at, published_at)` + partial index `outbox_unpublished_idx ON (id) WHERE published_at IS NULL`; `processed_outbox_ids(outbox_id BIGINT PK, processed_at)`.
- `09-use-cases/04-pg-to-elasticsearch/db/init.sql`: `drinks(id BIGINT PK, sku, name, description, category, base_price BIGINT, stock INT, created_at, updated_at)`; `articles(id BIGINT PK, title, body, author, tags TEXT, published_at, created_at)`; `customers(id BIGINT PK, phone, name, email, created_at)`; все три `REPLICA IDENTITY FULL`; `CREATE PUBLICATION dbz_publication FOR TABLE drinks, articles, customers`.
- Образец postgres-сервиса для песочницы: `lectures/09-use-cases/04-pg-to-elasticsearch/docker-compose.override.yml`.

> ⚠️ **Реконсиляция имён (важно).** On-disk канон использует `customers.id BIGINT` и `drinks.base_price BIGINT`. Это расходится с черновыми предложениями брейншторма (`customers.id uuid DEFAULT uuidv7()`, `price_cents GENERATED`). **Правило:** 6 CDC-релевантных таблиц (`orders`, `outbox`, `processed_outbox_ids`, `drinks`, `articles`, `customers`) держим байт-совместимыми ДОСЛОВНО — иначе сломается handoff 10-05. PG18-фичи (uuidv7, virtual generated columns) демонстрируем на НОВЫХ таблицах (`shops`, `order_items`, `inventory`, pricing-таблицы), где нет ограничения совместимости.

**Паттерны/конвенции эталона:**

- Логи → stderr, чистый stdout (нужно для педагогики «вставленный фактический вывод»).
- `internal/` — общий Go-модуль, юниты тянут его через `replace ../../internal` в своём `go.mod`.
- Makefile help-first, `?=` overridable vars, цели названы по действию читателя.
- `make web-check-coverage` сверяет `course.yaml` ↔ файловую систему; `make web-generate-readme-toc` регенерит корневой TOC.

**Зависимости:** Node ≥20 + pnpm 9.15.0, Go 1.26, Docker (postgres:18-alpine), `sqlc`, `psql` (libpq), `@dsbasko/cookbook-engine` (npm, **точный пин `1.0.0`** — как в эталонном `web/package.json`, без каретки; обновлять осознанным бампом).

## Зафиксированные решения (из брейншторма)

1. **Стек:** Go + pgx (jackc/pgx v5 + pgxpool) в каждом юните; **sqlc — протагонист** (пишем `query.sql` руками → `sqlc generate` → типизированный pgx-код, коммитим).
2. **Нарратив:** «Brew», каждый юнит самодостаточен; сущности байт-совместимы с kafka-cookbook.
3. **Гранулярность:** 11 модулей × 4–6 юнитов ≈ ~60 юнитов, ~30–35 ч.
4. **Схема:** общий baseline (`schema/brew.sql` + `schema/seed.sql`) + per-unit добавки (`schema.sql`); `make db-reset` накатывает baseline → добавки → seed.
5. **PL/pgSQL:** средняя глубина, всё внутри модуля 09; явная секция «когда НЕ класть логику в БД».
6. **PG18:** «просто версия» — современные фичи как «текущий способ», без бейджей и вводного модуля.
7. **Языки:** RU-first; DoD юнита = есть И ru, И en README.

## Development Approach

- **Testing approach:** Regular + **verification-gate** (адаптировано под content-репозиторий). Классический «unit-тест на каждую задачу» применяется к Go-инфраструктуре (`internal/pg`, `internal/brew`) и капстонам (asserted integration-тесты, как в kafka 09). Для контентных юнитов «тест» = воспроизводимая верификация (см. Testing Strategy) — это эквивалент той же строгости для прозы+SQL.
- Завершать каждую задачу полностью перед переходом к следующей; маленькие сфокусированные изменения.
- **CRITICAL:** обновлять этот план при изменении scope.
- **CRITICAL:** все проверки задачи зелёные перед началом следующей.
- Сохранять обратную совместимость канона Brew (байт-совместимые имена колонок).

## Testing Strategy

Маппинг «что значит проверено» по типу артефакта:

- **Go-инфраструктура** (`internal/pg`, `internal/brew`): обычные Go unit-тесты (success + error), `go test ./...` зелёный. Для `internal/brew` — тест идемпотентности применения схемы.
- **Контентный юнит** (модули 00–09): verification-gate — все пункты обязательны перед «готов»:
  - `make db-reset` идемпотентен (повторный прогон не падает);
  - `make gen` (`sqlc generate`) не даёт diff (сгенерённый код закоммичен и актуален);
  - `make run` отрабатывает и выдаёт ровно тот вывод, что вставлен в `## Запуск` README;
  - `make build` (`go build ./...`) зелёный;
  - присутствуют ОБА `i18n/ru/README.md` и `i18n/en/README.md`;
  - `make web-check-coverage` зелёный (course.yaml ↔ ФС сходятся).
- **Капстоны** (модуль 10): asserted Go integration-тесты против песочницы (`go test`), плюс всё из verification-gate.
- **Сайт целиком:** `make web-typecheck`, `make web-lint`, `make web-build` (статический экспорт, проверка `web/out/404.html`).

## Progress Tracking

- `[x]` сразу по завершении пункта.
- ➕ — новые задачи; ⚠️ — блокеры.
- Обновлять план при отклонении от scope.

> **Масштаб.** Это многонедельный курс (~60 юнитов). План детализирует фазы 0–2 (одноразовая инфраструктура + золотые шаблоны) по-задачно; фаза 3 (авторинг ~55 юнитов) идёт по повторяемому per-unit рецепту + модульным чек-листам (1 чекбокс = 1 юнит). Курс можно публиковать инкрементально по мере готовности модулей.

## Solution Overview

Высокоуровнево репозиторий состоит из четырёх слоёв (три переиспользуются от kafka-cookbook):

```
postgres-cookbook/
├── docker-compose.yml          # REBUILD: postgres:18-alpine + Adminer
├── course.yaml                 # ADAPT: брендинг Brew→Postgres, brand.accent #336791
├── README.md  CLAUDE.md        # NEW: TOC, getting-started, авторские конвенции, кросс-ссылка
├── Makefile                    # ADAPT: web-* дословно, lecture-цели; без connect-*
├── package.json                # ADAPT: name → postgres-cookbook
├── pnpm-workspace.yaml         # ADAPT: packages: [web]
├── .gitignore .github/         # ADAPT/COPY: deploy.yml дословно
├── schema/                     # NEW: brew.sql (baseline, байт-совместимый) + seed.sql
├── web/                        # DIRECT: тонкая обёртка над @dsbasko/cookbook-engine
└── lectures/
    ├── go.work · Makefile      # ADAPT
    ├── internal/               # config/log/runctx (COPY) + pg + brew (NEW), свой go.mod
    └── 00-…/ … 10-use-cases/   # NEW: весь контент (~60 юнитов)
```

**Ключевые решения и обоснование:**

- Сайт/деплой переиспользуются как зависимость → обновление сайта = бамп версии движка.
- Канон Brew байт-совместим с kafka-cookbook → реальная общая вселенная и handoff 10-05.
- sqlc держит SQL в главной роли (урок = `query.sql`), убирая boilerplate сканирования.
- Escape hatch для интерактивных/DDL/EXPLAIN/конкурентных уроков (psql-сессии или raw-pgx) — sqlc дефолт, но не догма.

## Technical Details

**Анатомия контентного юнита** (`lectures/<NN-module>/<MM-slug>/`):

```
go.mod                  # module github.com/dsbasko/postgres-cookbook/lectures/<NN>/<MM>, replace ../../internal
README.md               # 3-строчная заглушка выбора языка (как в kafka-cookbook)
i18n/{ru,en}/README.md  # сам урок
Makefile                # help · run · gen · db-reset · db-shell · build · clean
sqlc.yaml               # engine: postgresql; sql_package: pgx/v5; out: internal/db
schema.sql              # DDL-добавки именно этого юнита (поверх schema/brew.sql)
query.sql               # ★ SQL руками — протагонист урока
internal/db/            # сгенерённый sqlc-код (db.go, models.go, query.sql.go) — коммитим
cmd/demo/main.go        # тонкий: pgxpool → db.New → типизир. запрос → tabwriter
```

**README-дуга** (инвариант kafka-cookbook): холодный вход историей Brew → ограниченная цель → один концепт на `##` (привязан к инциденту) → `## Что показывает наш код` (центр — `query.sql`, затем ~5 строк `main.go`) → `## Запуск` (точные make-команды + вставленный фактический вывод) → `## Что забрать с собой` + ссылка на следующий юнит. ~65% проза / 25% код / 10% команды.

**Escape-hatch вариант юнита** (интерактив/EXPLAIN/конкурентность/DDL): вместо `query.sql`+sqlc — `session-a.sql`/`session-b.sql` (или `run.sql`) через psql в Makefile, либо raw-pgx (`conn.Query/Exec`) в `main.go`. README показывает interleaving/план.

**internal/pg** (новый домен-хелпер): `pg.NewPool(ctx, opts ...Option) (*pgxpool.Pool, error)` — читает `DATABASE_URL`/`PG*` env, дефолты под песочницу, opts-escape-hatch. Форма по аналогии с `internal/kafka.NewClient`.

**internal/brew:** `brew.Reset(ctx, pool)` / `brew.Apply(ctx, pool, extraDDL ...string)` — накатывает `schema/brew.sql` + `schema/seed.sql` (+ per-unit `schema.sql`); идемпотентно.

**schema/brew.sql** (канон): 6 байт-совместимых таблиц ДОСЛОВНО из эталонных init.sql (`orders`, `outbox`, `processed_outbox_ids`, `drinks`, `articles`, `customers` + partial index `outbox_unpublished_idx` + `REPLICA IDENTITY FULL` где было) **плюс** новые таблицы для богатых примеров: `shops`, `order_items`, `inventory`. PG18-демо (uuidv7/generated) — только на новых таблицах.

**Гардрейлы точности (при авторинге контента):**

- `JSON_TABLE` — это **PG17, не PG18**; не приписывать 18.
- «AIO до 3x» — подавать как «до ~2–3x на read-heavy scans».
- PG18 OAuth — Go-драйверы незрелы; только краткое упоминание-aside, без урока.
- `EXPLAIN ANALYZE` в PG18 показывает buffers по умолчанию — отметить в 06-01.

## What Goes Where

- **Implementation Steps** (`[ ]`): всё внутри репозитория — копирование/адаптация файлов, Go-код, схемы, контент юнитов, тесты, верификация сборки.
- **Post-Completion** (без чекбоксов): включение GitHub Pages в Settings, проверка реального стриминга 10-05 со стороны kafka-cookbook (Debezium), ручной визуальный обзор сайта, публикация/пин версии движка.

---

## Implementation Steps

## ФАЗА 0 — Bootstrap shell (переиспользуемая оболочка сайта)

### Task 0.1: Инициализация репо и копирование web-оболочки

**Files:**
- Create: `.git/` (`git init`)
- Create: `.gitignore` (адаптировать из kafka-cookbook — см. ⚠️ ниже)
- Copy: `web/**` ← `kafka-cookbook/web/{app,next.config.mjs,package.json,tsconfig.json,next-env.d.ts,.eslintrc.json,.prettierrc}` (БЕЗ `node_modules`, `.next`, `out`, `tsconfig.tsbuildinfo`)
- Copy: `.github/workflows/deploy.yml` ← дословно
- Create: `package.json`, `pnpm-workspace.yaml` (адаптировать)
- Create: `Makefile` (только секция `web-*` на этой фазе)

- [x] `git init` в корне; создать `.gitignore` (node_modules, .next, out, web/public/static/lectures кэш, *.tsbuildinfo). ⚠️ **НЕ копировать `.gitignore` дословно:** эталон содержит `/docs/` (строка 53) — он бы заигнорил ЭТОТ план; обязательно убрать `/docs/` (или добавить `!docs/plans/`). Убрать Kafka-only строки: `connect-plugins/*`, все `lectures/*/*/<kafka-бинарь>` (producer/consumer/courier/…); добавить вместо них `lectures/*/*/demo` (имя нашего бинаря `cmd/demo`)
- [x] скопировать `web/` дословно (включая `.eslintrc.json` + `.prettierrc` — нужны для `web-lint`); в `web/package.json` поменять только `name` → `postgres-cookbook-web` (зависимости оставить как есть; это сохраняет точный пин `@dsbasko/cookbook-engine@1.0.0`)
- [x] скопировать `.github/workflows/deploy.yml` без изменений
- [x] создать корневой `package.json` (`name: postgres-cookbook`, workspace-маркер) и `pnpm-workspace.yaml` (`packages: [web]`)
- [x] создать корневой `Makefile`: перенести цели `web-install/web-dev/web-build/web-lint/web-typecheck/web-clean/web-check-coverage/web-generate-readme-toc` дословно; НЕ переносить `connect-*` и `DEBEZIUM/CLICKHOUSE/ES_VERSION`
- [x] verification: `git status` чистый от игнор-файлов; структура `web/app` содержит все 12 route-файлов

### Task 0.2: Авторинг course.yaml (брендинг + скелет 11 модулей)

**Files:**
- Create: `course.yaml`

- [x] заполнить шапку: `title: PostgreSQL Cookbook`, `description.{ru,en}`, `basePath: /postgres-cookbook`, `repoUrl: https://github.com/dsbasko/postgres-cookbook`
- [x] секция `brand`: `glyph: 'P'`, `level: 'Go'`, `siteUrl: https://dsbasko.github.io`, `breadcrumbRoot`, `hero{lead,accent,tail}` (ru/en), `ogImage{title, footerTag: "PostgreSQL · Go"}`
- [x] **добавить** `brand.accent: '#336791'` + `brand.accentDark: '#5a9fd4'` (синий Postgres — единственный рычаг перекраски UI; в Kafka намеренно опущен)
- [x] объявить все 11 модулей с `id`/`title.{ru,en}`/`description.{ru,en}` и пустыми (или плейсхолдер) `lessons` — заполнять lessons помодульно в фазе 3
- [x] verification: `course.yaml` — валидный YAML; `id` модулей совпадут с будущими папками `lectures/`

### Task 0.3: Проверка сборки оболочки

**Files:** (только проверки, без новых файлов)

- [x] `pnpm install` из корня (corepack не поставляется со сборкой node 26 в Homebrew → использован глобальный pnpm 9.15.9, та же линия 9.15.x; сгенерён и закоммичен `pnpm-lock.yaml`, нужен для `--frozen-lockfile` в deploy.yml)
- [x] `make web-dev` — dev-сервер на :3000 отдаёт 200 на `/` и `/ru`; в `/ru` присутствуют «PostgreSQL Cookbook», accent `#336791` и footer-tag «PostgreSQL · Go» (`/en` → 308 — штатный trailing-slash редирект)
- [x] `make web-build` — статический экспорт прошёл (54 страницы); `web/out/404.html` присутствует; accent `#336791` и брендинг в выводе
- [x] `make web-typecheck` и `make web-lint` зелёные
- [x] ⚠️ `web-check-coverage` НЕ запускался на этой фазе (отложен до Task 2.1). Подтверждено: движок требует ≥1 урок в каждом модуле (`modules[N].lessons must be a non-empty array`) → для `web-build` создан временный плейсхолдер-урок в каждом из 11 модулей (course.yaml + `i18n/{ru,en}/README.md`), проверена сборка, затем плейсхолдеры удалены (`git checkout course.yaml` + `rm -rf lectures`) — рабочее дерево чистое, реальный первый урок придёт в Task 2.1

---

## ФАЗА 1 — Shared foundation (песочница + internal + канон схемы)

### Task 1.1: Песочница docker-compose (postgres:18 + Adminer)

**Files:**
- Create: `docker-compose.yml`

- [x] сервис `postgres` (`postgres:18-alpine`, `POSTGRES_DB/USER/PASSWORD`, healthcheck `pg_isready`, named volume, bind `127.0.0.1:5432:5432`) — взять за основу `kafka-cookbook/lectures/09-use-cases/04-pg-to-elasticsearch/docker-compose.override.yml`. ⚠️ PG18 сменил конвенцию: том монтируется на `/var/lib/postgresql` (а не `.../data`), иначе контейнер падает с ошибкой про major-version-подкаталог. Добавлен `wal_level=logical` + слоты/сендеры в базу (нужно 09/10-05, чтобы не плодить override)
- [x] сервис `adminer` (`adminer:4.8.1`) как веб-UI — зеркало роли kafka-ui, порт на loopback `127.0.0.1:8090:8080`, `ADMINER_DEFAULT_SERVER: postgres`, `depends_on` postgres healthy
- [x] задокументировать env-дефолты (`DATABASE_URL=postgres://brew:brew@localhost:5432/brew?sslmode=disable` + эквивалентные `PG*`) в шапке-комментарии
- [x] verification: `docker compose up -d` → postgres healthy; `pg_isready` accepting; `SELECT version()` = PostgreSQL 18.4; `SHOW wal_level` = logical; host-psql через loopback отвечает; Adminer `http://localhost:8090` → HTTP 200. Стенд снесён (`down -v`) после проверки

### Task 1.2: Скаффолдинг lectures/ (go.work, Makefile, internal go.mod)

**Files:**
- Create: `lectures/go.work`
- Create: `lectures/Makefile`
- Create: `lectures/internal/go.mod`

- [x] `lectures/internal/go.mod`: `module github.com/dsbasko/postgres-cookbook/lectures/internal`, Go 1.26, зависимости pgx v5 (`github.com/jackc/pgx/v5 v5.9.2`). ⚠️ Модуль-корень `postgres-cookbook` — НОВЫЙ путь (kafka-cookbook использует `github.com/dsbasko/kafka-sandbox`, имя репо ≠ модуль-путь); `replace ../../internal` в юнитах писать заново, НЕ копировать
- [x] `lectures/go.work`: написан ЗАНОВО (`go 1.26`, `use ./internal`; юниты добавляются по мере создания). НЕ скопирован kafka `go.work` — его `replace google.golang.org/genproto` нужен только pebble (07-02)
- [x] `lectures/Makefile`: цель `list` (дерево юнитов) + `lecture` с **проброшенным таргетом** `T ?= run`: `$(MAKE) -C "<dir>" $(T)`; `make lecture L=<slug>` по умолчанию запускает `run` (демо), `T=help` показывает справку юнита. Гард `ifndef L` и проверка существования директории на месте. Также перенесены `sync`/`build` (build собирает каждый workspace-модуль отдельно)
- [x] verification: `cd lectures && go work sync` → exit 0, require pgx сохранён; `make list` отрабатывает (пусто — юнитов ещё нет); `make help`, `make lecture` (без L → error), `make lecture L=nonexistent` (not found), `make sync`, `make build` — все ведут себя корректно

### Task 1.3: internal/{config,log,runctx} (копия) + internal/pg (новый)

**Files:**
- Copy: `lectures/internal/config/env.go`, `lectures/internal/log/log.go`, `lectures/internal/runctx/runctx.go` ← дословно из kafka-cookbook
- Create: `lectures/internal/pg/pool.go`
- Create: `lectures/internal/pg/pool_test.go`

- [x] скопировать `config/log/runctx` (код дословно: API/поведение MustEnv/EnvOr, stderr-логгер, signal-context идентичны эталону). ⚠️ Иллюстративные комментарии адаптированы под Postgres-контекст (вместо `kafka-console-producer`/«кафка-клиенты» — psql/`cmd/demo`/«пул соединений»), чтобы в Postgres-курсе не было Kafka-ссылок; конвенция «логи в stderr, чистый stdout» сохранена
- [x] `pg.NewPool(ctx, opts...)` поверх `pgxpool.ParseConfig`+`NewWithConfig`: `DSN()` читает `DATABASE_URL`, иначе собирает из `PG*` с дефолтами под песочницу; `Option`-escape-hatch (`WithMaxConns`); возвращает `*pgxpool.Pool, error`. Пул ленивый (как `kafka.NewClient`) — соединение проверяется через `pool.Ping`
- [x] тесты `pg.NewPool`: success (интеграционный против песочницы — подтверждён против живого postgres:18, `t.Skip` если БД недоступна, чтобы `go test` был зелёным без стенда) + error (битый DSN — невалидный `%zz`, падает в `ParseConfig`)
- [x] тест дефолтов env (`TestDSN`, table-driven: дефолты под песочницу / приоритет `DATABASE_URL` / сборка из `PG*` / частичный override)
- [x] `go test ./internal/pg/...` зелёный — пройден (gofmt/go vet/go build тоже чистые; success-путь верифицирован против живого Postgres 18, стенд снесён `down -v`)

### Task 1.4: internal/brew + schema/brew.sql + schema/seed.sql (канон)

**Files:**
- Create: `schema/brew.sql`
- Create: `schema/seed.sql`
- Create: `lectures/internal/brew/brew.go`
- Create: `lectures/internal/brew/brew_test.go`

- [x] `schema/brew.sql`: 6 байт-совместимых таблиц ДОСЛОВНО (`orders`, `outbox` + `outbox_unpublished_idx`, `processed_outbox_ids`, `drinks`, `articles`, `customers` + `REPLICA IDENTITY FULL`) **плюс** новые `shops`, `order_items`, `inventory` (FK: `order_items`→`orders`/`drinks`, `inventory`→`shops`/`drinks`; новые таблицы на `GENERATED ALWAYS AS IDENTITY`). Все 6 канон-таблиц транскрибированы из эталонных init.sql дословно; `REPLICA IDENTITY FULL` только на 3 CDC-источниках (drinks/articles/customers), verified против живой PG18 (`relreplident='f'`, orders='d'). Публикация `dbz_publication` НЕ в baseline — она специфична для handoff-юнита 10-05
- [x] ⚠️ НЕ переименовывать существующие колонки (`orders.customer_id TEXT`, `drinks.base_price BIGINT`, `customers.id BIGINT`); uuidv7/generated демонстрировать на новых таблицах. Колонки канона не тронуты; защищены тестом `TestBrewSchema_ByteCompatCanon` (DB-free, падает при любом переименовании канон-колонки). Новые таблицы (без CDC-ограничения) держат современные идиомы PG18 (IDENTITY)
- [x] `schema/seed.sql`: детерминированные демо-данные Brew (2 shops, 5 drinks, 2 articles, 3 customers, 3 orders, 4 order_items, 5 inventory) — явные id + явные `created_at` для воспроизводимого вывода; `TRUNCATE ... RESTART IDENTITY CASCADE` в начале + `setval` для `orders`-sequence
- [x] `brew.Reset/Apply`: `Apply(ctx, pool, extraDDL...)` накатывает brew.sql → per-unit extra DDL → seed.sql (порядок baseline→добавки→seed); `Reset = Apply()`. Schema-каталог резолвится через `runtime.Caller` (override `BREW_SCHEMA_DIR`); multi-statement .sql через simple protocol pgx (no-args Exec). Идемпотентно (IF NOT EXISTS + TRUNCATE-reseed)
- [x] написать тесты: применение success + идемпотентность (двойной Reset, стабильные счётчики строк) + наличие всех 9 таблиц (`to_regclass`) + Apply с extra DDL идемпотентен; плюс DB-free байт-compat гард и проверка существования/непустоты schema-файлов. Интеграционные тесты `t.Skip` при недоступной БД
- [x] `go test ./internal/brew/...` зелёный — пройден (DB-free тесты + интеграционные против живой postgres:18, стенд снесён `down -v`; `gofmt`/`go vet`/`go build` чистые)

---

## ФАЗА 2 — Reference units (золотые шаблоны)

### Task 2.1: Юнит 00-01 как эталон sqlc-юнита (end-to-end)

**Files:**
- Create: `lectures/00-getting-connected/00-01-client-server-and-sandbox/` (go.mod, README.md, i18n/{ru,en}/README.md, Makefile, sqlc.yaml, schema.sql, query.sql, internal/db/*, cmd/demo/main.go)

- [x] собрать полную анатомию юнита (см. Technical Details); `query.sql` = `SELECT version()` (ServerVersion) + `count(*)`/`SELECT ... FROM drinks` к seed-данным Brew (CountDrinks/ListDrinks). Все 14 файлов на месте, в go.work добавлен `use ./00-getting-connected/00-01-client-server-and-sandbox`
- [x] **заморожен КАНОНИЧЕСКИЙ `sqlc.yaml`**: `version: "2"`, `engine: postgresql`, `queries: query.sql`, `schema: [../../../schema/brew.sql, schema.sql]` (три уровня вверх = корень репо, проверено), `gen.go` с `sql_package: pgx/v5`, `out: internal/db` и зафиксированным набором emit-флагов (`emit_json_tags`, `emit_interface`, `emit_empty_slices`, `emit_exact_table_names: false`). sqlc — `go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0`; `make gen` стабилен (повторный прогон без diff, gofmt чистый), `internal/db/{db,models,querier,query.sql}.go` закоммичены
- [x] `Makefile`: `help`(default) · `run` (go run ./cmd/demo) · `gen` (sqlc generate) · `db-reset` (`go run ./cmd/demo -reset` → internal/brew.Reset) · `db-shell` (psql) · `build` · `clean`. db-reset идемпотентен (прогнан дважды)
- [x] `cmd/demo/main.go`: тонкий — log/runctx → pg.NewPool → db.New(pool) → ServerVersion/CountDrinks/ListDrinks → tabwriter в stdout; логи в stderr. Флаг `-reset` для цели db-reset (brew.Reset и выход)
- [x] написаны `i18n/ru/README.md` (README-дуга: клиент-сервер → песочница → «что показывает код» → запуск с вставленным фактическим выводом → забрать с собой) и `i18n/en/README.md`. Forward-ссылка на 00-02 — прозой (движок валидирует существование цели в course.yaml; ссылку добавит автор 00-02)
- [x] verification-gate целиком зелёный: db-reset идемпотентен, gen без diff, `make run` = вставленный в README вывод (PostgreSQL 18.4 + меню 5 напитков), `go build`/`go vet`/`go test` зелёные, оба языка; lesson добавлен в `course.yaml` (модули 01–10 запаркованы в комментарии — движок требует непустой lessons[] у каждого объявленного модуля); `make web-check-coverage` зелёный (1 lesson, RU 1/1, EN 1/1, 0 mismatches); бонусом `make web-build`/`web-typecheck`/`web-lint` зелёные, обе lesson-страницы рендерятся, `web/out/404.html` на месте

### Task 2.2: Эталон escape-hatch (интерактивный/EXPLAIN юнит)

**Files:**
- Create: `lectures/05-transactions-and-mvcc/05-02-mvcc-mental-model/` (вариант с двумя psql-сессиями) ИЛИ `lectures/06-indexing-and-explain/06-01-reading-explain-analyze-buffers/`

- [x] выбран 05-02 как канонический escape-hatch. Юнит pure-psql (БЕЗ go.mod — escape-hatch не обязан быть Go; не добавлен в go.work, `make build` его не трогает). `session-a.sql`/`session-b.sql` (читатель REPEATABLE READ + писатель) демонстрируют снапшот-изоляцию через видимые `xmin`; `demo.sql` (цель `run`) показывает механику версий через `ctid`/`xmin` на лабораторном столе `mvcc_lab`. Все 7 файлов: demo.sql, session-a.sql, session-b.sql, Makefile, README.md, i18n/{ru,en}/README.md
- [x] `Makefile`: цели `session-a`/`session-b` (psql -f) + `run` (алиас на основной демо `demo.sql`, как требует Task 4.1) + `db-reset` (psql напрямую по `../../../schema/*.sql`, без internal/brew — Go в юните нет; NOTICE заглушены `PGOPTIONS=client_min_messages=warning`). README показывает interleaving сессий таблицей A↔B. ⚠️ Interleaving НЕ гонка: `\prompt` в session-a держит транзакцию открытой до Enter → порядок детерминирован (улучшение против caveat'а в Post-Completion)
- [x] конвенция зафиксирована в README обоих языков и в шапке Makefile/demo.sql: «когда уроку нужен интерактив, системные колонки или конкурентные сессии → пишем .sql под psql, а не query.sql под sqlc». Включён «заборчик» (упрощение xmin/xmax/ctid → bloat/VACUUM, длинные транзакции держат горизонт видимости — «в проде твой DBA…»)
- [x] написаны оба README (ru+en) по README-дуге: инцидент Brew (отчёт под конкурентным UPDATE) → снимок вместо блокировки → что показывает код (demo + две сессии) → запуск с ВСТАВЛЕННЫМ фактическим выводом → заборчик → забрать с собой + проза-ссылки на 05-01/05-03
- [x] verification-gate (адаптированный) зелёный: `make run` детерминирован и идемпотентен (lab-стол DROP+CREATE, канон нетронут; ctid_changed/xmin_changed=t воспроизводятся дословно); двусессионный сценарий верифицирован реальным interleaving двух коннектов (A: 450→450→500 через границу снапшота — снапшот-изоляция доказана); lesson добавлен в course.yaml (модуль 05 раскомментирован, 05-01 запаркован в комментарии lessons[]); `make web-check-coverage` зелёный (2 lessons, RU 2/2, EN 2/2, 0 mismatches); бонусом `make web-build`/`web-typecheck`/`web-lint` зелёные, обе lesson-страницы рендерятся в ru/en, `web/out/404.html` на месте. (`make gen`/`build` — N/A: юнит без Go.) Стенд снесён `down -v`

### Task 2.3: Авторские конвенции и «как добавить юнит»

**Files:**
- Create: `CLAUDE.md`
- Create: `README.md`

- [x] `CLAUDE.md`: что такое репо (курс + сайт), архитектура движка (engine + thin wrapper, точный пин 1.0.0), общие команды, sqlc-конвенция (канон sqlc.yaml из 2.1), escape-hatch (05-02 как эталон, инвариант `run`), гардрейлы точности (JSON_TABLE=PG17, AIO ~2–3x, OAuth-aside, buffers-by-default), канон Brew (CANON vs RICH) и правило байт-совместимости (защищено тестом). Язык — английский, как в sibling kafka-cookbook (контент юнитов остаётся RU-first/bilingual)
- [x] `README.md`: hero с бейджами (live/deploy/Go/PostgreSQL/pgx/cookbook-engine, accent #336791), getting-started (docker compose up → make list → make lecture + env-дефолты), стек, sandbox-таблица (Postgres 18 + Adminer), repository layout, shared helpers (pg/brew/config/runctx/log + go.mod-пример с replace), scope «What's not covered» (DBA/DevOps out)
- [x] раздел «A shared universe with kafka-cookbook» (кросс-ссылка + handoff-история 10-05: REPLICA IDENTITY FULL → CREATE PUBLICATION dbz_publication → Debezium без переписывания схемы) и раздел «How to add a unit» (7-шаговый рецепт из плана: go.mod/replace → go.work → schema/query/sqlc.yaml-копия → cmd/demo+Makefile → ru-README по дуге с заборчиками → verification-gate → en-README → course.yaml + web-check-coverage)
- [x] wired: TOC встроен в `## Table of contents` между маркерами `<!-- generated by: make web-generate-readme-toc -->` … `<!-- end generated -->` ВЕРБАТИМ из генератора (генератор обрезает label по первой запятой — embed воспроизводится без diff); «How to add a unit» ссылается на `make web-check-coverage` (sanity) и `make web-generate-readme-toc` (+ `TOC_LANG=ru` для RU-варианта). Цели уже существуют в корневом Makefile (Task 0.1)
- [x] verification зелёный: встроенный TOC бит-в-бит воспроизводится из `make web-generate-readme-toc`; `make web-check-coverage` зелёный (2 lessons, RU 2/2, EN 2/2, 0 mismatches); `make web-build` собрал статический экспорт — обе reference-юниты рендерятся в ru И en (`{ru,en}/00-getting-connected/00-01-…/index.html` и `{ru,en}/05-transactions-and-mvcc/05-02-…/index.html` на месте), accent `#336791` + бренд в выводе, `web/out/404.html` на месте

---

## ФАЗА 3 — Авторинг модулей (per-unit рецепт + чек-листы)

> **Per-unit рецепт (DoD для каждого юнита, повторяемый):**
> 1. `mkdir lectures/<NN-module>/<MM-slug>/`; `go.mod` (replace ../../internal); добавить `use` в `lectures/go.work`.
> 2. `schema.sql` (добавки) · `query.sql` (или escape-hatch файлы) · `sqlc.yaml` (копия канона из Task 2.1, `schema:` покрывает baseline+добавки, менять только out-путь при необходимости) · `make gen` → `internal/db/`.
> 3. `cmd/demo/main.go` (тонкий) · `Makefile` (по шаблону 2.1/2.2).
> 4. `i18n/ru/README.md` по дуге; **заборчики**: каждое упрощение + строка «в проде / твой DBA сделал бы X».
> 5. Прогнать verification-gate; вставить фактический вывод в README.
> 6. `i18n/en/README.md` (перевод) — **до отметки «выпущен»**.
> 7. Объявить lesson в `course.yaml`; `make web-check-coverage` зелёный.
>
> Каждый чекбокс ниже = один юнит, доведённый по этому рецепту до DoD.

### Task 3.0: Модуль 00 — Подключение и ориентация (остаток)

**Files:** `lectures/00-getting-connected/{00-02..00-05}/`

- [x] 00-02 psql survival kit — escape-hatch (без go.mod, не в go.work). `demo.sql` (цель `run`) прогоняет детерминированные мета-команды `\dt`/`\d drinks`/`\x` по канону Brew (read-only); `\l`/`\timing`/`\i` разобраны прозой (их вывод машинозависим). Makefile по шаблону 05-02 (run/db-reset psql'ом напрямую/db-shell). Оба README по дуге (инцидент «пропал колд брю» → SQL vs мета-команды → аптечка → запуск с вставленным выводом → заборчик «psql для разведки руками, не для прод-кода»)
- [x] 00-03 подключение из Go — raw-pgx escape-hatch до sqlc (go.mod, без sqlc.yaml/internal/db). `cmd/demo/main.go`: pgxpool + ручной `Query`/`rows.Scan` (тот boilerplate, что в 00-04 заберёт sqlc); анти-демо инъекции на безопасной read-only песочнице — `' OR 1=1 --` через склейку строкой утекает все 5 строк, через `$1` → 0 (детерминировано). Оба README по дуге с заборчиком «SQL строками не склеивать никогда»
- [x] 00-04 типизированные запросы через sqlc — полный канонический sqlc-юнит (go.mod, sqlc.yaml-копия, schema.sql, query.sql, committed internal/db, тонкий main.go). Три формы результата с параметром `$1`: `ListDrinksByCategory :many`, `GetDrinkBySKU :one`, `CountDrinksByCategory :one`-скаляр; sqlc типизирует и именует `$1` из схемы (`category string`). `make gen` воспроизводим (хэш internal/db бит-в-бит совпал при повторе). README явно контрастирует с ручным разбором из 00-03; заборчик «sqlc — кодогенератор, не ORM; схема = миграции»
- [x] 00-05 жизненный цикл соединения и пулинг — raw-pgx escape-hatch (go.mod, без sqlc). Демо прослеживает цикл: ленивый пул (0 коннектов) → `Acquire`×4 (4 реальных бэкенда) → счётчик `pg_stat_activity` по `application_name` = 4 (запрос по уже захваченному коннекту, т.к. пул исчерпан) → `Release` (всего=4, занято=0, простаивают=4 — не закрылись). `application_name` проставлен кастомным `pg.Option`-литералом (escape-hatch поверх `WithMaxConns`). Заборчик: размер пула под `max_connections`/нагрузку, PgBouncer (forward → 10-04), `Acquire`/`Release` парны. Вывод полностью детерминирован
- [x] verification-gate по каждому зелёный: `make db-reset` идемпотентен; `make gen` без diff (00-04); `make run` = вставленный в README вывод (детерминировано, проверено повторным прогоном); `go work sync`/`make build` зелёные по всем модулям; `go vet`/`gofmt` чистые; `go test ./internal/...` зелёный против живой PG18; оба языка у всех 4 юнитов. course.yaml: 4 lesson добавлены в модуль 00; `make web-check-coverage` зелёный (6 lessons, RU 6/6, EN 6/6, 0 mismatches); бонусом `make web-build` собрал статический экспорт — все 8 страниц (4 юнита × ru/en) рендерятся, accent #336791 на месте, `web/out/404.html` присутствует. Стенд снесён `down -v` после проверки

### Task 3.1: Модуль 01 — Типы данных

**Files:** `lectures/01-data-types/{01-01..01-05}/`

- [ ] 01-01 numbers-and-money (numeric vs float: 0.1+0.2)
- [ ] 01-02 text-boolean-and-null-teaser
- [ ] 01-03 date-time-timestamptz (хранить timestamptz всегда; сдвиг по SET TIME ZONE)
- [ ] 01-04 uuid-and-uuidv7 (`gen_random_uuid` vs PG18 `uuidv7()` — на НОВОЙ таблице, не на customers)
- [ ] 01-05 enums-arrays-and-jsonb-intro
- [ ] verification-gate; course.yaml; web-check-coverage

### Task 3.2: Модуль 02 — Схема, DDL, ограничения

**Files:** `lectures/02-schema-and-constraints/{02-01..02-06}/`

- [ ] 02-01 identity-and-defaults (`GENERATED ALWAYS AS IDENTITY` vs serial)
- [ ] 02-02 not-null-pk-natural-vs-surrogate
- [ ] 02-03 foreign-keys (ON DELETE CASCADE/SET NULL)
- [ ] 02-04 unique-and-check (`NULLS NOT DISTINCT`; CHECK)
- [ ] 02-05 generated-columns-and-domains (PG18 virtual vs stored — на новой таблице)
- [ ] 02-06 alter-table-migration-mindset (instant vs rewrite; какие ALTER блокируют) — escape-hatch (DDL)
- [ ] verification-gate; course.yaml; web-check-coverage

### Task 3.3: Модуль 03 — CRUD-беглость

**Files:** `lectures/03-crud-fluency/{03-01..03-06}/`

- [ ] 03-01 insert-and-returning
- [ ] 03-02 select-where-order-limit + keyset-pagination
- [ ] 03-03 update-delete-safely (RETURNING; «забыл WHERE» внутри ROLLBACK)
- [ ] 03-04 upsert-on-conflict (`DO UPDATE SET ... EXCLUDED`)
- [ ] 03-05 returning-old-new (PG18 `UPDATE ... RETURNING old.status, new.status`)
- [ ] 03-06 null-semantics-reckoning (`NOT IN`+NULL; COALESCE/NULLIF/IS DISTINCT FROM)
- [ ] verification-gate; course.yaml; web-check-coverage

### Task 3.4: Модуль 04 — Запросы по таблицам

**Files:** `lectures/04-querying-across-tables/{04-01..04-06}/`

- [ ] 04-01 joins-inner-left-right-full
- [ ] 04-02 multi-table-and-self-joins
- [ ] 04-03 aggregation-group-by-having (count(*) vs count(col))
- [ ] 04-04 distinct-on (последний заказ на клиента)
- [ ] 04-05 subqueries-exists-vs-in (ловушка NOT IN+NULL)
- [ ] 04-06 ctes-and-materialization
- [ ] verification-gate; course.yaml; web-check-coverage

### Task 3.5: Модуль 05 — Транзакции, MVCC, конкурентность (остаток)

**Files:** `lectures/05-transactions-and-mvcc/{05-01,05-03..05-06}/` (05-02 готов в 2.2)

- [ ] 05-01 transactions-and-acid (BEGIN/COMMIT/ROLLBACK; перевод баланса)
- [ ] 05-03 row-locks-and-lost-updates (`FOR UPDATE`; `SKIP LOCKED` для очередей) — escape-hatch (две сессии)
- [ ] 05-04 isolation-levels-for-devs (RC → RR → SERIALIZABLE write-skew) — escape-hatch
- [ ] 05-05 retry-on-40001 (app retry-loop — Go-центричный)
- [ ] 05-06 deadlocks-and-advisory-locks (`40P01`; `pg_advisory_lock`) — escape-hatch
- [ ] verification-gate; course.yaml; web-check-coverage

### Task 3.6: Модуль 06 — Индексы и производительность через EXPLAIN (остаток)

**Files:** `lectures/06-indexing-and-explain/{06-02..06-06}/` (06-01 — кандидат на 2.2; если выбран 05-02, то 06-01 здесь)

- [ ] 06-01 reading-explain-analyze-buffers (seed 1M через `generate_series`; PG18 buffers по умолчанию) — если не сделан в 2.2
- [ ] 06-02 btree-and-composite-column-order (left-prefix; PG18 skip-scan)
- [ ] 06-03 when-indexes-dont-help (`lower(email)` → expression index)
- [ ] 06-04 partial-covering-and-unique (`INCLUDE` → Index-Only Scan)
- [ ] 06-05 gin-for-jsonb-and-arrays (`@>`; jsonb_path_ops)
- [ ] 06-06 create-index-concurrently (без блокировки записей)
- [ ] verification-gate; course.yaml; web-check-coverage

### Task 3.7: Модуль 07 — JSONB, массивы, поиск в БД

**Files:** `lectures/07-jsonb-arrays-and-search/{07-01..07-06}/`

- [ ] 07-01 jsonb-access-and-containment (`-> ->> @> ?`)
- [ ] 07-02 when-not-to-use-jsonb (write-amplification, потеря per-field constraints)
- [ ] 07-03 sql-json-path-and-building (`jsonb_path_query`, `jsonb_set`, `jsonb_agg`; пометка: JSON_TABLE = PG17)
- [ ] 07-04 arrays-vs-junction-table (`text[] @> / = ANY` + GIN vs нормализация)
- [ ] 07-05 full-text-search (generated `tsvector` + GIN; `ts_rank`; `setweight` на drinks/articles)
- [ ] 07-06 pg_trgm-fuzzy (`similarity`, `%`, ускоренный LIKE; decision matrix FTS/trgm/движок)
- [ ] verification-gate; course.yaml; web-check-coverage

### Task 3.8: Модуль 08 — Оконные функции, рекурсивные CTE, LATERAL

**Files:** `lectures/08-analytical-and-lateral/{08-01..08-06}/`

- [ ] 08-01 window-basics-partition-order (running total на клиента)
- [ ] 08-02 ranking-and-top-n-per-group (`ROW_NUMBER()=1`; NTILE)
- [ ] 08-03 lag-lead-and-frames (day-over-day; 7-day moving average RANGE)
- [ ] 08-04 recursive-ctes (дерево категорий Brew; CYCLE guard)
- [ ] 08-05 lateral-joins (top-3 заказа на клиента — N+1 killer)
- [ ] 08-06 grouping-sets-rollup-cube (субитоги + grand total)
- [ ] verification-gate; course.yaml; web-check-coverage

### Task 3.9: Модуль 09 — Продвинутая запись, outbox/NOTIFY, серверная логика

**Files:** `lectures/09-writes-eventing-and-server-logic/{09-01..09-05}/`

- [ ] 09-01 merge-and-copy (`MERGE ... RETURNING merge_action()`; MERGE НЕ race-safe vs ON CONFLICT; `COPY FROM STDIN`)
- [ ] 09-02 skip-locked-job-queue (N воркеров, без двойной обработки — Go-центричный)
- [ ] 09-03 transactional-outbox (атомарная запись order+outbox; relay `FOR UPDATE SKIP LOCKED`)
- [ ] 09-04 listen-notify (`pg_notify` из триггера; transactional/at-most-once/8KB caveats)
- [ ] 09-05 triggers-and-volatility (BEFORE updated_at; AFTER audit OLD/NEW; IMMUTABLE/STABLE/VOLATILE; **«когда НЕ класть логику в БД»**)
- [ ] verification-gate; course.yaml; web-check-coverage

### Task 3.10: Модуль 10 — Use cases (капстоны, с integration-тестами)

**Files:** `lectures/10-use-cases/{01..05}/` (каждый крупнее: несколько файлов, `*_test.go`, при необходимости `docker-compose.override.yml`)

- [ ] 10-01 build-the-brew-schema-capstone (типы+констрейнты+CRUD+tx-retry+индексы, каждый EXPLAIN-verified) + integration-тест
- [ ] 10-02 price-and-promo-engine (PG18 temporal `WITHOUT OVERLAPS`/`PERIOD` + `tstzrange` EXCLUDE + RETURNING old/new audit) + тест
- [ ] 10-03 app-anti-patterns-clinic (N+1, SELECT *, non-sargable, deep OFFSET vs keyset, huge IN → `= ANY($1::int[])`) + тест
- [ ] 10-04 pooling-from-the-app (pgbouncer transaction-mode ломает session advisory locks/LISTEN-NOTIFY/prepared stmts; фиксы) + тест
- [ ] 10-05 the-cdc-seam-handoff (создать+проиндексировать outbox, `REPLICA IDENTITY FULL`, `CREATE PUBLICATION dbz_publication`; `init.sql` байт-совместим с kafka-cookbook → эстафета) + тест
- [ ] verification-gate + `go test` по капстонам; course.yaml; web-check-coverage

---

### Task 4.1: Verify acceptance criteria

- [ ] все 11 модулей объявлены в course.yaml, lessons совпадают с ФС (`make web-check-coverage` зелёный)
- [ ] каждый опубликованный юнит имеет ОБА языка (нет noindex-fallback на «выпущенных»)
- [ ] `make web-build` — полный статический экспорт без ошибок; `web/out/404.html` есть
- [ ] `cd lectures && go build ./...` зелёный по всем модулям; `go test ./...` зелёный (internal + капстоны)
- [ ] выборочно: `make lecture L=<...>` (по умолчанию пробрасывает `run`) для 3–4 юнитов разных типов (sqlc / escape-hatch / капстон) выдаёт вывод из README; у escape-hatch `run` — алиас на основной демо/сессию
- [ ] scope-дисциплина соблюдена: нет DBA/DevOps-тем; каждое упрощение с «в проде…»

### Task 4.2: [Final] Документация и кросс-связка

- [ ] обновить `README.md` (финальный TOC через `make web-generate-readme-toc`, ru-вариант с `TOC_LANG=ru`)
- [ ] кросс-ссылки в обоих READMEs (postgres-cookbook ↔ kafka-cookbook), описать handoff 10-05
- [ ] зафиксировать в `CLAUDE.md` все обнаруженные при авторинге паттерны
- [ ] переместить план в `docs/plans/completed/` (после готовности всего курса; при инкрементальной публикации — отметить, какие фазы завершены)

## Post-Completion
*Требует ручного вмешательства или внешних систем — без чекбоксов.*

**Ручная верификация:**
- Включить GitHub Pages: Settings → Pages → Source «GitHub Actions» (разовый шаг); проверить деплой на `dsbasko.github.io/postgres-cookbook/`.
- Визуальный обзор сайта: RU/EN-переключатель, 3 темы, синий accent (#336791), reading-prefs, free-reading mode.
- Проверить реальный handoff 10-05: поднять kafka-cookbook Debezium-стенд (07-04/09-03/09-04) против `init.sql` из 10-05 и убедиться, что `dbz_publication` стримится (UPDATE/DELETE видны благодаря `REPLICA IDENTITY FULL`).
- Прогон производительностных юнитов (06) на «холодной» и «разогретой» БД — проверить, что планы в README воспроизводятся на разных машинах (отметить зависимость от железа, если есть).
- Escape-hatch-юниты с двумя psql-сессиями (05-02/05-03/05-04/05-06): порядок interleaving сессий недетерминирован (гонка) — задокументировать ожидаемый сценарий и пометить как known caveat в README этих юнитов (аналогично hardware-caveat для 06).

**Внешние системы:**
- Точный пин версии `@dsbasko/cookbook-engine` (`1.0.0`, без каретки — как в эталоне); при обновлении сайта — осознанный бамп версии, пересборка.
- Опционально: добавить badge сборки/Pages в README (как в kafka-cookbook).
- Опционально: настроить `connect`/CI-проверки coverage в GitHub Actions по аналогии с эталоном.
