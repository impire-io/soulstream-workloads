.PHONY: fmt tidy build test test-msb test-k8s test-wrap lint check

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

# The M2.1 real-cluster proof: needs a kind cluster + local OCI registry
# (`scripts/kind-registry.sh up`). Kept out of `test` so the default suite
# stays hermetic; the M2.1 gate is `make check && make test-k8s`.
test-k8s:
	go test -tags k8s_e2e -count=1 -timeout 600s -run 'TestK8s' -v ./integration/

# The specs/006 real-harness proof: an actual `claude -p` answers a mention
# through the wrapper (installed and logged in on this machine). Kept out of
# `test` so the default suite stays hermetic; the wrap gate is
# `make check && make test-wrap`.
test-wrap:
	go test -tags wrap_e2e -count=1 -timeout 600s -run TestWrapLive -v ./integration/

lint:
	golangci-lint run

# The one gate to run before every commit: everything green.
check: fmt tidy build test lint
