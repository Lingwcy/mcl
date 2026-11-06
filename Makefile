GO := ./scripts/go.sh
GOFILES := $(wildcard *.go) $(shell find internal -name '*.go' -type f)

.PHONY: dev run fmt check-fmt vet test race check build package doctor

dev:
	$(GO) tool mygo dev

run:
	$(GO) run .

fmt:
	$(GO) fmt ./...

check-fmt:
	@unformatted=$$("$$($(GO) env GOROOT)/bin/gofmt" -l $(GOFILES)) && \
		test -z "$$unformatted" || \
		{ printf '%s\n' "$$unformatted" 'Run make fmt to format Go files.'; exit 1; }

vet:
	$(GO) vet ./...
	$(GO) tool mygo vet .

test:
	CGO_ENABLED=0 $(GO) test ./...

race:
	$(GO) test -race ./...

check: check-fmt vet test build

build:
	CGO_ENABLED=0 $(GO) build ./...

package:
	$(GO) tool mygo build

doctor:
	$(GO) tool mygo doctor
