.PHONY: help run db-reset db-shell build clean

# Юнит 08-04 — рекурсивные CTE (обход дерева + защита от цикла секцией CYCLE).
# Escape-hatch-юнит (как 05-02/06-01): sqlc не понимает секцию CYCLE, а урок
# ровно про неё — ведём psql-скриптом. demo.sql на лабораторных столах
# category_tree_lab и cyclic_routes_lab; канон Brew демо не трогает.
#
# Подключение к песочнице курса (корневой docker-compose.yml). Переопределяемо:
# DATABASE_URL целиком или PG*-переменные.
DATABASE_URL ?= postgres://brew:brew@localhost:5432/brew?sslmode=disable
PSQL ?= psql

# Канон Brew лежит в корне репозитория — ровно три уровня вверх от каталога
# юнита (lectures/<NN>/<MM>/ → корень).
REPO_ROOT := $(CURDIR)/../../..

help:
	@echo "make run        - детерминированное демо: обход дерева + CYCLE-guard (psql -f demo.sql)"
	@echo "make db-reset   - восстановить канон Brew (демо его не трогает, но цель есть у всех юнитов)"
	@echo "make db-shell   - psql к песочнице"
	@echo "make clean      - no-op (юнит без Go, собирать нечего)"
	@echo ""
	@echo "Перед запуском подними песочницу из корня репозитория:"
	@echo "  docker compose up -d"

# run = «основной демо» (алиас escape-hatch-юнита): детерминированный demo.sql.
run:
	@$(PSQL) '$(DATABASE_URL)' -q -f demo.sql

# Юнит без Go → нет internal/brew. Канон накатываем psql'ом напрямую: brew.sql
# идемпотентен (IF NOT EXISTS), seed.sql делает TRUNCATE ... RESTART IDENTITY.
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
