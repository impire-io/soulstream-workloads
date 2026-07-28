# Quickstart: run the M1.3 slice by hand

Prove the same declaration runs under both backends on one machine.

## Prerequisites

- Go 1.24+, a NATS server with JetStream on `localhost:4222` (operator mode
  for scope enforcement, open mode is fine for the smoke run)
- microsandbox: `brew install superradcompany/tap/microsandbox` (macOS) or
  `curl -fsSL https://install.microsandbox.dev | sh` (Linux), then
  `msb doctor` → green. Apple Silicon required on macOS.
- Realm env for the runner (as in M1.1): `SOULREALM_REALM`,
  `SOULREALM_PERSONA`, `SOULREALM_REALM_SIGNING_KEY`, `SOULREALM_ROOT_ACCOUNT`.

## 1. Build artifacts — host build for native, guest build for msb

```sh
go build -o bin/ ./cmd/...
GOOS=linux GOARCH=$(go env GOARCH) CGO_ENABLED=0 \
  go build -o bin/linux/agent-echo ./cmd/agent-echo
```

## 2. One declaration, no backend field

`agent.json` (note: adding any backend key makes parsing fail — that is
constitution III enforced):

```json
{
  "role": "agent",
  "lifecycle": "service",
  "persona": "researcher",
  "topic": "<your-topic-path>",
  "artifact": "file:///path/to/bin/linux/agent-echo"
}
```

## 3. Run it natively, then sandboxed — declaration untouched

```sh
# native (default backend; artifact must be the host build)
soulrealm workload start agent.json

# microsandbox (same file byte-for-byte; artifact is the linux build)
SOULREALM_BACKEND=msb soulrealm workload start agent.json
```

Both runs post a turn as `researcher` and drive
`work.open → work.claim → work.done` on the topic. `diff` the declaration
between runs: empty. (The artifact *path* can be identical too if you point
it at the linux build and run native on a Linux host; on macOS the native run
needs the darwin build — node-side provisioning, per the spec's assumptions.)

## 4. See the isolation

```sh
msb ls            # while running: soulrealm-<work-item-id>  running
                  # after:         nothing — the sandbox is reaped
```

## 5. The automated proof

```sh
make check        # hermetic gate: fmt, tidy, build, test (no msb needed), lint
make test-msb     # real-microVM e2e: agent + tool + isolation probe + crash
```
