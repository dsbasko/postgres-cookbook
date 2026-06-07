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

- [x] 01-01 numbers-and-money — sqlc-юнит. FloatVsNumeric на литералах: `0.1+0.2` во float8 даёт `0.30000000000000004` (`= 0.3` → false), в numeric — точно (`0.3`, true). Деньги Brew как BIGINT-центы (`drinks.base_price`, `order_items.unit_price`) → Go int64; меню разворачивается в ₽.коп целочисленной арифметикой, итог заказа #1 = 970 центов. Заборчик: numeric тоже точен, но центы ложатся в int64 без pgtype.Numeric; float для денег — никогда
- [x] 01-02 text-boolean-and-null-teaser — sqlc-юнит. Тизер NULL построен на реальном LEFT JOIN customers↔orders (Карина без заказов → `order_id` NULL, sqlc типизирует как `pgtype.Int8`); `count(*)`=4 vs `count(o.id)`=3 показывает, что count(col) пропускает NULL. NullComparison на литералах: `(NULL=NULL) IS NOT TRUE` и `IS NULL` оба true (приведены к ::boolean → чистый Go bool). boolean из предиката `base_price>400`; text vs char(n) (хвостовой пробел: text значим, char(n) паддит). Полный разбор NULL отложен прозой на 03-06
- [x] 01-03 date-time-timestamptz — escape-hatch (psql `demo.sql`, без go.mod, не в go.work). Один инстант `orders.created_at` заказа #1 под `SET TIME ZONE` UTC/Europe/Moscow(+03)/America/New_York(-05) — три отображения одного момента; затем ловушка `timestamp` без зоны (не сдвигается) vs `timestamptz` (сдвигается). Пояса с фиксированным зимним смещением → детерминированный вывод. Makefile по шаблону 00-02/05-02 (run=psql -f demo.sql, db-reset psql'ом напрямую); `\pset footer off` + `-q` дают чистый вывод
- [x] 01-04 uuid-and-uuidv7 — sqlc-юнит на НОВОЙ таблице `loyalty_signups` (ключ `DEFAULT uuidv7()`, канон не тронут). UUIDFacts детерминирован: версии 4 vs 7, `uuid_extract_timestamp` NULL у v4 / не-NULL у v7; монотонность — вставляем 3 строки, `bool_and(порядок по id = порядку seq)` = true. Значения uuid случайны → печатаем СВОЙСТВА, не значения. Установлен паттерн per-unit DDL: `//runtime.Caller` читает `schema.sql`, `brew.Apply(ctx, pool, ddl)` накатывает канон→DDL→seed (go:embed не дотягивается из cmd/demo/)
- [x] 01-05 enums-arrays-and-jsonb-intro — sqlc-юнит со своим типом `drink_size` (DROP TYPE IF EXISTS+CREATE → идемпотентно; через brew.Apply). EnumOrder: `'small'<'large'`=true (порядок объявления, не алфавит). Массивы: `string_to_array(tags,',')::text[]` → Go `[]string`, `@>` находит 2 статьи с coffee. jsonb intro: `->>` text (`oat`) vs `->` jsonb (`"oat"` с кавычками) vs `jsonb_exists`/`?`. Глубокий jsonb/GIN/FTS отложены прозой на модуль 07
- [x] verification-gate; course.yaml; web-check-coverage — всё зелёное: `make db-reset` идемпотентен у всех 5 (прогон дважды); `make gen` воспроизводим (хэш internal/db бит-в-бит совпал при повторе) у 4 sqlc-юнитов; `make run` = вставленный в README вывод (детерминировано); `cd lectures && go work sync`/`make build`/`go vet`/`gofmt` зелёные по всем модулям; `go test ./internal/...` зелёный; оба языка (ru+en) у всех 5 юнитов + стаб README.md. course.yaml: модуль 01 раскомментирован, 5 lessons добавлены (parking-комментарий → «02–10»); `make web-check-coverage` зелёный (11 lessons, RU 11/11, EN 11/11, 0 mismatches); бонусом `make web-build` собрал статический экспорт — все 10 страниц (5 юнитов × ru/en) рендерятся, `web/out/404.html` на месте. Стенд снесён `down -v`

### Task 3.2: Модуль 02 — Схема, DDL, ограничения

**Files:** `lectures/02-schema-and-constraints/{02-01..02-06}/`

