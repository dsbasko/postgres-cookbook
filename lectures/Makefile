.PHONY: help list lecture sync build

# Корень для целей внутри lectures/.
LECTURES_DIR := $(CURDIR)
REPO_ROOT := $(CURDIR)/..

# Какой таргет пробросить в Makefile юнита. По умолчанию `run` (демо урока):
# `make lecture L=<path>` сразу запускает демо, `T=help` показывает справку юнита.
# Конвенция: КАЖДЫЙ юнит обязан иметь таргет `run` (escape-hatch-юниты — алиас
# на основной демо/сессию).
T ?= run

help:
	@echo "make list                              - вывести дерево юнитов"
	@echo "make lecture L=<path> [T=<target>]     - выполнить make в директории юнита (по умолчанию T=run)"
	@echo "  пример:  make lecture L=00-getting-connected/00-01-client-server-and-sandbox"
	@echo "  справка: make lecture L=<path> T=help"
	@echo "make sync                              - go work sync"
	@echo "make build                             - собрать все workspace-модули"

list:
	@find . -mindepth 2 -maxdepth 3 -name 'README.md' -not -path './internal/*' \
	  | sed -e 's|^./||' -e 's|/README.md$$||' \
	  | sort

lecture:
ifndef L
	$(error L is required: make lecture L=00-getting-connected/00-01-client-server-and-sandbox)
endif
	@test -d "$(LECTURES_DIR)/$(L)" || (echo "lecture not found: $(L)"; exit 1)
	$(MAKE) -C "$(LECTURES_DIR)/$(L)" $(T)

sync:
	go work sync

# Собирает каждый модуль из go.work отдельно — `go build ./...` из workspace
# root в Go 1.26 ругается «directory prefix . does not contain modules».
build:
	@for d in $$(go list -f '{{.Dir}}' -m); do \
	  echo "==> $$d"; \
	  (cd "$$d" && go build ./...) || exit 1; \
	done
