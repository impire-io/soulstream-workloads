.PHONY: fmt tidy build test test-msb lint check

# Format all Go source (gofmt); golangci-lint's formatters also cover goimports.
fmt:
	gofmt -w .

# Keep go.mod/go.sum honest.
tidy:
	go mod tidy

build:
	go build ./...
	go build -o bin/ ./cmd/...

# All tests, no skips.
test:
	go test ./...

# The M1.3 real-microVM proof: needs microsandbox installed (msb doctor
# green). Kept out of `test` so the default suite stays hermetic; the M1.3
# gate is `make check && make test-msb`.
test-msb:
	go test -tags msb_e2e -count=1 -timeout 600s -run 'TestMsb' -v ./integration/

lint:
	golangci-lint run

# The one gate to run before every commit: everything green.
check: fmt tidy build test lint