- [x] 02-01 identity-and-defaults — sqlc-юнит на двух своих таблицах (`id_always` GENERATED ALWAYS / `id_by_default` GENERATED BY DEFAULT). Три INSERT без id → 1,2,3; явный id в ALWAYS отбит (428C9), в BY DEFAULT принят (поведение serial → рассинхрон счётчика, заборчик про setval). Канон не тронут
- [x] 02-02 not-null-pk-natural-vs-surrogate — sqlc-юнит (`shop_natural` PK на бизнес-коде / `shop_surrogate` суррогатный id + UNIQUE code). PK = NOT NULL+UNIQUE (отбивает NULL 23502 и дубль 23505); ключевой контраст: переименование кода «уводит» натуральный ключ, а суррогатный id стоит. Заборчик про выбор ключа
- [x] 02-03 foreign-keys — sqlc-юнит (fk_* таблицы). FK блокирует висящую ссылку (23503); ON DELETE CASCADE (удалить детей) / SET NULL (обнулить, колонка NULLABLE) / дефолт NO ACTION≈RESTRICT (запретить, 23503). Заборчик: CASCADE обоюдоострый, индекс под FK (→ 06)
- [x] 02-04 unique-and-check — sqlc-юнит. UNIQUE по умолчанию NULL≠NULL (несколько NULL проходят) vs PG15 NULLS NOT DISTINCT (второй NULL — дубль 23505); CHECK price>0 / size IN(...) (нарушение 23514). Заборчик: CHECK пропускает NULL (отсюда NOT NULL рядом), NOT VALID на большой таблице → 02-06
- [x] 02-05 generated-columns-and-domains — escape-hatch (psql `demo.sql`, без go.mod): sqlc v1.30.0 не парсит PG18 `GENERATED ... VIRTUAL`, а урок именно про неё. STORED (на диске, attgenerated='s') vs VIRTUAL (на лету, 'v'), прямая запись отбита (428C9); DOMAIN positive_cents = BIGINT+CHECK (0 отбит 23514, 300 принят). Лабораторный стол (DROP+CREATE), канон не тронут. Заборчик: VIRTUAL нельзя индексировать/в PK/с доменами
- [x] 02-06 alter-table-migration-mindset — escape-hatch (psql `demo.sql`, без go.mod): физическая цена DDL через relfilenode. ADD COLUMN с константным DEFAULT мгновенен (filenode unchanged=t); ALTER TYPE int→bigint переписывает (=f); CHECK NOT VALID мгновенен (convalidated=f) + VALIDATE отдельным шагом (=t). Заборчик: блокировка важнее переписывания (ACCESS EXCLUSIVE+lock_timeout), zero-downtime ≈ DBA
- [x] verification-gate; course.yaml; web-check-coverage — всё зелёное: `make db-reset` идемпотентен у всех 6 (escape-hatch demo.sql DROP+CREATE — re-run бит-в-бит идентичен); `make gen` воспроизводим (хэш internal/db бит-в-бит совпал при повторе) у 4 sqlc-юнитов; `make run` = вставленный в README вывод (детерминировано — SQLSTATE-коды печатаются вместо недетерминированного текста; для escape-hatch — VERBOSITY terse); `cd lectures && go work sync`/`make build`/`go vet`/`gofmt` зелёные по всем модулям; `go test ./internal/...` зелёный против живой PG18; оба языка (ru+en) у всех 6 юнитов + стаб README.md. course.yaml: модуль 02 раскомментирован, 6 lessons добавлены (parking-комментарий → «03–10»), go.work получил 4 sqlc-юнита (escape-hatch без go.mod в workspace не входят); `make web-check-coverage` зелёный (17 lessons, RU 17/17, EN 17/17, 0 mismatches). Стенд снесён `down -v`

### Task 3.3: Модуль 03 — CRUD-беглость

**Files:** `lectures/03-crud-fluency/{03-01..03-06}/`

