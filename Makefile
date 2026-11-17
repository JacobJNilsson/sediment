.PHONY: all fmt lint antislop antislop-all test coverage build check clean

ANTISLOP := go run github.com/JacobJNilsson/anti-slop-go/cmd/antislop@v1.4.1
# A rule joins this list when `make antislop-all` reports zero findings for it.
# The goal is to enforce every rule.
ANTISLOP_ENFORCED := -nolaundering -noerrorassert -noreflect -noadhoctypeswitch -noanyreturn -justifypanic -nointerfacereturn -separategotwant -nomonkeypatch

all: check build

fmt:
	gofmt -l -w .

lint:
	go vet ./...

antislop:
	$(ANTISLOP) $(ANTISLOP_ENFORCED) ./...

antislop-all:
	-$(ANTISLOP) ./...

test:
	go test -race -count=1 ./...

coverage:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...
	@go tool cover -func=coverage.out | while IFS= read -r line; do \
		echo "$$line"; \
	done
	@uncovered=$$(go tool cover -func=coverage.out | grep -v '100.0%' | grep -v '^total:' | grep -v '\.go:[0-9]*:[[:space:]]*main[[:space:]]'); \
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

check: fmt lint antislop coverage

clean:
	rm -f sediment coverage.out coverage.html
