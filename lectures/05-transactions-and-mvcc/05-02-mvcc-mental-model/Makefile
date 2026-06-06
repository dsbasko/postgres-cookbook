.PHONY: help run session-a session-b db-reset db-shell clean

# Юнит 05-02 — ментальная модель MVCC. КАНОНИЧЕСКИЙ escape-hatch-юнит: эталон
# целей для юнитов, где sqlc неприменим. Когда уроку нужны системные колонки
# (ctid/xmin/xmax), интерактив или две конкурентные сессии — query.sql + кодоген
# не подходят, и урок ведётся psql-скриптами напрямую.
#
# Подключение к песочнице курса (корневой docker-compose.yml). Переопределяемо:
# DATABASE_URL целиком или PG*-переменные.
DATABASE_URL ?= postgres://brew:brew@localhost:5432/brew?sslmode=disable
PSQL ?= psql

# Канон Brew лежит в корне репозитория — ровно три уровня вверх от каталога
# юнита (lectures/<NN>/<MM>/ → корень), как и schema: в sqlc-юнитах.
REPO_ROOT := $(CURDIR)/../../..

help:
	@echo "make run        - детерминированное демо механики MVCC: UPDATE пишет НОВУЮ версию строки (psql -f demo.sql)"
	@echo "make session-a  - сессия A (читатель, REPEATABLE READ) — запусти в ПЕРВОМ терминале"
	@echo "make session-b  - сессия B (писатель) — запусти во ВТОРОМ терминале по подсказке из A"
	@echo "make db-reset   - восстановить канон Brew после демо двух сессий"
	@echo "make db-shell   - psql к песочнице"
	@echo "make clean      - no-op (юнит без Go, собирать нечего)"
	@echo ""
	@echo "Перед запуском подними песочницу из корня репозитория:"
	@echo "  docker compose up -d  &&  make lecture L=05-transactions-and-mvcc/05-02-mvcc-mental-model T=db-reset"
	@echo ""
	@echo "Сценарий двух сессий: терминал-1 'make session-a' → по подсказке"
	@echo "терминал-2 'make session-b' → вернись в A и нажми Enter → 'make db-reset'."

# run = «основной демо» (алиас escape-hatch-юнита): детерминированный demo.sql.
run:
	@$(PSQL) '$(DATABASE_URL)' -q -f demo.sql

session-a:
	@$(PSQL) '$(DATABASE_URL)' -f session-a.sql

session-b:
	@$(PSQL) '$(DATABASE_URL)' -f session-b.sql

# Юнит без Go → нет internal/brew. Канон накатываем psql'ом напрямую: brew.sql
# идемпотентен (IF NOT EXISTS), seed.sql делает TRUNCATE ... RESTART IDENTITY.
# client_min_messages=warning глушит NOTICE «relation already exists» от
# повторного IF NOT EXISTS — на чистоту наката не влияет.
db-reset:
	@PGOPTIONS='-c client_min_messages=warning' $(PSQL) '$(DATABASE_URL)' -q -o /dev/null -f '$(REPO_ROOT)/schema/brew.sql'
	@PGOPTIONS='-c client_min_messages=warning' $(PSQL) '$(DATABASE_URL)' -q -o /dev/null -f '$(REPO_ROOT)/schema/seed.sql'
	@echo "Канон Brew восстановлен."

db-shell:
	$(PSQL) '$(DATABASE_URL)'

clean:
	@true
