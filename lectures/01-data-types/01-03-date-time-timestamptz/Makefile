.PHONY: help run db-reset db-shell clean

# Юнит 01-03 — дата и время. Escape-hatch-юнит: урок про SET TIME ZONE — это
# команда уровня сессии psql, а не SQL-запрос, поэтому здесь нет query.sql +
# sqlc, урок ведётся psql-скриптом напрямую (как 00-02 / 05-02).
#
# Подключение к песочнице курса (корневой docker-compose.yml). Переопределяемо:
# DATABASE_URL целиком или PG*-переменные.
DATABASE_URL ?= postgres://brew:brew@localhost:5432/brew?sslmode=disable
PSQL ?= psql

# Канон Brew лежит в корне репозитория — ровно три уровня вверх от каталога юнита.
REPO_ROOT := $(CURDIR)/../../..

help:
	@echo "make run        - один инстант под разными SET TIME ZONE + timestamp vs timestamptz (psql -f demo.sql)"
	@echo "make db-reset   - восстановить канон Brew (schema/brew.sql + seed.sql), идемпотентно"
	@echo "make db-shell   - psql к песочнице (попробуй SET TIME ZONE ... и SELECT created_at)"
	@echo "make clean      - no-op (юнит без Go, собирать нечего)"
	@echo ""
	@echo "Перед запуском подними песочницу из корня репозитория:"
	@echo "  docker compose up -d  &&  make lecture L=01-data-types/01-03-date-time-timestamptz T=db-reset"

# run = «основной демо» (алиас escape-hatch-юнита): только читает, данные не меняет.
run:
	@$(PSQL) '$(DATABASE_URL)' -q -f demo.sql

# Юнит без Go → нет internal/brew. Канон накатываем psql'ом напрямую: brew.sql
# идемпотентен (IF NOT EXISTS), seed.sql делает TRUNCATE ... RESTART IDENTITY.
db-reset:
	@PGOPTIONS='-c client_min_messages=warning' $(PSQL) '$(DATABASE_URL)' -v ON_ERROR_STOP=1 -q -o /dev/null -f '$(REPO_ROOT)/schema/brew.sql'
	@PGOPTIONS='-c client_min_messages=warning' $(PSQL) '$(DATABASE_URL)' -v ON_ERROR_STOP=1 -q -o /dev/null -f '$(REPO_ROOT)/schema/seed.sql'
	@echo "Канон Brew восстановлен."

db-shell:
	$(PSQL) '$(DATABASE_URL)'

clean:
	@true