- [x] 03-01 insert-and-returning — sqlc-юнит на лабораторной таблице loyalty_cards. INSERT ... RETURNING отдаёт сгенерированный id + колонки по DEFAULT (points, created_at) в одном round-trip, без второго SELECT; created_at печатается как факт «заполнено» (created_set), т.к. now() недетерминирован. Многострочный INSERT ... VALUES (...),(...) RETURNING — форма :many (RETURNING работает и для многих строк). ⚠️ multi-arg `unnest($1::bigint[], $2::text[])` sqlc v1.30.0 НЕ парсит («function unnest(unknown,unknown) does not exist») → перешёл на многострочный VALUES с sqlc.arg-именами; bulk-from-slice (unnest одного массива) и COPY вынесены в заборчик + forward на 09-01
- [x] 03-02 select-where-order-limit + keyset-pagination — sqlc-юнит, read-only по каноническим drinks (детерминирован, 5 напитков). WHERE/ORDER/LIMIT (FilterMenu); PageByOffset (LIMIT/OFFSET) и PageByKeyset (`WHERE (base_price, id) < (after_price, after_id) ORDER BY base_price DESC, id DESC`) — обе с полным порядком (tie-break по id). Демо листает всё меню keyset'ом в цикле (курсор = последняя строка, первая страница со сторожевым курсором 1<<62) и показывает, что страница 2 keyset = страница 2 OFFSET. Заборчик: keyset быстр под индексом (base_price,id), не умеет прыжки на произвольную страницу
- [x] 03-03 update-delete-safely — sqlc-юнит на лабораторной таблице price_lab (засев в начале демо, id 1..5 → детерминированно/идемпотентно). Целевой UPDATE ... WHERE ... RETURNING показывает ровно затронутые строки; «забытый WHERE» (RaiseAll :execrows) и DeleteCategory :execrows исполняются внутри tx (pool.Begin → queries.WithTx(tx)), печатают RowsAffected (масштаб), затем ROLLBACK — состояние после отката = как до катастрофы. Заборчик: psql AUTOCOMMIT off, блокировки/bloat на больших таблицах, soft-delete в проде
- [x] 03-04 upsert-on-conflict — sqlc-юнит на лабораторной таблице stock_levels (составной PK (shop_code, drink_sku) как арбитр). UpsertStock: INSERT ... ON CONFLICT (...) DO UPDATE SET on_hand = EXCLUDED.on_hand RETURNING ..., (xmax <> 0) AS was_update — приём xmax отличает вставку (f) от обновления (t), verified live; UpsertIgnore: ON CONFLICT DO NOTHING (:execrows → 0 при конфликте). Заборчик: атомарность под конкуренцией vs SELECT-потом-INSERT, не клобберить лишние колонки из EXCLUDED, MERGE НЕ race-safe (→ 09-01), COPY для массовой загрузки
- [x] 03-05 returning-old-new — escape-hatch raw-pgx (go.mod, без sqlc.yaml/internal/db). ⚠️ ПРИЧИНА escape-hatch: sqlc v1.30.0 НЕ парсит PG18 `RETURNING old.col, new.col` («column does not exist» — verified) — выбираем фичу, а не инструмент. Демо на лабораторном столе order_status_lab (создаётся inline в run, идемпотентно): UPDATE (обе версии), INSERT (old.* = NULL, печатаем ∅), DELETE (new.* = NULL) — симметрия «есть ли строка до/после», вся семантика verified live. Заборчик: не замена аудиту (триггер+history → 09-05), фича именно PG18, sqlc пока не поддерживает
- [x] 03-06 null-semantics-reckoning — sqlc-юнит (расплата за тизер 01-02). NullLogic на литералах: `(NULL=NULL) IS NULL`=true, `NULL IS NOT DISTINCT FROM NULL`=true, `NULLIF(100,100) IS NULL`=true, `COALESCE(NULL,NULL,42)`=42. Ловушка на данных: список unavailable={4,NULL} (drink_id NULLABLE), `id NOT IN (подзапрос с NULL)` → 0 (молча обнулил ответ), `NOT EXISTS (...)` → 4 (правильно). Заборчик: трёхзначная логика везде (WHERE/JOIN ON/CHECK/count(col)/DISTINCT), NOT NULL в проде, IS DISTINCT FROM для nullable-сравнений
- [x] verification-gate; course.yaml; web-check-coverage — всё зелёное: `cd lectures && go work sync` exit 0; `make build` собрал все workspace-модули (включая 6 новых, escape-hatch 03-05 — go.mod-юнит, в workspace входит); `go vet`/`gofmt` чистые по всем 6; `go test ./internal/...` зелёный против живой PG18; `make gen` воспроизводим (хэш internal/db бит-в-бит совпал при повторе) у 5 sqlc-юнитов (03-05 без internal/db); `make run` = вставленный в README вывод у всех 6 (детерминированно, проверено повторным прогоном — created_at/uuid/время не печатаются, SQLSTATE/xmax/факты-предикаты стабильны); оба языка (ru+en) + стаб README.md у всех 6. course.yaml: модуль 03 раскомментирован, 6 lessons добавлены (parking-комментарий → «04–10»), go.work получил все 6 юнитов; `make web-check-coverage` зелёный (23 lessons, RU 23/23, EN 23/23, 0 mismatches); бонусом `make web-build` собрал статический экспорт — все 12 страниц (6 юнитов × ru/en) рендерятся, `web/out/404.html` на месте. Стенд снесён `down -v`

