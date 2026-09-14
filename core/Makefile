BUF ?= buf
GO ?= go

# Run from repo root:  make -C core gen
# Run from core/:      make gen

.PHONY: gen lint breaking check-generated vet test build fmt

gen:
	$(BUF) generate --config ../proto/buf.yaml --template ../proto/buf.gen.yaml

lint:
	$(BUF) lint --config ../proto/buf.yaml

breaking:
	$(BUF) breaking --config ../proto/buf.yaml --against '.git#branch=main'

check-generated: gen
	git diff --exit-code

vet:
	$(GO) vet ./...

test:
	$(GO) test ./... -count=1 -race

build:
	$(GO) build ./...

fmt:
	@test -z "$$($(GO) fmt ./... 2>&1)" || (echo "unformatted files"; $(GO) fmt ./...; exit 1)
