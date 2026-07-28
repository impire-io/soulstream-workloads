# Data Model: 003-microsandbox-backend

No new durable data (constitution I). The entities below are runtime-only.

## Entities

### Sandbox backend (`backend/msb.Backend`)

Node-side configuration for how this node isolates workloads. Never appears
in a declaration.

| Field | Meaning | Default |
|---|---|---|
| `Image` | OCI image booted as the guest | `alpine` |
| `MsbPath` | msb executable to invoke (unit-test seam) | `msb` |
| `HostAlias` | guest name for the host's loopback | `host.microsandbox.internal` |

### Sandbox instance

One per launch; identity `soulrealm-<work-item-id>` — the same id as the
scratch dir, so a topic op, a scratch dir, and a `msb ls` row all correlate.

**States** (all transitions driven by the runner through the seam):

```
(none) --Start--> running --guest exit-------> exited --reap--> (none)
                   |   ^--- guest crash ------^
                   +--Stop (SIGTERM msb, 5s, SIGKILL)--> exited --reap--> (none)
Start failure --> (none)   [scratch cleaned, work.open + work.abandon(start-failed)]
```

**Invariant (SC-004)**: after reap, nothing remains — no `msb ls` row
(`msb rm --force`), no scratch dir, no creds file. The credential also
expires by TTL as a backstop.

### Launch spec mapping (seam crossing)

How the backend maps the seam's `backend.LaunchSpec` onto a sandbox:

| LaunchSpec field | Native | msb |
|---|---|---|
| `Artifact` | exec'd directly | parent dir bind-mounted read-only at `/artifact`; exec `/artifact/<base>` |
| `Args` | argv | argv after the image separator |
| `Cred` | creds file in scratch + env | same creds file; in-guest path `/scratch/nats.creds`; server URLs loopback→`HostAlias` |
| `Realm`/`Topic` | env | same env, unchanged |
| `ScratchDir` | cwd of process | bind-mounted read-write at `/scratch`, guest workdir |

### Workload environment (unchanged contract)

`SOULREALM_NATS_SERVERS`, `SOULREALM_NATS_CREDS`, `SOULREALM_REALM`,
`SOULREALM_PERSONA`, `SOULREALM_TOPIC` — identical names and semantics under
both backends (contracts/workload-env.md). This is why the reference
workloads run unmodified.

## Key validation rules

- Declaration: unchanged; strict parsing already rejects any backend field.
- Backend selection (`SOULREALM_BACKEND`): `native` (default) | `msb`;
  anything else fails before any op is published.
- Server-URL rewrite: only loopback hosts (`127.0.0.1`, `::1`, `localhost`)
  are rewritten to `HostAlias`; other hosts pass through untouched (named
  limitation: they would additionally need the `public` net profile).
