.PHONY: help run gen db-reset db-shell build clean

# Юнит 05-01 — транзакции и ACID. Канонический sqlc-юнит на лабораторной
# таблице ledger_accounts: перевод денег между счетами как BEGIN → списать →
# зачислить → COMMIT (или ROLLBACK). Атомарность и инвариант суммы наблюдаемы.
#
# Подключение к песочнице курса (корневой docker-compose.yml). Переопределяемо:
# DATABASE_URL целиком или PG*-переменные — их читает internal/pg.NewPool.
DATABASE_URL ?= postgres://brew:brew@localhost:5432/brew?sslmode=disable

# sqlc ставится `go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0`
# (бинарь окажется в $(go env GOPATH)/bin — добавь его в PATH).
SQLC ?= sqlc

help:
	@echo "make db-reset   - накатить канон Brew + таблицу ledger_accounts, идемпотентно"
	@echo "make run        - запустить демо (go run ./cmd/demo): успешный перевод в tx + неудачный с ROLLBACK"
	@echo "make gen        - sqlc generate (перегенерить internal/db из query.sql)"
	@echo "make db-shell   - psql к песочнице"
	@echo "make build      - go build ./..."
	@echo "make clean      - go clean"
	@echo ""
	@echo "Перед первым запуском подними песочницу из корня репозитория:"
	@echo "  docker compose up -d"

run:
	@DATABASE_URL='$(DATABASE_URL)' go run ./cmd/demo

gen:
	$(SQLC) generate

db-reset:
	@DATABASE_URL='$(DATABASE_URL)' go run ./cmd/demo -reset

db-shell:
	psql '$(DATABASE_URL)'

build:
	go build ./...

clean:
	go clean
