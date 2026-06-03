.PHONY: help web-install web-dev web-build web-lint web-typecheck web-clean web-check-coverage web-generate-readme-toc

# On this phase the root Makefile owns only the web-* targets.
# Lecture targets (list/lecture/sync/build) are wired once lectures/ exists (phase 1).

help:
	@echo "make web-install                            - pnpm install from the root (workspace)"
	@echo "make web-dev                                - start the Next.js dev server (http://localhost:3000)"
	@echo "make web-build                              - build the static site into web/out/"
	@echo "make web-lint                               - eslint check for web/"
	@echo "make web-typecheck                          - tsc --noEmit in web/"
	@echo "make web-check-coverage                     - reconcile course.yaml with lectures/ + RU/EN translation coverage"
	@echo "make web-generate-readme-toc [TOC_LANG=en|ru] - print a markdown TOC for the root README (default EN)"
	@echo "make web-clean                              - remove web/.next and web/out"

# --- web targets ----------------------------------------------------------
WEB_DIR := $(CURDIR)/web

# Install from the workspace root so pnpm hoists a single react/next instance.
# Installing inside web/ would risk a second copy and break React hooks in SSG.
web-install:
	@cd "$(CURDIR)" && pnpm install

web-dev:
	@cd "$(WEB_DIR)" && pnpm dev

web-build:
	@cd "$(WEB_DIR)" && pnpm build

web-lint:
	@cd "$(WEB_DIR)" && pnpm lint

web-typecheck:
	@cd "$(WEB_DIR)" && pnpm typecheck

# Coverage/TOC helpers live in the engine package and resolve course data
# via process.cwd() — run them from web/ so course.yaml/lectures are found.
ENGINE_SCRIPTS := node_modules/@dsbasko/cookbook-engine/scripts

web-check-coverage:
	@cd "$(WEB_DIR)" && pnpm exec tsx $(ENGINE_SCRIPTS)/check-course-coverage.mts

# Prints a markdown TOC for the root README. Default TOC_LANG=en.
# TOC_LANG=ru — prints a TOC with Russian headings and links to /i18n/ru/README.md.
TOC_LANG ?= en
web-generate-readme-toc:
	@cd "$(WEB_DIR)" && pnpm exec tsx $(ENGINE_SCRIPTS)/generate-readme-toc.mts --lang=$(TOC_LANG)

web-clean:
	@rm -rf "$(WEB_DIR)/.next" "$(WEB_DIR)/out"
