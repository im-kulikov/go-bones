-include .env

SHELL            := /bin/sh
GOBIN            ?= $(GOPATH)/bin
PATH             := $(GOBIN):$(PATH)
GO               = go

M                = $(shell printf "\033[34;1m>>\033[0m")
TARGET_DIR       ?= $(PWD)/.build
MIGRATIONS_DIR   = ./sql/migrations/
CRUD_FILE        = ./sql/queries/crud_queries.sql

ifeq ($(DELVE_ENABLED),true)
GCFLAGS	= -gcflags 'all=-N -l'
endif

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(firstword $(MAKEFILE_LIST)) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.PHONY: deps
deps: ## Ensure dependencies
	@printf "$(M) Ensure deps: "
	@$(GO) mod tidy && echo OK || (echo FAIL && exit 2)
	@printf "$(M) Download deps: "
	@$(GO) mod download && echo OK || (echo FAIL && exit 2)
	@printf "$(M) Ensure vendor: "
	@$(GO) mod vendor && echo OK || (echo FAIL && exit 2)

.PHONY: lint
lint: ## Run Golang linters aggregator
	$(info $(M) running linters...)
	@$(GOBIN)/golangci-lint run -v --timeout 5m0s ./...

.PHONY: install-tools
install-tools: $(GOBIN) ## Install tools needed for development
	$(info $(M) install tools needed for development...)
	@GOBIN=$(GOBIN) $(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

.PHONY: test
test: MIN_COVERAGE = 70
test: ## Run all tests (pass package as argument if you want test specific one)
	$(eval @_scope := $(or $(addprefix './',$(filter-out $@,$(MAKECMDGOALS))), './...'))
	$(info $(M) running tests for $(@_scope))
	@$(GO) test ./... -v -race -count=1 -cover -coverprofile=coverage.txt
	@$(GO) tool cover -func=coverage.txt | grep total | tee /dev/stderr | sed 's/\%//g' | awk '{err=0;c+=$$3}{if (c > 0 && c < $(MIN_COVERAGE)) {printf "=== FAIL: Coverage failed at %.2f%%\n", c; err=1}} END {exit err}'
