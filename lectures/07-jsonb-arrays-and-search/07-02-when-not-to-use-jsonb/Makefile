.PHONY: help run db-reset db-shell build clean

# Юнит 07-02 — когда НЕ нужен jsonb. Escape-hatch-юнит (как 06-05): урок про
# физическую (write-amplification) и семантическую (нет per-field ограничений)
# цену jsonb — ведём psql-скриптом, чтобы показать pg_column_size и SQLSTATE
# отбитых ограничений. demo.sql работает на лабораторном столе menu_doc_lab.
#
# Подключение к песочнице курса (корневой docker-compose.yml). Переопределяемо:
# DATABASE_URL целиком или PG*-переменные.
DATABASE_URL ?= postgres://brew:brew@localhost:5432/brew?sslmode=disable
PSQL ?= psql

# Канон Brew лежит в корне репозитория — ровно три уровня вверх от каталога юнита.
REPO_ROOT := $(CURDIR)/../../..

help:
	@echo "make run        - демо: write-amplification + потеря per-field ограничений (psql -f demo.sql)"
	@echo "make db-reset   - восстановить канон Brew (демо его не трогает, но цель есть у всех юнитов)"
	@echo "make db-shell   - psql к песочнице"
	@echo "make clean      - no-op (юнит без Go, собирать нечего)"
	@echo ""
	@echo "Перед запуском подними песочницу из корня репозитория:"
	@echo "  docker compose up -d"

run:
	@$(PSQL) '$(DATABASE_URL)' -q -f demo.sql

db-reset:
	@PGOPTIONS='-c client_min_messages=warning' $(PSQL) '$(DATABASE_URL)' -q -o /dev/null -f '$(REPO_ROOT)/schema/brew.sql'
	@PGOPTIONS='-c client_min_messages=warning' $(PSQL) '$(DATABASE_URL)' -q -o /dev/null -f '$(REPO_ROOT)/schema/seed.sql'
	@echo "Канон Brew восстановлен."

db-shell:
	$(PSQL) '$(DATABASE_URL)'

build:
	@true

clean:
	@true
