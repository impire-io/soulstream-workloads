# Contract: the backend seam

The interface both backends satisfy — frozen by M1.1 (SC-005), reproven here
by a second implementation. Defined in `backend/backend.go`; this document
states the behavioral contract an implementation must honor, which the unit
tests of each backend assert.

## Interface

```go
type Backend interface {
    Start(ctx context.Context, spec LaunchSpec) (Handle, error)
}

type Handle interface {
    Wait() ExitStatus              // blocks; reaps; idempotent
    Stop(ctx context.Context) error // graceful, escalating
}
```

## Behavioral requirements

1. **Start** launches exactly one workload process/VM from `spec.Artifact` +
   `spec.Args`, with `spec.Cred` delivered as a NATS creds file and the
   `SOULREALM_*` env contract (contracts/workload-env.md). On failure it
   returns an error and leaves nothing behind (scratch cleaned, no sandbox).
2. **Clean environment**: the workload sees only the documented env contract —
   never the parent soulrealm process's environment (constitution II; the
   realm signing key must not be able to leak).
3. **Wait** blocks until the workload ends, returns how (exit code XOR
   signal), reaps everything the launch created (process/VM record, scratch
   dir, creds file), and is safe to call more than once (same status).
4. **Stop** requests termination, waits a bounded grace (5 s), then kills.
   Stop does not itself publish anything; the runner decides the terminal op.
5. **No ops, no control channel**: a backend never publishes to NATS and
   never creates a private subject; lifecycle publication belongs to the
   runner alone (constitutions I and V).
6. **Exit fidelity**: a guest/workload exit code N surfaces as
   `ExitStatus{Code: N}`; a termination by signal surfaces as
   `ExitStatus{Signal: <name>}` — so `runner.Outcome` maps identically for
   every backend.

## msb-specific mapping (informative)

- child process = `msb run --no-tty --quiet --name soulrealm-<id> …` (exit
  code of the guest command propagates — measured, research D1/D4); the
  artifact enters the guest by pre-boot copy (`--copy-file`), never by
  exposing the host path; mount/copy sources are symlink-resolved (D3);
- Stop = SIGTERM to the msb process (stops the VM — measured), SIGKILL after
  grace; Wait additionally runs `msb rm --force soulrealm-<id>`;
- network: deny-by-default with only the `host` destination group granted;
  loopback NATS URLs rewritten to `host.microsandbox.internal` (research D2).
