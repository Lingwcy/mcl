GO := ./scripts/go.sh
GOFILES := $(wildcard *.go) $(shell find internal -name '*.go' -type f)

.PHONY: dev run fmt check-fmt vet test race check build package package-signed doctor

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

# Ad-hoc signed and not notarized: runs on this Mac only.
package:
	$(GO) tool mygo build -sign - -skip-notarize

# Developer ID signed and notarized, as releases are. Needs the identity
# (name or SHA-1 from `security find-identity -v -p codesigning`) and the
# notarytool profile mcl-notary in the login keychain.
package-signed:
	@test -n "$$MACOS_SIGNING_IDENTITY" || { echo 'Set MACOS_SIGNING_IDENTITY to a Developer ID Application identity.'; exit 1; }
	$(GO) tool mygo build -sign "$$MACOS_SIGNING_IDENTITY"

doctor:
	$(GO) tool mygo doctor
