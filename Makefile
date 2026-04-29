.PHONY: all fmt lint test coverage build check clean

all: check build

fmt:
	gofmt -l -w .

lint:
	go vet ./...

test:
	go test -race -count=1 ./...

coverage:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...
	@go tool cover -func=coverage.out | while IFS= read -r line; do \
		echo "$$line"; \
	done
	@uncovered=$$(go tool cover -func=coverage.out | grep -v '100.0%' | grep -v '^total:' | grep -v '	main	'); \
	if [ -n "$$uncovered" ]; then \
		echo ""; \
		echo "FAIL: the following functions are not at 100% coverage:"; \
		echo "$$uncovered"; \
		exit 1; \
	fi
	@echo ""
	@echo "OK: all functions at 100% coverage (excluding main)"

build:
	go build -o sediment ./cmd/sediment

check: fmt lint coverage

clean:
	rm -f sediment coverage.out coverage.html
