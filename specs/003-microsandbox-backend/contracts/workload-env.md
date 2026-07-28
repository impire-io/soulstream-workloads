# Contract: what a workload sees (unchanged from M1.1/M1.2)

A workload artifact, under **any** backend, is started with exactly this
environment — nothing more (clean env, constitution II):

| Variable | Meaning |
|---|---|
| `SOULREALM_NATS_SERVERS` | comma-separated NATS URLs reachable *from where the workload runs* |
| `SOULREALM_NATS_CREDS` | path (valid *inside* the workload's world) to its scoped creds file |
| `SOULREALM_REALM` | realm name |
| `SOULREALM_PERSONA` | the persona the workload acts as |
| `SOULREALM_TOPIC` | the topic path the workload participates in |
| `PATH`, `HOME` | basic operation |

Working directory: the workload's private scratch area (reaped at end of
life; never durable — constitution I).

**The backend adapts values, never names or semantics.** Under msb the creds
path is the in-guest `/scratch/nats.creds` and loopback server hosts are
rewritten to `host.microsandbox.internal`; under native both are host-local.
A workload that honors this contract (as `cmd/agent-echo` and
`cmd/tool-upper` do) runs unmodified under every backend — which is what
SC-001/SC-002 measure.
