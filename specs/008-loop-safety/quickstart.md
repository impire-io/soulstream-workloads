# Quickstart: the wake budget (consumer view)

## Default — you do nothing

```go
w := &wrap.Wrapper{
    Config: wrap.Config{Persona: "clerk", Template: tpl, Scratch: dir},
    Client: client,
}
_ = w.Run(ctx)
```

An untouched config gets the design budget (depth 4, window 8 per 10
minutes). Ordinary mentions and short delegation chains behave exactly
as before. A runaway agent-wakes-agent cascade halts at the bound; each
refused wake is one `wake_refused` log line with the numbers, and
nothing is posted for it. A refused mention is delayed, not lost: the
next catch-up re-evaluates it.

## Tuning

```go
cfg.Budget = wrap.Budget{MaxHops: 6, WindowMax: 20, WindowPer: time.Hour}
```

A zero part disables that part alone (e.g. `MaxHops: 0, WindowMax: 20,
WindowPer: time.Hour` keeps only the window floor).

## Opting out

```go
cfg.Unbudgeted = true
```

No gate runs; behavior is byte-identical to the pre-budget wrapper; the
standing is logged once at startup (`wrap_unbudgeted`).

## Diagnostics

```go
chain, ambiguous := wrap.ChainToRoot(view, op) // who provably triggered what, to the root
hops := wrap.ProvableHops(view, op)            // the depth the bound counts
```

The walker answers "where did this cascade come from" from the record
alone; outcomes posted outside the deterministic id (the MCP arm) appear
as chain roots — that is precisely why the window floor exists.
