MOD_NAMES     := cnfg validator
MODS_TIDY     := ${MOD_NAMES:%=tidy/%}
MODS_CHECK    := ${MOD_NAMES:%=tidy-check/%}
MODS_TEST     := ${MOD_NAMES:%=test/%}
MODS_LINT     := ${MOD_NAMES:%=lint/%}
MODS_DOWNLOAD := ${MOD_NAMES:%=download/%}
MODS_UPDATE   := ${MOD_NAMES:%=update-deps/%}
MODS_APICHECK := ${MOD_NAMES:%=api-check/%}
MODS_TAG      := ${MOD_NAMES:%=tag/%}

TOOLS_DIR     := internal/tools
BIN           := $(abspath .bin)
GOLANGCILINT  := ${BIN}/golangci-lint
GOTESTSUM     := ${BIN}/gotestsum
GORELEASE     := ${BIN}/gorelease

REMOTE        ?= origin
GO_VERSION    ?= $(shell go env GOVERSION | sed 's/^go//')

mod_dir        = $(if $(filter cnfg,$(1)),.,$(1))
mod_prefix     = $(if $(filter cnfg,$(1)),,$(1)/)
mod_last_tag   = $(shell git ls-remote --tags --refs ${REMOTE} 'refs/tags/$(call mod_prefix,$(1))v*' | sed 's|.*refs/tags/||' | grep -E '^$(call mod_prefix,$(1))v[0-9]' | sort -V | tail -1)

.PHONY: all
all: clean tidy-check .WAIT lint test

.PHONY: tools
tools: ## Build dev tools
	cd ${TOOLS_DIR} && GOBIN=${BIN} go install tool

.NOTPARALLEL: lint
.PHONY: lint
lint: ${MODS_LINT} ## Run linter

.PHONY: ${MODS_LINT}
${MODS_LINT}: | tools
	cd $(call mod_dir,${@F}) && ${GOLANGCILINT} run --timeout=15m ./...

.PHONY: test
test: ${MODS_TEST} ## Run tests

.PHONY: ${MODS_TEST}
${MODS_TEST}: | tools
	mkdir -p .output/coverage .output/junit
	cd $(call mod_dir,${@F}) && ${GOTESTSUM} --junitfile=$(abspath .output/junit)/${@F}.xml -- \
		-race -covermode=atomic -coverprofile=$(abspath .output/coverage)/${@F}.txt ./...

.PHONY: tidy
tidy: ${MODS_TIDY} tidy/tools ## Tidy all mods

.PHONY: ${MODS_TIDY}
${MODS_TIDY}:
	cd $(call mod_dir,${@F}) && go mod tidy

.PHONY: tidy/tools
tidy/tools:
	cd ${TOOLS_DIR} && go mod tidy

.PHONY: download
download: ${MODS_DOWNLOAD} download/tools ## Download deps for all mods

.PHONY: ${MODS_DOWNLOAD}
${MODS_DOWNLOAD}:
	cd $(call mod_dir,${@F}) && go mod download

.PHONY: download/tools
download/tools:
	cd ${TOOLS_DIR} && go mod download

.PHONY: tidy-check
tidy-check: ${MODS_CHECK} tidy-check/tools ## Check if all mods are tidy

.PHONY: ${MODS_CHECK}
${MODS_CHECK}:
	cd $(call mod_dir,${@F}) && go mod tidy
	git diff --exit-code --name-status -- $(call mod_dir,${@F})/go.mod $(call mod_dir,${@F})/go.sum

.PHONY: tidy-check/tools
tidy-check/tools:
	cd ${TOOLS_DIR} && go mod tidy
	git diff --exit-code --name-status -- ${TOOLS_DIR}/go.mod ${TOOLS_DIR}/go.sum

.PHONY: update-deps
update-deps: ${MODS_UPDATE} update-deps/tools ## Update all deps

.PHONY: ${MODS_UPDATE}
${MODS_UPDATE}:
	cd $(call mod_dir,${@F}) && go mod edit -go=${GO_VERSION}
	cd $(call mod_dir,${@F}) && go get $$(go mod edit -json | jq -r '[(.Require[]? | select(.Indirect | not) | .Path)] | map(. + "@latest") | .[]')
	cd $(call mod_dir,${@F}) && go mod tidy

.PHONY: update-deps/tools
update-deps/tools:
	cd ${TOOLS_DIR} && go mod edit -go=${GO_VERSION}
	cd ${TOOLS_DIR} && go get $$(go mod edit -json | jq -r '[.Tool[]?.Path] | map(. + "@latest") | .[]')
	cd ${TOOLS_DIR} && go mod tidy

.PHONY: api-check
api-check: ${MODS_APICHECK} ## Fail on breaking API changes vs the latest tag

.PHONY: ${MODS_APICHECK}
${MODS_APICHECK}: MOD  = ${@F}
${MODS_APICHECK}: LAST = $(call mod_last_tag,${@F})
${MODS_APICHECK}: BASE = $(if $(LAST),$(LAST:$(call mod_prefix,${@F})%=%),none -version=v0.0.1)
${MODS_APICHECK}: | tools
	cd $(call mod_dir,${MOD}) && ${GORELEASE} -base=${BASE}

.PHONY: tag
tag: ${MODS_TAG} ## Tag any mod that has changes since its last tag

.PHONY: ${MODS_TAG}
${MODS_TAG}: MOD  = ${@F}
${MODS_TAG}: LAST = $(call mod_last_tag,${@F})
${MODS_TAG}: BASE = $(LAST:$(call mod_prefix,${@F})%=%)
${MODS_TAG}: | tools
	@v="v0.0.1"; if [ -n "${LAST}" ]; then \
	  v=$$(cd $(call mod_dir,${MOD}) && ${GORELEASE} -base=${BASE} | tee /dev/stderr | awk '/^Suggested version:/ {print $$3; exit}'); \
	  test -n "$$v" || { echo "${MOD}: gorelease did not suggest a version" >&2; exit 1; }; \
	fi; \
	git tag "$(call mod_prefix,${MOD})$$v" && echo "tagged $(call mod_prefix,${MOD})$$v"

.PHONY: clean
clean: ## Clean files
	git clean -Xdf
