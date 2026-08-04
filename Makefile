.PHONY: build test test-race lint clean install run

BIN := a2migrate
PKG := ./cmd/a2migrate

build:
	go build -o $(BIN) $(PKG)

install:
	go install $(PKG)

test:
	go test ./...

test-race:
	go test -race ./...

lint:
	go vet ./...

clean:
	rm -f $(BIN)

run:
	go run $(PKG)

# Coverage report
cover:
	go test -coverprofile=coverage.txt ./...
	go tool cover -html=coverage.txt -o coverage.html