.PHONY: help run session-a session-b db-reset db-shell clean

# Юнит 05-03 — блокировки строк и потерянные обновления. Escape-hatch-юнит
# (как 05-02): урок про конкуренцию двух сессий, sqlc неприменим — ведём
# psql-скриптами. demo.sql детерминированно показывает потерянное обновление и
# его починку (атомарный UPDATE, FOR UPDATE); session-a/-b — живую очередь на
# FOR UPDATE SKIP LOCKED (два воркера, без двойной обработки).
#
# Подключение к песочнице курса (корневой docker-compose.yml). Переопределяемо:
# DATABASE_URL целиком или PG*-переменные.
DATABASE_URL ?= postgres://brew:brew@localhost:5432/brew?sslmode=disable
PSQL ?= psql

# Канон Brew лежит в корне репозитория — ровно три уровня вверх от каталога
# юнита (lectures/<NN>/<MM>/ → корень).
REPO_ROOT := $(CURDIR)/../../..

help:
	@echo "make run        - детерминированное демо потерянного обновления и его починки (psql -f demo.sql)"
	@echo "make session-a  - сессия A (воркер 1: готовит очередь, берёт задачу) — запусти в ПЕРВОМ терминале"
	@echo "make session-b  - сессия B (воркер 2: SKIP LOCKED берёт другую задачу) — во ВТОРОМ терминале по подсказке из A"
	@echo "make db-reset   - восстановить канон Brew после демо двух сессий"
	@echo "make db-shell   - psql к песочнице"
	@echo "make clean      - no-op (юнит без Go, собирать нечего)"
	@echo ""
	@echo "Перед запуском подними песочницу из корня репозитория:"
	@echo "  docker compose up -d  &&  make lecture L=05-transactions-and-mvcc/05-03-row-locks-and-lost-updates T=db-reset"
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
db-reset:
	@PGOPTIONS='-c client_min_messages=warning' $(PSQL) '$(DATABASE_URL)' -v ON_ERROR_STOP=1 -q -o /dev/null -f '$(REPO_ROOT)/schema/brew.sql'
	@PGOPTIONS='-c client_min_messages=warning' $(PSQL) '$(DATABASE_URL)' -v ON_ERROR_STOP=1 -q -o /dev/null -f '$(REPO_ROOT)/schema/seed.sql'
	@echo "Канон Brew восстановлен."

db-shell:
	$(PSQL) '$(DATABASE_URL)'

clean:
	@true