### Task 3.4: Модуль 04 — Запросы по таблицам

**Files:** `lectures/04-querying-across-tables/{04-01..04-06}/`

- [x] 04-01 joins-inner-left-right-full — sqlc-юнит. INNER/LEFT/RIGHT на каноне customers↔orders (Карина без заказов — несовпавшая строка: INNER её роняет 3 строки, LEFT/RIGHT сохраняют 4 с NULL); RIGHT подан как зеркало LEFT. FULL раскрыт на лабораторной паре листов пересчёта (count_floor/count_storage: зал {1,2} vs склад {2,4}) — несовпадения с ОБЕИХ сторон, на каноне такого нет (каждый заказ ссылается на существующего клиента). sqlc типизирует LEFT-колонки как nullable (pgtype.Int8/Text). Заборчик: JOIN без индекса под ключ, приведение c.id::text, FULL — инструмент сверки источников
- [x] 04-02 multi-table-and-self-joins — sqlc-юнит. Чек заказа = JOIN по 4 таблицам канона (orders→customers→order_items→drinks), line_total приведён к ::bigint (без приведения sqlc вывел бы int32 по первому операнду). Self-join — лабораторная staff (manager_id ссылается на id той же таблицы); e/m под разными псевдонимами, LEFT JOIN сохраняет вершину (Анна без руководителя). Заборчик: иерархия на 1 уровень → рекурсивный CTE (08-04), не стопка self-join'ов
- [x] 04-03 aggregation-group-by-having — sqlc-юнит, read-only по каноне. MenuStatsByCategory (count/min/max/round(avg)::bigint по категориям). Гвоздь урока — count(*) vs count(o.id) на customers LEFT JOIN orders: у Карины count(*)=1, count(o.id)=0 (sum→COALESCE 0, выручка numeric(10,2)::text). HAVING count(o.id)>=2 → только Алиса (WHERE так не умеет — агрегат ещё не посчитан). Заборчик: тихий баг count(*)/count(col), источник «выручки» фиксировать
- [x] 04-04 distinct-on — sqlc-юнит, read-only по каноне. DISTINCT ON (customer_id) + ORDER BY (обязан начинаться с выражения DISTINCT ON): последний заказ на клиента (created_at DESC, id DESC tie-break) → Алиса #3, Борис #2; смена хвоста на amount DESC → самый дорогой (Алиса #1). amount/created_at приведены к ::numeric(10,2)::text / ::date::text (чистые Go-строки). Заборчик: tie-break обязателен, нестандарт → ROW_NUMBER (08-02) для портируемости/top-N
- [x] 04-05 subqueries-exists-vs-in — sqlc-юнит. Три формы на каноне: scalar (base_price > (SELECT avg) — напитки дороже 4.00), IN (id IN order_items.drink_id — 4 заказанных), EXISTS-коррелированный (клиентов с заказами = 2). Кульминация на лабораторной promo (featured_drink_id НАМЕРЕННО nullable): NOT IN {1,NULL} → 0 (ловушка, NULL сворачивает в NULL), NOT EXISTS → 4 (правильно). Перекличка с 03-06, теперь как довод за EXISTS. Заборчик: для «нет среди» — NOT EXISTS; гигантский IN → = ANY($1::тип[]) (10-03)
- [x] 04-06 ctes-and-materialization — sqlc-юнит, read-only по каноне. CTE-конвейер из двух шагов (order_totals→per_customer→имя): траты клиента из позиций (Алиса 19.30/2, Борис 3.00/1). CTE, использованный дважды (FROM + scalar-подзапрос) → доля заказа от общего (43.5/13.5/43.0); AS MATERIALIZED написан явно (sqlc v1.30.0 парсит) — но при ≥2 ссылках это и так умолчание PG12+. Заборчик: разница inline/материализация видна в ПЛАНЕ (EXPLAIN, модуль 06), не в результате; CTE про читаемость, не скорость; рекурсивный CTE → 08-04
- [x] verification-gate; course.yaml; web-check-coverage — всё зелёное: `cd lectures && go work sync` exit 0; `make build` собрал все workspace-модули (включая 6 новых sqlc-юнитов); `go vet`/`gofmt` чистые по всем 6; `go test ./internal/...` зелёный против живой PG18; `make gen` воспроизводим (хэш internal/db бит-в-бит совпал при повторе) у всех 6; `make db-reset` идемпотентен (прогон дважды); `make run` детерминирован (повторный прогон бит-в-бит) и БИТ-В-БИТ совпадает с вставленным в README выводом (проверено программно по всем 6 — created_at не печатается, суммы/счётчики/факты стабильны); оба языка (ru+en) + стаб README.md у всех 6. course.yaml: модуль 04 раскомментирован, 6 lessons добавлены (parking-комментарий → «06–10»), go.work получил все 6 юнитов; `make web-check-coverage` зелёный (29 lessons, RU 29/29, EN 29/29, 0 mismatches); бонусом `make web-build` собрал статический экспорт — все 12 страниц (6 юнитов × ru/en) рендерятся, `web/out/404.html` на месте. Стенд снесён `down -v`

