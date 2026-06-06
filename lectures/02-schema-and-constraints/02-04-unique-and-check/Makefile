.PHONY: help run gen db-reset db-shell build clean

# Юнит 02-04 — UNIQUE и CHECK (включая NULLS NOT DISTINCT). Канонический
# sqlc-юнит со своими таблицами (uniq_default/uniq_nnd/check_drink).
# db-reset накатывает канон Brew + DDL юнита (через brew.Apply + //schema.sql).
#
# Подключение к песочнице курса (корневой docker-compose.yml). Переопределяемо:
# DATABASE_URL целиком или PG*-переменные — их читает internal/pg.NewPool.
DATABASE_URL ?= postgres://brew:brew@localhost:5432/brew?sslmode=disable

# sqlc ставится `go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0`.
SQLC ?= sqlc

help:
	@echo "make db-reset   - накатить канон Brew + таблицы uniq_default/uniq_nnd/check_drink, идемпотентно"
	@echo "make run        - запустить демо (go run ./cmd/demo): NULL в UNIQUE, NULLS NOT DISTINCT, CHECK"
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
