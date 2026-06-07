.PHONY: help run db-reset db-shell clean

# Юнит 02-06 — мышление миграций (ALTER TABLE). Escape-hatch-юнит: урок про DDL
# и его физическую цену (переписывание таблицы vs только метаданные), наблюдаемую
# через relfilenode и pg_constraint. Это про сам DDL, а не про query.sql + sqlc,
# поэтому ведём psql-скриптом напрямую (как 00-02 / 01-03 / 05-02).
#
# Подключение к песочнице курса (корневой docker-compose.yml). Переопределяемо:
# DATABASE_URL целиком или PG*-переменные.
DATABASE_URL ?= postgres://brew:brew@localhost:5432/brew?sslmode=disable
PSQL ?= psql

# Канон Brew лежит в корне репозитория — ровно три уровня вверх от каталога юнита.
REPO_ROOT := $(CURDIR)/../../..

help:
	@echo "make run        - мгновенный ALTER vs переписывание (relfilenode) + NOT VALID/VALIDATE (psql -f demo.sql)"
	@echo "make db-reset   - восстановить канон Brew (schema/brew.sql + seed.sql), идемпотентно"
	@echo "make db-shell   - psql к песочнице (попробуй свои ALTER и сравни pg_relation_filenode)"
	@echo "make clean      - no-op (юнит без Go, собирать нечего)"
	@echo ""
	@echo "Перед запуском подними песочницу из корня репозитория:"
	@echo "  docker compose up -d  &&  make lecture L=02-schema-and-constraints/02-06-alter-table-migration-mindset T=db-reset"

# run = «основной демо» (алиас escape-hatch-юнита). demo.sql работает на своём
# лабораторном столе (DROP+CREATE) — канон не трогает, вывод воспроизводим.
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
