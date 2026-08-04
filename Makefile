.PHONY: build test test-race lint clean install run snapshot release

BIN := a2migrate
PKG := ./cmd/a2migrate
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

build:
	go build -ldflags "-X github.com/mirhan/a2migrate/internal/version.Version=$(VERSION)" -o $(BIN) $(PKG)

install:
	go install $(PKG)

test:
	go test ./...

test-race:
	go test -race ./...

lint:
	go vet ./...

fmt:
	gofmt -w .

clean:
	rm -f $(BIN)

run:
	go run $(PKG)

cover:
	go test -coverprofile=coverage.txt ./...
	go tool cover -html=coverage.txt -o coverage.html

# Validate the GoReleaser configuration without building anything.
# Useful as a CI gate before tagging a release.
# Requires goreleaser (https://goreleaser.com/install/).
release-check:
	goreleaser check

# Build every binary locally without publishing. Output lands in ./dist/.
# Equivalent to `goreleaser release --snapshot --skip=publish --clean`.
# Catches ldflags / CGO / path issues before pushing a tag.
snapshot:
	goreleaser release --snapshot --skip=publish --clean

# Tag a release locally; pushes happen via the release pipeline.
release:
	@if [ -z "$$TAG" ]; then echo "usage: make release TAG=v0.1.0"; exit 1; fi
	git tag -s $$TAG -m "Release $$TAG"
	git push origin $$TAG
	@echo "tag $$TAG pushed; release workflow should start shortly"

# Print what `make release TAG=v0.x.0` would do without pushing anything.
release-dry-run:
	@echo "Would: git tag -s $(TAG) -m 'Release $(TAG)'"
	@echo "Would: git push origin $(TAG)"
	@echo "Then .github/workflows/release.yml runs goreleaser, which:"
	@echo "  - builds 5 OS/arch targets (linux/amd64, linux/arm64,"
	@echo "    darwin/amd64, darwin/arm64, windows/amd64)"
	@echo "  - produces a2migrate_Linux_x86_64.tar.gz etc. + SHA256SUMS"
	@echo "  - drafts the GitHub Release from CHANGELOG.md entries"
	@echo "  - attaches the binaries"
	@echo ""
	@echo "Run \`make release-check\` first to validate .goreleaser.yaml."
	@echo "Run \`make snapshot\` to build everything locally."

.PHONY: release release-check release-dry-run snapshot

