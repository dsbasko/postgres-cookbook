.PHONY: help run gen db-reset db-shell build clean

# Юнит 03-04 — upsert через ON CONFLICT. Канонический sqlc-юнит на лабораторной
# таблице stock_levels: «вставь или обнови» одной командой (DO UPDATE SET ... =
# EXCLUDED) и идемпотентная вставка (DO NOTHING).
#
# Подключение к песочнице курса (корневой docker-compose.yml). Переопределяемо:
# DATABASE_URL целиком или PG*-переменные — их читает internal/pg.NewPool.
DATABASE_URL ?= postgres://brew:brew@localhost:5432/brew?sslmode=disable

# sqlc ставится `go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0`
# (бинарь окажется в $(go env GOPATH)/bin — добавь его в PATH).
SQLC ?= sqlc

help:
	@echo "make db-reset   - накатить канон Brew + таблицу stock_levels, идемпотентно"
	@echo "make run        - запустить демо (go run ./cmd/demo): вставка → обновление → DO NOTHING"
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
