.PHONY: help run session-a session-b db-reset db-shell build clean

# Юнит 06-06 — CREATE INDEX CONCURRENTLY. Escape-hatch-юнит (как 05-03): урок про
# DDL и блокировки, sqlc неприменим — ведём psql-скриптами. demo.sql
# детерминированно показывает правила CONCURRENTLY (нельзя в транзакции,
# валидность индекса, поиск битых); session-a/-b — живую незаблокированность
# записи во время конкурентной сборки (два терминала).
#
# Подключение к песочнице курса (корневой docker-compose.yml). Переопределяемо:
# DATABASE_URL целиком или PG*-переменные.
DATABASE_URL ?= postgres://brew:brew@localhost:5432/brew?sslmode=disable
PSQL ?= psql

# Канон Brew лежит в корне репозитория — ровно три уровня вверх от каталога юнита.
REPO_ROOT := $(CURDIR)/../../..

help:
	@echo "make run        - детерминированное демо правил CONCURRENTLY (psql -f demo.sql)"
	@echo "make session-a  - сессия A: строит индекс CONCURRENTLY на большой таблице — запусти в ПЕРВОМ терминале"
	@echo "make session-b  - сессия B: пишет в таблицу во время сборки — во ВТОРОМ терминале, пока A строит"
	@echo "make db-reset   - убрать таблицу живого демо (cic_live) и восстановить канон Brew"
	@echo "make db-shell   - psql к песочнице"
	@echo "make clean      - no-op (юнит без Go, собирать нечего)"
	@echo ""
	@echo "Перед запуском подними песочницу из корня репозитория:"
	@echo "  docker compose up -d"
	@echo ""
	@echo "Сценарий двух сессий: терминал-1 'make session-a' → БЫСТРО терминал-2 'make session-b'"
	@echo "(пока A строит индекс) → затем 'make db-reset'."

# run = «основной демо» (алиас escape-hatch-юнита): детерминированный demo.sql.
run:
	@$(PSQL) '$(DATABASE_URL)' -q -f demo.sql

session-a:
	@$(PSQL) '$(DATABASE_URL)' -f session-a.sql

session-b:
	@$(PSQL) '$(DATABASE_URL)' -f session-b.sql

# Чистим таблицу живого демо и восстанавливаем канон Brew. Канон накатываем
# psql'ом напрямую (Go в юните нет): brew.sql идемпотентен, seed.sql делает
# TRUNCATE ... RESTART IDENTITY.
db-reset:
	@PGOPTIONS='-c client_min_messages=warning' $(PSQL) '$(DATABASE_URL)' -q -o /dev/null -c 'DROP TABLE IF EXISTS cic_live;'
	@PGOPTIONS='-c client_min_messages=warning' $(PSQL) '$(DATABASE_URL)' -q -o /dev/null -f '$(REPO_ROOT)/schema/brew.sql'
	@PGOPTIONS='-c client_min_messages=warning' $(PSQL) '$(DATABASE_URL)' -q -o /dev/null -f '$(REPO_ROOT)/schema/seed.sql'
	@echo "Таблица живого демо убрана, канон Brew восстановлен."

db-shell:
	$(PSQL) '$(DATABASE_URL)'

build:
	@true

clean:
	@true
