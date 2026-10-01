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

.PHONY: dev dev-api dev-web build web test e2e lint sqlc gen

## dev: run the Go server and the Vite dev server (proxying /api to Go) side by side
dev:
	$(MAKE) -j2 dev-api dev-web

dev-api:
	go run ./cmd/waypoint

dev-web:
ifneq ($(HAVE_WEB),)
	npm --prefix web run dev
else
	@echo Skipping the Vite dev server: web/ is not set up yet
endif

## build: production binary with the frontend embedded
build: web
	go build -o $(BIN) ./cmd/waypoint

## web: build the frontend into internal/web/dist, where the Go binary embeds it
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

## e2e: Playwright smoke test of the full loop against the real binary (separate from test:
## it needs a frontend build and a Chromium download)
e2e: web
	npm --prefix web exec -- playwright install chromium
	npm --prefix web run e2e

## lint: go vet, staticcheck, generated code is current (sqlc diff, gen -check), svelte-check
lint:
	go vet ./...
	go tool staticcheck ./...
ifneq ($(HAVE_SQLC),)
	go tool sqlc diff
endif
	go run ./internal/api/apitypes/gen -check
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

## gen: regenerate the frontend's API types (web/src/lib/api/*.gen.ts) after editing
## internal/api/apitypes or the enums in internal/domain (ADR 0001)
gen:
	go run ./internal/api/apitypes/gen
