.PHONY: fmt tidy build test test-msb test-k8s test-wake lint check

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

# The M3.2 real-harness proof: wakes an actual `claude -p` (installed and
# logged in on this machine) instead of the scripted harness. Kept out of
# `test` so the default suite stays hermetic; the M3.2 gate is
# `make check && make test-wake`.
test-wake:
	go test -tags wake_e2e -count=1 -timeout 600s -run 'TestWakerLive' -v ./integration/

lint:
	golangci-lint run

# The one gate to run before every commit: everything green.
check: fmt tidy build test lint
