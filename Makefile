TOOLS_DIR    := internal/tools
BIN          := $(abspath .bin)
GOLANGCILINT := ${BIN}/golangci-lint
GOTESTSUM    := ${BIN}/gotestsum
GORELEASE    := ${BIN}/gorelease

REMOTE     ?= origin
GO_VERSION ?= $(shell go env GOVERSION | sed 's/^go//')
LAST_TAG    = $(shell git ls-remote --tags --refs ${REMOTE} 'refs/tags/v*' | sed 's|.*refs/tags/||' | grep -E '^v[0-9]' | sort -V | tail -1)

all: clean tidy-check lint api-check test

.PHONY: tools
tools: ## Build dev tools
	cd ${TOOLS_DIR} && GOBIN=${BIN} go install tool

.PHONY: lint
lint: | tools ## Run linter
	${GOLANGCILINT} run --timeout=15m ./...

.PHONY: test
test: | tools ## Run tests
	${GOTESTSUM} --junitfile=junit.xml -- -race -covermode=atomic -coverprofile=coverage.txt ./...

.PHONY: tidy
tidy: ## Tidy go.mod of the module and the tools
	go mod tidy
	cd ${TOOLS_DIR} && go mod tidy

.PHONY: tidy-check
tidy-check: tidy ## Check that go.mod and go.sum are tidy
	git diff --exit-code --name-status -- go.mod go.sum ${TOOLS_DIR}/go.mod ${TOOLS_DIR}/go.sum

.PHONY: api-check
api-check: BASE = $(if ${LAST_TAG},${LAST_TAG},none -version=v0.0.1)
api-check: | tools ## Fail on breaking API changes vs the latest tag
	${GORELEASE} -base=${BASE}

.PHONY: tag
tag: | tools ## Tag commit using gorelease's suggested version, prints version on stdout
	@v="v0.0.1"; if [ -n "${LAST_TAG}" ]; then \
	  v=$$(${GORELEASE} -base=${LAST_TAG} | tee /dev/stderr | awk '/^Suggested version:/ {print $$3; exit}'); \
	  test -n "$$v" || { echo "gorelease did not suggest a version" >&2; exit 1; }; \
	fi; \
	git tag "$$v" >&2 && echo "$$v"

.PHONY: update-deps
update-deps: ## Update Go version, tools, and deps
	go mod edit -go=${GO_VERSION}
	go get $$(go mod edit -json | jq -r '[(.Require[]? | select(.Indirect | not) | .Path)] | map(. + "@latest") | .[]')
	go mod tidy
	cd ${TOOLS_DIR} && go mod edit -go=${GO_VERSION}
	cd ${TOOLS_DIR} && go get $$(go mod edit -json | jq -r '[.Tool[]?.Path] | map(. + "@latest") | .[]')
	cd ${TOOLS_DIR} && go mod tidy

.PHONY: clean
clean: ## Clean files
	git clean -Xdf
