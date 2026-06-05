# fate developer Makefile. The engine is a zero-dependency module; the Temporal
# integration lives in ./temporal with its own module.

.PHONY: all test test-race cover lint vet fmt examples temporal tidy

all: test-race lint

test:
	go test ./...

test-race:
	go test -race ./...
	cd temporal && go test -race ./...

cover:
	go test -coverprofile=cover.out $$(go list ./... | grep -vE '/examples/|/cmd/|/internal/demos')
	@go tool cover -func=cover.out | tail -1
	@total=$$(go tool cover -func=cover.out | awk '/^total:/ {print substr($$3,1,length($$3)-1)}'); \
		awk "BEGIN { exit !($$total >= 85) }" || { echo "coverage $$total% below 85% gate"; exit 1; }

vet:
	go vet ./...
	cd temporal && go vet ./...

fmt:
	gofmt -w .

lint:
	golangci-lint run

examples:
	go run ./examples/quickstart
	go run ./examples/trafficlight
	go run ./examples/realtime-timer

temporal:
	cd temporal && go test ./...

tidy:
	go mod tidy
	cd temporal && go mod tidy
