.PHONY: help run db-reset db-shell build clean

# Юнит 00-03 — подключение из Go (raw pgx). Escape-hatch до sqlc: запрос пишем
# строкой и сами разбираем строки через rows.Scan, чтобы в 00-04 увидеть, какой
# boilerplate за нас сгенерирует sqlc. Поэтому здесь нет sqlc.yaml и цели gen.
#
# Подключение к песочнице курса (корневой docker-compose.yml). Переопределяемо:
# DATABASE_URL целиком или PG*-переменные — их читает internal/pg.NewPool.
DATABASE_URL ?= postgres://brew:brew@localhost:5432/brew?sslmode=disable

help:
	@echo "make db-reset   - накатить канон Brew (schema/brew.sql + seed.sql) на песочницу, идемпотентно"
	@echo "make run        - запустить демо (go run ./cmd/demo): параметризованный поиск + анти-демо инъекции"
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
