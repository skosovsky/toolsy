GO                     := go
GOLANGCI_LINT_VERSION := v2.14.0
GOLANGCI_LINT         := $(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
MODULES                := $(shell find . -type d \( -name ".*" -not -name "." -o -name "vendor" \) -prune -o -type f -name "go.mod" -exec dirname {} \;)

.PHONY: lint fix test bench bench-hotpath fuzz cover task34-preflight release-patch release-break

lint:
	@for dir in $(MODULES); do \
		echo "golangci-lint $(GOLANGCI_LINT_VERSION) - $$dir"; \
		(cd "$$dir" && $(GOLANGCI_LINT) run --allow-serial-runners ./...) || exit 1; \
	done

fix:
	@if [ -f "go.work" ]; then $(GO) work sync; fi
	@for dir in $(MODULES); do \
		echo "fix & tidy - $$dir"; \
		(cd "$$dir" && $(GO) fix ./... && $(GO) mod tidy) || exit 1; \
		(cd "$$dir" && $(GOLANGCI_LINT) run --allow-serial-runners --fix ./...) || exit 1; \
	done

test:
	@for dir in $(MODULES); do \
		echo "test - $$dir"; \
		(cd "$$dir" && $(GO) test -v -race ./...) || exit 1; \
	done

bench:
	@for dir in $(MODULES); do \
		echo "bench - $$dir"; \
		(cd "$$dir" && $(GO) test -bench=. -run=^$$ ./...) || exit 1; \
	done

fuzz:
	@for dir in $(MODULES); do \
		echo "fuzz - $$dir"; \
		(cd "$$dir" && \
			for pkg in $$($(GO) list -tags=fuzz ./...); do \
				if $(GO) test -tags=fuzz -list . "$$pkg" 2>/dev/null | grep -q '^Fuzz'; then \
					$(GO) test -tags=fuzz -fuzz=. -fuzztime=30s "$$pkg" || exit 1; \
				fi; \
			done \
		) || exit 1; \
	done

cover:
	@for dir in $(MODULES); do \
		echo "cover - $$dir"; \
		(cd "$$dir" && $(GO) test -coverprofile=coverage.out ./... && $(GO) tool cover -func=coverage.out) || exit 1; \
	done

task34-preflight: ## non-destructive MCP 2026-07-28 clear-break contract gate
	@./scripts/task34-preflight.sh

release-patch: ## v0.5.0 -> v0.5.1
	@bash ./scripts/release.sh patch

release-break: ## destructive v0.5.1 -> v0.6.0 release after preflight
	@bash ./scripts/release.sh break
