GO ?= go
GOLANGCI_LINT ?= golangci-lint
NILAWAY ?= nilaway

DIRECT_DEPS_TEMPLATE := {{if and (not .Main) (not .Indirect) (not .Replace)}}{{.Path}}{{end}}

# Resolve go-sphere modules straight from GitHub, bypassing the module proxy
# and its cached "@latest", which lags behind freshly pushed tags.
DIRECT_ORIGIN := GOPRIVATE=github.com/go-sphere/*

.DEFAULT_GOAL := check

.PHONY: deps-update tidy fmt test lint check

deps-update:
	@GOWORK=off $(DIRECT_ORIGIN) $(GO) mod tidy; \
	deps="$$(GOWORK=off $(DIRECT_ORIGIN) $(GO) list -m -f '$(DIRECT_DEPS_TEMPLATE)' all)"; \
	if [ -n "$$deps" ]; then GOWORK=off $(DIRECT_ORIGIN) $(GO) get -u $$deps; fi
	GOWORK=off $(DIRECT_ORIGIN) $(GO) mod tidy

tidy:
	GOWORK=off $(GO) mod tidy

fmt:
	$(GO) fmt ./...
	$(GOLANGCI_LINT) fmt --no-config --enable gofmt --enable goimports

test:
	$(GO) test ./...

lint:
	$(GOLANGCI_LINT) fmt --no-config --enable gofmt --enable goimports --diff
	$(GO) vet ./...
	$(GOLANGCI_LINT) run --no-config
	$(NILAWAY) -include-pkgs="$$($(GO) list -m)" ./...

check:
	GOWORK=off $(GO) mod tidy -diff
	$(MAKE) lint
	$(MAKE) test
