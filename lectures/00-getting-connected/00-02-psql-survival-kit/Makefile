.PHONY: help run db-reset db-shell clean

# Юнит 00-02 — psql survival kit. Escape-hatch-юнит: урок про сам psql и его
# мета-команды (\dt, \d, \x, \l, \timing, \i) — это команды клиента, а не SQL,
# поэтому здесь нет query.sql + sqlc, урок ведётся psql-скриптом напрямую.
#
# Подключение к песочнице курса (корневой docker-compose.yml). Переопределяемо:
# DATABASE_URL целиком или PG*-переменные.
DATABASE_URL ?= postgres://brew:brew@localhost:5432/brew?sslmode=disable
PSQL ?= psql

# Канон Brew лежит в корне репозитория — ровно три уровня вверх от каталога
# юнита (lectures/<NN>/<MM>/ → корень).
REPO_ROOT := $(CURDIR)/../../..

help:
	@echo "make run        - экскурсия по psql: \\dt, \\d, \\x на каноне Brew (psql -f demo.sql)"
	@echo "make db-reset   - восстановить канон Brew (schema/brew.sql + seed.sql), идемпотентно"
	@echo "make db-shell   - psql к песочнице (интерактивно — попробуй \\dt, \\d drinks, \\?)"
	@echo "make clean      - no-op (юнит без Go, собирать нечего)"
	@echo ""
	@echo "Перед запуском подними песочницу из корня репозитория:"
	@echo "  docker compose up -d  &&  make lecture L=00-getting-connected/00-02-psql-survival-kit T=db-reset"

# run = «основной демо» (алиас escape-hatch-юнита): читает структуру и одну
# строку меню, данные не меняет.
run:
	@$(PSQL) '$(DATABASE_URL)' -q -f demo.sql

# Юнит без Go → нет internal/brew. Канон накатываем psql'ом напрямую: brew.sql
# идемпотентен (IF NOT EXISTS), seed.sql делает TRUNCATE ... RESTART IDENTITY.
# client_min_messages=warning глушит NOTICE «relation already exists».
db-reset:
	@PGOPTIONS='-c client_min_messages=warning' $(PSQL) '$(DATABASE_URL)' -q -o /dev/null -f '$(REPO_ROOT)/schema/brew.sql'
	@PGOPTIONS='-c client_min_messages=warning' $(PSQL) '$(DATABASE_URL)' -q -o /dev/null -f '$(REPO_ROOT)/schema/seed.sql'
	@echo "Канон Brew восстановлен."

db-shell:
	$(PSQL) '$(DATABASE_URL)'

clean:
	@true
