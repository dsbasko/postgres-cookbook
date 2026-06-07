.PHONY: help run gen db-reset db-shell build clean

# Юнит 08-05 — LATERAL-join (top-N на группу одним запросом, убийца N+1).
# Канонический sqlc-юнит со своими лабораторными таблицами lat_customers_lab /
# lat_orders_lab. db-reset накатывает канон + DDL+seed через brew.Apply.
#
# Подключение к песочнице курса (корневой docker-compose.yml). Переопределяемо:
# DATABASE_URL целиком или PG*-переменные — их читает internal/pg.NewPool.
DATABASE_URL ?= postgres://brew:brew@localhost:5432/brew?sslmode=disable

# sqlc ставится `go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0`.
SQLC ?= sqlc

help:
	@echo "make db-reset   - накатить канон Brew + lat_*_lab, идемпотентно"
	@echo "make run        - запустить демо (go run ./cmd/demo): top-3 и top-1 на клиента через LATERAL"
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
