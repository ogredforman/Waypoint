# Waypoint build targets. See AGENTS.md for when to run each one.
#
# Recipes avoid shell-specific syntax (no `[ -f ]`, no inline `VAR=x cmd`) so they
# behave the same whether make runs them through sh or cmd.exe. Conditionals use
# make functions, and steps for parts that don't exist yet are skipped.

GOEXE := $(shell go env GOEXE)
BIN   := bin/waypoint$(GOEXE)

# No CGO, ever. The SQLite driver is modernc.org/sqlite (pure Go).
export CGO_ENABLED := 0

HAVE_WEB  := $(wildcard web/package.json)
HAVE_SQLC := $(wildcard sqlc.yaml)

.PHONY: dev build web test lint sqlc

## dev: run the Go server (the Vite dev server joins in PLAN 0.7)
dev:
	go run ./cmd/waypoint

## build: production binary with the frontend embedded
build: web
	go build -o $(BIN) ./cmd/waypoint

## web: build the frontend into internal/web/dist (skipped until web/ exists)
web:
ifneq ($(HAVE_WEB),)
	npm --prefix web ci
	npm --prefix web run build
else
	@echo Skipping frontend build: web/ is not set up yet, see PLAN 0.7
endif

## test: all Go and frontend tests
test:
	go test ./...
ifneq ($(HAVE_WEB),)
	npm --prefix web test
endif

## lint: go vet, staticcheck, sqlc diff (generated code is current), svelte-check
lint:
	go vet ./...
	go tool staticcheck ./...
ifneq ($(HAVE_SQLC),)
	go tool sqlc diff
endif
ifneq ($(HAVE_WEB),)
	npm --prefix web run check
endif

## sqlc: regenerate query code after editing internal/db/queries/
sqlc:
ifneq ($(HAVE_SQLC),)
	go tool sqlc generate
else
	@echo Nothing to generate: no sqlc.yaml yet, see PLAN 0.5
endif
