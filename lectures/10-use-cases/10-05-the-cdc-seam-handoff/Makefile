.PHONY: help run test db-reset db-shell build clean

# Капстон 10-05 — шов CDC, эстафета в kafka-cookbook. Go-центричный raw-pgx юнит
# (escape-hatch): урок про конфигурацию логической репликации (PUBLICATION,
# REPLICA IDENTITY, слоты) и доказательство before-image через test_decoding —
# это DDL + системные функции, не SQL уровня sqlc. Как у капстонов положено —
# есть `make test` (asserted integration-тест; без песочницы t.Skip).
#
# Требует wal_level=logical — он уже выставлен в корневом docker-compose.yml.
# Подключение к песочнице. Переопределяемо: DATABASE_URL целиком или PG*.
DATABASE_URL ?= postgres://brew:brew@localhost:5432/brew?sslmode=disable

help:
	@echo "make db-reset   - накатить канон Brew (schema/brew.sql + seed.sql) на песочницу, идемпотентно"
	@echo "make run        - запустить демо (go run ./cmd/demo): PUBLICATION + REPLICA IDENTITY FULL + before-image"
	@echo "make test       - integration-тест против песочницы (go test ./...); без БД — t.Skip"
	@echo "make db-shell   - psql к песочнице"
	@echo "make build      - go build ./..."
	@echo "make clean      - go clean"
	@echo ""
	@echo "Перед первым запуском подними песочницу из корня репозитория:"
	@echo "  docker compose up -d"

run:
	@DATABASE_URL='$(DATABASE_URL)' go run ./cmd/demo

test:
	@DATABASE_URL='$(DATABASE_URL)' go test ./...

db-reset:
	@DATABASE_URL='$(DATABASE_URL)' go run ./cmd/demo -reset

db-shell:
	psql '$(DATABASE_URL)'

build:
	go build ./...

clean:
	go clean
