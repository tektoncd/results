MODULE   = $(shell env GO111MODULE=on $(GO) list -m)
DATE    ?= $(shell date +%FT%T%z)
VERSION ?= $(shell git describe --tags --always --dirty --match=v* 2> /dev/null || \
			cat $(CURDIR)/.version 2> /dev/null || echo v0)
PKGS     = $(or $(PKG),$(shell env GO111MODULE=on $(GO) list ./... ))
TESTPKGS = $(shell env GO111MODULE=on $(GO) list -f \
			'{{ if or .TestGoFiles .XTestGoFiles }}{{ .ImportPath }}{{ end }}' \
			$(PKGS))
BIN      = $(CURDIR)/.bin

# Get golangci_version from tools/go.mod
GOLANGCI_VERSION := $(shell yq '.jobs.golangci.steps[] | select(.name == "golangci-lint") | .with.version' .github/workflows/golangci-lint.yaml)

GO           = go
TIMEOUT_UNIT = 5m
TIMEOUT_E2E  = 20m
V = 0
Q = $(if $(filter 1,$V),,@)
M = $(shell printf "\033[34;1m🐱\033[0m")

export GO111MODULE=on

COMMANDS=$(patsubst cmd/%,%,$(wildcard cmd/*))
BINARIES=$(addprefix bin/,$(COMMANDS))

.PHONY: all
all: fmt $(BINARIES) | $(BIN) ; $(info $(M) building executable…) @ ## Build program binary

$(BIN):
	@mkdir -p $@
$(BIN)/%: | $(BIN) ; $(info $(M) building $(PACKAGE)…)
	$Q tmp=$$(mktemp -d); \
	   env GO111MODULE=off GOPATH=$$tmp GOBIN=$(BIN) $(GO) get $(PACKAGE) \
		|| ret=$$?; \
	   rm -rf $$tmp ; exit $$ret

FORCE:

bin/%: cmd/% FORCE
	$Q $(GO) build -mod=vendor $(LDFLAGS) -v -o $@ ./$<
## Linters configuration and targets
# TODO(vdemeester) gofmt and goimports checks (run them with -w and make a diff)

GOLINT = $(BIN)/golint
$(BIN)/golint: PACKAGE=golang.org/x/lint/golint

.PHONY: golint
golint: | $(GOLINT) ; $(info $(M) running golint…) @ ## Run golint
	$Q $(GOLINT) -set_exit_status $(PKGS)

.PHONY: vet
vet: | ; $(info $(M) running go vet…) @ ## Run go vet
	$Q go vet ./...

INEFFASSIGN = $(BIN)/ineffassign
$(BIN)/ineffassign: PACKAGE=github.com/gordonklaus/ineffassign

.PHONY: ineffassign
ineffassign: | $(INEFFASSIGN) ; $(info $(M) running static ineffassign…) @ ## Run ineffassign
	$Q $(INEFFASSIGN) .

STATICCHECK = $(BIN)/staticcheck
$(BIN)/staticcheck: PACKAGE=honnef.co/go/tools/cmd/staticcheck

.PHONY: staticcheck
staticcheck: | $(STATICCHECK) ; $(info $(M) running static check…) @ ## Run staticcheck
	$Q $(STATICCHECK) ./...

DUPL = $(BIN)/dupl
$(BIN)/dupl: PACKAGE=github.com/mibk/dupl

.PHONY: dupl
dupl: | $(DUPL) ; $(info $(M) running dupl…) ## Run dupl
	$Q $(DUPL)

ERRCHECK = $(BIN)/errcheck
$(BIN)/errcheck: PACKAGE=github.com/kisielk/errcheck

.PHONY: errcheck
errcheck: | $(ERRCHECK) ; $(info $(M) running errcheck…) ## Run errcheck
	$Q $(ERRCHECK) ./...

GOLANGCILINT = $(BIN)/golangci-lint-$(GOLANGCI_VERSION)
$(BIN)/golangci-lint-$(GOLANGCI_VERSION): ; $(info $(M) getting golangci-lint $(GOLANGCI_VERSION))
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(BIN) $(GOLANGCI_VERSION)
	mv $(BIN)/golangci-lint $(BIN)/golangci-lint-$(GOLANGCI_VERSION)

.PHONY: golangci-lint
golangci-lint: | $(GOLANGCILINT) ; $(info $(M) running golangci-lint…) @ ## Run golangci-lint
	$Q $(GOLANGCILINT) run --verbose --modules-download-mode=vendor --max-issues-per-linter=0 --max-same-issues=0 --timeout 10m

.PHONY: golangci-lint-check
golangci-lint-check: | $(GOLANGCILINT) ; $(info $(M) Testing if golint has been done…) @ ## Run golangci-lint for build tests CI job
	$Q $(GOLANGCILINT) run -j 1 --color=never --deadline 15m

GOIMPORTS = $(BIN)/goimports
$(BIN)/goimports: PACKAGE=golang.org/x/tools/cmd/goimports

.PHONY: goimports
goimports: | $(GOIMPORTS) ; $(info $(M) running goimports…) ## Run goimports
	$Q $(GOIMPORTS) -l -e -w pkg cmd test

.PHONY: fmt
fmt: ; $(info $(M) running gofmt…) @ ## Run gofmt on all source files
	$Q $(GO) fmt $(PKGS)

.PHONY: cli-docs
cli-docs: ; $(info $(M) Generating docs…) ## update docs
	@mkdir -p ./docs/cli ./docs/man/man1
	@go run ./cmd/cli-docs --root=. --target=./docs/cli
	@go run ./cmd/cli-docs --root=. --target=./docs/man/man1 --kind=man

# Misc

.PHONY: clean
clean: ; $(info $(M) cleaning…)	@ ## Cleanup everything
	@rm -rf $(BIN)
	@rm -rf bin
	@rm -rf test/tests.* test/coverage.*

.PHONY: help
help:
	@grep -hE '^[ a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-17s\033[0m %s\n", $$1, $$2}'

.PHONY: version
version:

	@echo $(VERSION)
