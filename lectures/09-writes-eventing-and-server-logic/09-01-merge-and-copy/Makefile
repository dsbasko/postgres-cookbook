.PHONY: help run db-reset db-shell build clean

# Юнит 09-01 — MERGE + COPY FROM STDIN. Go-центричный escape-hatch до sqlc:
# COPY FROM STDIN — это протокол COPY (в pgx — pool.CopyFrom), а не запрос, и
# MERGE ... RETURNING merge_action() парсер sqlc v1.30.0 не понимает. Поэтому
# raw-pgx, без sqlc.yaml и цели gen (как 00-03 / 05-05 / 03-05).
#
# Подключение к песочнице курса (корневой docker-compose.yml). Переопределяемо:
# DATABASE_URL целиком или PG*-переменные — их читает internal/pg.NewPool.
DATABASE_URL ?= postgres://brew:brew@localhost:5432/brew?sslmode=disable

help:
	@echo "make db-reset   - накатить канон Brew (schema/brew.sql + seed.sql) на песочницу, идемпотентно"
	@echo "make run        - запустить демо (go run ./cmd/demo): COPY FROM STDIN + MERGE (INSERT/UPDATE/DELETE)"
	@echo "make db-shell   - psql к песочнице"
	@echo "make build      - go build ./..."
	@echo "make clean      - go clean"
	@echo ""
	@echo "Перед первым запуском подними песочницу из корня репозитория:"
	@echo "  docker compose up -d"

run:
	@DATABASE_URL='$(DATABASE_URL)' go run ./cmd/demo

db-reset:
	@DATABASE_URL='$(DATABASE_URL)' go run ./cmd/demo -reset

db-shell:
	psql '$(DATABASE_URL)'

build:
	go build ./...

clean:
	go clean
