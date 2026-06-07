.PHONY: help run db-reset db-shell build clean

# Юнит 06-03 — когда индексы не помогают (non-sargable условия) и индекс по
# выражению. Escape-hatch-юнит (как 06-01): урок про планы, sqlc неприменим —
# ведём psql-скриптом. demo.sql на лабораторном столе accounts_lab (200K строк)
# показывает, как функция поверх столбца срывает план в Seq Scan и как это чинит
# индекс по lower(email).
#
# Подключение к песочнице курса (корневой docker-compose.yml). Переопределяемо:
# DATABASE_URL целиком или PG*-переменные.
DATABASE_URL ?= postgres://brew:brew@localhost:5432/brew?sslmode=disable
PSQL ?= psql

# Канон Brew лежит в корне репозитория — ровно три уровня вверх от каталога юнита.
REPO_ROOT := $(CURDIR)/../../..

help:
	@echo "make run        - демо: non-sargable lower(email) → Seq Scan → индекс по выражению (psql -f demo.sql)"
	@echo "make db-reset   - восстановить канон Brew (демо его не трогает, но цель есть у всех юнитов)"
	@echo "make db-shell   - psql к песочнице"
	@echo "make clean      - no-op (юнит без Go, собирать нечего)"
	@echo ""
	@echo "Перед запуском подними песочницу из корня репозитория:"
	@echo "  docker compose up -d"

run:
	@$(PSQL) '$(DATABASE_URL)' -q -f demo.sql

db-reset:
	@PGOPTIONS='-c client_min_messages=warning' $(PSQL) '$(DATABASE_URL)' -v ON_ERROR_STOP=1 -q -o /dev/null -f '$(REPO_ROOT)/schema/brew.sql'
	@PGOPTIONS='-c client_min_messages=warning' $(PSQL) '$(DATABASE_URL)' -v ON_ERROR_STOP=1 -q -o /dev/null -f '$(REPO_ROOT)/schema/seed.sql'
	@echo "Канон Brew восстановлен."

db-shell:
	$(PSQL) '$(DATABASE_URL)'

build:
	@true

clean:
	@true
