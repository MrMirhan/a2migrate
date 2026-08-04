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

# Requires goreleaser (https://goreleaser.com/install/).
snapshot:
	goreleaser release --snapshot --skip=publish --clean

# Tag a release locally; pushes happen via the release pipeline.
release:
	@if [ -z "$$TAG" ]; then echo "usage: make release TAG=v0.1.0"; exit 1; fi
	git tag -s $$TAG -m "Release $$TAG"
	git push origin $$TAG
	@echo "tag $$TAG pushed; release workflow should start shortly"

# Verify goreleaser config locally.
release-check:
	goreleaser check