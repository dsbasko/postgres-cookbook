.PHONY: help run db-reset db-shell build clean

# Юнит 03-05 — PG18 RETURNING old/new. Escape-hatch до sqlc: парсер sqlc v1.30.0
# ещё не понимает `RETURNING old.col, new.col` (падает на "column does not
# exist"), а урок именно про эту фичу — поэтому пишем raw-pgx, без sqlc.yaml и
# цели gen (как 00-03 / 00-05).
#
# Подключение к песочнице курса (корневой docker-compose.yml). Переопределяемо:
# DATABASE_URL целиком или PG*-переменные — их читает internal/pg.NewPool.
DATABASE_URL ?= postgres://brew:brew@localhost:5432/brew?sslmode=disable

help:
	@echo "make db-reset   - накатить канон Brew (schema/brew.sql + seed.sql) на песочницу, идемпотентно"
	@echo "make run        - запустить демо (go run ./cmd/demo): UPDATE/INSERT/DELETE ... RETURNING old/new"
	@echo "make db-shell   - psql к песочнице (попробуй UPDATE ... RETURNING old.col, new.col)"
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