### Task 3.5: Модуль 05 — Транзакции, MVCC, конкурентность (остаток)

**Files:** `lectures/05-transactions-and-mvcc/{05-01,05-03..05-06}/` (05-02 готов в 2.2)

- [x] 05-01 transactions-and-acid — sqlc-юнит на лабораторной таблице ledger_accounts (перевод денег между кассами Brew, центы как BIGINT). transfer() = pool.Begin → Debit → Credit → Commit (иначе defer Rollback): сценарий 2 (успех 30.00 в одной tx, инвариант суммы 150.00 цел), сценарий 3 (получатель #999 не существует → Credit задел 0 строк → весь перевод откатывается, баланс #1 как до катастрофы). CHECK (balance >= 0) ловит overdraft (23514). Вывод детерминирован (TRUNCATE RESTART IDENTITY + фиксированный seed), бит-в-бит совпадает с README; gen воспроизводим
- [x] 05-03 row-locks-and-lost-updates — escape-hatch (psql, без go.mod). demo.sql (цель run) детерминированно показывает потерянное обновление (два \gset читают «10» до записи → оба пишут «9», один декремент потерян), его починку атомарным UPDATE (on_hand = on_hand - 1 под блокировкой строки) и FOR UPDATE на лабораторном seat_lab (DROP в конце — канон нетронут). session-a/-b демонстрируют живую очередь FOR UPDATE SKIP LOCKED (воркер B берёт #2, не дожидаясь #1 у A). Вывод demo бит-в-бит совпадает с README
- [x] 05-04 isolation-levels-for-devs — escape-hatch (psql). demo.sql: дефолт READ COMMITTED (SHOW transaction_isolation), уровень на транзакцию (BEGIN ISOLATION LEVEL REPEATABLE READ/SERIALIZABLE), ЛОГИКА write-skew на shift_lab (правило «на полу ≥1 бариста»: оба видят 2, оба уходят → 0, инвариант сломан; RC/RR пропускают, ловит только SERIALIZABLE через 40001). session-a/-b показывают живой конфликт 40001 под SERIALIZABLE. Вывод бит-в-бит совпадает с README
- [x] 05-05 retry-on-40001 — Go-центричный raw-pgx escape-hatch (go.mod, без sqlc). withRetry() прогоняет транзакцию SERIALIZABLE и повторяет на 40001 (isSerializationFailure через errors.As → *pgconn.PgError, code 40001). Конфликт ДЕТЕРМИНИРОВАН: на попытке 1 синхронно вклинивается коммит «Бориса» (отдельная tx) → попытка 1 падает 40001, попытка 2 на свежем снимке принимает СОДЕРЖАТЕЛЬНО иное решение (на полу 1 → остаться), COMMIT успешен. Вывод бит-в-бит совпадает с README, детерминирован
- [x] 05-06 deadlocks-and-advisory-locks — escape-hatch (psql). demo.sql: pg_try_advisory_lock (взять без ожидания: t/f), реентрабельность (тот же ключ повторно → счётчик 2 → отпускать дважды; третий unlock → f + WARNING в stderr), транзакционный pg_advisory_xact_lock (живёт до COMMIT, освобождается сам: held_now=1, held_after_commit=0). session-a/-b воспроизводят живой дедлок с 40P01 (deadlock_detected). stdout бит-в-бит совпадает с README (WARNING — в stderr, как отмечено)
- [x] verification-gate; course.yaml; web-check-coverage — всё зелёное: `cd lectures && go work sync` exit 0; `make build` собрал все workspace-модули (05-01, 05-05 — go.mod-юниты; escape-hatch 05-03/05-04/05-06 без go.mod в workspace не входят); `go vet`/`gofmt` чистые по 05-01/05-05; `go test ./internal/...` зелёный против живой PG18; `make gen` воспроизводим (хэш internal/db бит-в-бит совпал при повторе) у 05-01; `make db-reset` идемпотентен (прогон дважды); `make run` детерминирован (повторный прогон бит-в-бит) и БИТ-В-БИТ совпадает с вставленным в README выводом у всех 5 (для 05-06 — stdout; WARNING в stderr); оба языка (ru+en) + стаб README.md у всех 5. course.yaml: модуль 05 раскомментирован, 6 lessons (включая 05-02 из 2.2) на месте; попутно достроено объявление модуля 04 (6 lessons), которое осталось незавершённым после Task 3.4 — без него web-check-coverage падал MISSING_IN_YAML; go.work получил 05-01/05-05; `make web-check-coverage` зелёный (34 lessons, RU 34/34, EN 34/34, 0 mismatches); бонусом `make web-build` собрал статический экспорт — страницы модуля 05 рендерятся в ru/en, `web/out/404.html` на месте. Двусессионные сценарии (05-03/05-04/05-06) — интерактивные (known caveat в Post-Completion), демо-цель run детерминирована. Стенд снесён `down -v`

### Task 3.6: Модуль 06 — Индексы и производительность через EXPLAIN (остаток)

**Files:** `lectures/06-indexing-and-explain/{06-02..06-06}/` (06-01 — кандидат на 2.2; если выбран 05-02, то 06-01 здесь)

- [x] 06-01 reading-explain-analyze-buffers — escape-hatch (psql `demo.sql`, без go.mod, не в go.work; урок про чтение плана, sqlc неприменим). Лабораторный стол `events_lab` 1M строк через `generate_series` (детерминированно, без random): один запрос `WHERE ref_no=762312` сначала `Seq Scan` (`Rows Removed by Filter: 999999` — прочитан миллион ради одной строки), после `CREATE INDEX` — `Index Scan` точно в строку. `EXPLAIN (ANALYZE, COSTS OFF, TIMING OFF, BUFFERS OFF)` + `max_parallel_workers_per_gather=0` → вывод воспроизводим дословно; время/buffers разобраны прозой с hardware-caveat. Заборчик: EXPLAIN объясняет ОДИН запрос, не здоровье БД (pg_stat_statements/автовакуум — приборка DBA)
- [x] 06-02 btree-and-composite-column-order — escape-hatch (`menu_lab` 200K строк, индекс `(category, price)`). Левый префикс: Q1 (оба столбца) и Q2 (только лидирующий `category`) идут индексом; Q3 (только второй `price`) — до PG18 Seq Scan, в PG18 skip-scan (отмечено в README). Вывод детерминирован, бит-в-бит совпадает с README. Заборчик: порядок столбцов в составном индексе = порядок самого селективного префикса запросов
- [x] 06-03 when-indexes-dont-help — escape-hatch (`accounts_lab` 200K). non-sargable: индекс по `email` обслуживает `email = ...` (Index Scan), но `lower(email) = ...` его не видит → Seq Scan; `CREATE INDEX ... (lower(email))` (expression index) возвращает Index Scan. Контраст «функция на колонке слепит обычный индекс» доказан планом. Заборчик: оборачивать колонку функцией = строить индекс под ту же функцию (или нормализовать данные)
- [x] 06-04 partial-covering-and-unique — escape-hatch (`orders_lab` 200K). Частичный `(id) WHERE status='pending'` много меньше полного PK (`pg_relation_size` сравнение → `partial_is_smaller=t`) и обслуживает «разгрести pending по порядку»; покрывающий `(customer_id) INCLUDE (total)` + `VACUUM (ANALYZE)` → Index Only Scan с `Heap Fetches: 0`. Вывод детерминирован. Заборчик: INCLUDE-колонки не для поиска, only-scan зависит от visibility map (автовакуум)
- [x] 06-05 gin-for-jsonb-and-arrays — escape-hatch (`drink_specs_lab` 200K, jsonb `attrs` + `text[] tags`). `@>` по jsonb и по массиву БЕЗ индекса — Seq Scan (B-tree не помощник); два GIN-индекса (`USING gin`) → Bitmap Index Scan по обоим. Демонстрирует containment как точку приложения GIN. Заборчик: GIN для @>/?/membership, не для `=`/диапазонов; `jsonb_path_ops` (меньше/быстрее на @>) разобран прозой
- [x] 06-06 create-index-concurrently — escape-hatch (`cic_lab` 5K + session-a/session-b). Детерминированные факты: обычный `CREATE INDEX` внутри BEGIN/COMMIT ок; `CREATE INDEX CONCURRENTLY` внутри транзакции запрещён (SQLSTATE 25001 захвачен в stdout через `:LAST_ERROR_SQLSTATE`, сырой текст ошибки — в stderr, как помечено в README); CONCURRENTLY вне транзакции → `indisvalid=t`; проверка битых индексов (`NOT indisvalid` → 0). Живую незаблокированность записи показывают session-a/-b (интерактивно). Заборчик: сорванный CONCURRENTLY оставляет невалидный индекс → чистить
- [x] verification-gate; course.yaml; web-check-coverage — всё зелёное: все 6 юнитов escape-hatch (psql, без go.mod → не в go.work, `make build` их не трогает); `make run` детерминирован (повторный прогон бит-в-бит) и БИТ-В-БИТ совпадает с вставленным в README выводом у всех 6 (EXPLAIN с COSTS/TIMING/BUFFERS OFF + серийный план + `generate_series` без random; 06-06 stdout — детерминированный SQLSTATE, raw-ошибка в stderr by-design); `make db-reset` идемпотентен (прогон дважды); оба языка (ru+en) + стаб README.md у всех 6 (EN с run-секцией). course.yaml: модуль 06 объявлен, 6 lessons на месте; `cd lectures && go work sync` exit 0, `make build` зелёный по всем workspace-модулям, `go test ./internal/...` зелёный против живой PG18, `go vet`/`gofmt` чистые; `make web-check-coverage` зелёный (40 lessons, RU 40/40, EN 40/40, 0 mismatches). Стенд снесён `down -v`

### Task 3.7: Модуль 07 — JSONB, массивы, поиск в БД

**Files:** `lectures/07-jsonb-arrays-and-search/{07-01..07-06}/`

- [x] 07-01 jsonb-access-and-containment — sqlc-юнит на лабораторной таблице order_options_lab (разношёрстные jsonb-options). Четыре оператора доступа: `->` (jsonb «"oat"») против `->>` (text `oat`), `#>>` по пути (`'{extras,0}'`), containment `@>` (плоская пара `{"milk":"oat"}` → Алиса/Карина; и внутрь массива `{"extras":["honey"]}` → Карина), `?` наличие ключа (ловит пустой `extras` у Дины, которого `@>` не видит). `coalesce(...,'∅')` даёт определённый тип/поведение. Заборчик: фильтруемое/считаемое/джойнимое — в колонки, jsonb — для бесформенного
- [x] 07-02 when-not-to-use-jsonb — escape-hatch (psql `demo.sql`, без go.mod). Две цены jsonb на цифрах: (1) write-amplification — `pg_column_size` колонки=8 байт vs документа=531; `jsonb_set` ради одного поля отдаёт снова 531-байтовый документ (правка = пересборка всего значения); (2) потеря per-field ограничений — колонка `price_cents` с типом+CHECK отбивает мусор (`-5` → SQLSTATE 23514, `banana` → 22P02), а те же значения внутри jsonb записываются молча (`doc_price=banana` при честной `column_price=450`). Печатаем SQLSTATE, не машинозависимый текст. Заборчик: гибридная схема (колонки + один jsonb-хвост) бьёт обе крайности
- [x] 07-03 sql-json-path-and-building — sqlc-юнит на drink_recipe_lab (вложенный массив ингредиентов). jsonpath: `jsonb_path_query_array($.ingredients[*].name)`, фильтр `? (@.grams > 100)` → только milk (220 г), `jsonb_path_query_first`; предикаты `@?`/`@@` (есть milk / kcal>100 → только Латте); сборка — `jsonb_set` (правка → новый документ, хранимая строка цела), `jsonb_agg(jsonb_build_object(...) ORDER BY id)` собирает меню канона drinks в один документ. Явная пометка: `JSON_TABLE` = PG17, не PG18 (прозой, не используется). sqlc типизирует jsonpath/@?/@@ как `interface{}`, pgx возвращает string/bool → печатаем `%v`
- [x] 07-04 arrays-vs-junction-table — sqlc-юнит, одни и те же теги в двух моделях: `drink_tags_arr` (text[] + GIN) и `drink_tags` (junction, составной PK). Массив: `@>` («содержит» → coffee у CAP/CLD/ESP) и `= ANY` (членство, параметр `$1::text`); junction: `WHERE tag='coffee'` даёт ТОТ ЖЕ ответ (эквивалентны по данным), но «частота тегов» — тривиальный `GROUP BY` (coffee/hot по 3), а на массиве нужен `unnest`. Мост: `array_agg(... ORDER BY ...)` сворачивает junction обратно в массив. Заборчик: junction для связей/атрибутов, массив для коротких списков с единственной операцией `@>`/`= ANY`
- [x] 07-05 full-text-search — sqlc-юнит на kb_articles (генерируемый `tsvector` STORED + GIN, веса `setweight` A заголовку / B телу). Контент английский, конфигурация `'english'` → стемминг (`brewing`/`brew` → `'brew':2,8`, `hours`→`hour`) и стоп-слова детерминированы без зависимости от локали. `@@` совпадение, `ts_rank` ранжирование (вес A поднял «Cold brew guide» 0.6957 > «Espresso basics» 0.2432), `to_tsquery('milk & cappuccino')`, морфология (`brewing` находит `brew`). Ранг округлён `round(...::numeric,4)::text`. Заборчик: границы FTS (опечатки/синонимы/масштаб) → pg_trgm или внешний движок
- [x] 07-06 pg_trgm-fuzzy — sqlc-юнит на menu_search_lab (+ `CREATE EXTENSION pg_trgm` + trgm-GIN `gin_trgm_ops`). `similarity(name,'capucino')` по триграммам: только Cappuccino 0.538, остальные ~0; оператор `%` (порог `pg_trgm.similarity_threshold`=0.3) оставляет единственный «did-you-mean» Cappuccino; ускоренный `ILIKE '%presso%'` (подстрока в середине, B-tree бессилен → trgm-GIN) находит Espresso. Матрица выбора FTS/trgm/массив-junction/внешний движок в README. Имена английские → similarity детерминирована. go vet: `%presso%` в Println вынесен в `%s`-аргумент Printf (вывод не изменился)
- [x] verification-gate; course.yaml; web-check-coverage — всё зелёное: `cd lectures && go work sync` exit 0; `make build` собрал все workspace-модули (5 sqlc-юнитов 07-01/03/04/05/06; escape-hatch 07-02 без go.mod в workspace не входит); `gofmt -l` пусто и `go vet` чисто по всем 5; `go test ./internal/...` зелёный против живой PG18; `make gen` воспроизводим у всех 5 sqlc-юнитов (хэш internal/db бит-в-бит совпал при повторе); `make db-reset` идемпотентен (двойной прогон, incl. `CREATE EXTENSION` в 07-06 и две таблицы в 07-04); `make run` детерминирован (повторный прогон бит-в-бит) и БИТ-В-БИТ совпадает с вставленным в README выводом у всех 6 (jsonb-вывод нормализован, ранги/similarity округлены, SQLSTATE вместо текста ошибки); оба языка (ru+en) + стаб README.md у всех 6. course.yaml: модуль 07 раскомментирован, 6 lessons добавлены (parking-комментарий → «08–10»), go.work получил 5 sqlc-юнитов; `make web-check-coverage` зелёный (46 lessons, RU 46/46, EN 46/46, 0 mismatches); бонусом `make web-build` собрал статический экспорт — все 12 страниц (6 юнитов × ru/en) рендерятся, `web/out/404.html` на месте. Стенд снесён `down -v`

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
