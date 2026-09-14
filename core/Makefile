BUF ?= buf
GO ?= go
PROTO_DIR ?= ../proto

# Run from repo root:  make -C core gen
# Run from core/:      make gen
#
# buf is always invoked with CWD=proto/ (the module root). `out: ../core/pkg/protocol`
# in proto/buf.gen.yaml is resolved relative to the working directory, so running buf
# from core/ or from the repo root fails or writes to the wrong place.

.PHONY: gen lint breaking check-generated vet test build fmt

gen:
	cd $(PROTO_DIR) && $(BUF) generate

lint:
	cd $(PROTO_DIR) && $(BUF) lint

# Must run from the repo root so the baseline module root matches the input.
breaking:
	cd .. && $(BUF) breaking --config proto/buf.yaml --against '.git#branch=main'

# Fails on modified *and* newly generated (untracked) files: `git diff --exit-code`
# alone cannot see untracked output.
check-generated: gen
	@if [ -n "$$(git status --porcelain -- . ../ui/lib/generated)" ]; then \
		echo "generated code is stale or uncommitted — commit it:"; \
		git status --porcelain -- . ../ui/lib/generated; \
		exit 1; \
	fi

vet:
	$(GO) vet ./...

test:
	$(GO) test ./... -count=1 -race

build:
	$(GO) build ./...

fmt:
	@test -z "$$($(GO) fmt ./... 2>&1)" || (echo "unformatted files"; $(GO) fmt ./...; exit 1)

