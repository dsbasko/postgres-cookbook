.PHONY: help run session-a session-b db-reset db-shell clean

# Юнит 05-06 — дедлоки и advisory-локи. Escape-hatch-юнит (как 05-02): дедлок
# конкурентен (нужны две сессии), ведём psql-скриптами. demo.sql детерминированно
# показывает API advisory-локов; session-a/-b — живой дедлок с ошибкой 40P01,
# который Postgres обнаруживает и разрывает сам.
#
# Подключение к песочнице курса (корневой docker-compose.yml). Переопределяемо:
# DATABASE_URL целиком или PG*-переменные.
DATABASE_URL ?= postgres://brew:brew@localhost:5432/brew?sslmode=disable
PSQL ?= psql

# Канон Brew лежит в корне репозитория — ровно три уровня вверх от каталога
# юнита (lectures/<NN>/<MM>/ → корень).
REPO_ROOT := $(CURDIR)/../../..

help:
	@echo "make run        - детерминированное демо API advisory-локов (psql -f demo.sql)"
	@echo "make session-a  - сессия A (берёт строку #1, затем #2) — запусти в ПЕРВОМ терминале"
	@echo "make session-b  - сессия B (берёт строку #2, затем #1 — встречный порядок) — во ВТОРОМ терминале по подсказке из A"
	@echo "make db-reset   - восстановить канон Brew после демо двух сессий"
	@echo "make db-shell   - psql к песочнице"
	@echo "make clean      - no-op (юнит без Go, собирать нечего)"
	@echo ""
	@echo "Перед запуском подними песочницу из корня репозитория:"
	@echo "  docker compose up -d  &&  make lecture L=05-transactions-and-mvcc/05-06-deadlocks-and-advisory-locks T=db-reset"
	@echo ""
	@echo "Сценарий двух сессий: терминал-1 'make session-a' → по подсказке"
	@echo "терминал-2 'make session-b' → вернись в A, Enter → вернись в B, Enter → 'make db-reset'."

# run = «основной демо» (алиас escape-hatch-юнита): детерминированный demo.sql.
run:
	@$(PSQL) '$(DATABASE_URL)' -q -f demo.sql

session-a:
	@$(PSQL) '$(DATABASE_URL)' -f session-a.sql

session-b:
	@$(PSQL) '$(DATABASE_URL)' -f session-b.sql

# Юнит без Go → нет internal/brew. Канон накатываем psql'ом напрямую.
db-reset:
	@PGOPTIONS='-c client_min_messages=warning' $(PSQL) '$(DATABASE_URL)' -v ON_ERROR_STOP=1 -q -o /dev/null -f '$(REPO_ROOT)/schema/brew.sql'
	@PGOPTIONS='-c client_min_messages=warning' $(PSQL) '$(DATABASE_URL)' -v ON_ERROR_STOP=1 -q -o /dev/null -f '$(REPO_ROOT)/schema/seed.sql'
	@echo "Канон Brew восстановлен."

db-shell:
	$(PSQL) '$(DATABASE_URL)'

clean:
	@true
