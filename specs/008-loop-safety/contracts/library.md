# Library Contract: wrap — the wake budget surface

The wrap package's grown public surface. Everything else in the package
is unchanged.

## Configuration

```go
// Budget bounds what wakes this wrapper will admit. Zero values disable
// the part; a wholly zero Budget is the explicit unbudgeted standing,
// logged once at startup.
type Budget struct {
    MaxHops   int           // provable-chain depth bound D (default 4)
    WindowMax int           // own turn.posts per topic per window K (default 8)
    WindowPer time.Duration // the window W (default 10m)
}

type Config struct {
    // ... existing fields unchanged ...
    Budget Budget
}
```

The opt-out is an explicit field, never a clever zero-value reading:

```go
type Config struct {
    // ...
    Budget     Budget
    Unbudgeted bool // explicit opt-out: no gate, logged once at startup
}
```

`(*Config) ApplyDefaults()` fills `Budget{4, 8, 10m}` exactly when
`Unbudgeted` is false and the whole `Budget` struct is zero (an
untouched config gets the design defaults). Any partly-set Budget is
taken as written — a zero part is that part disabled.

- `Unbudgeted == true` → gate never runs; `ApplyDefaults` leaves Budget
  alone; `Run` logs `wrap_unbudgeted` once.
- `Unbudgeted == false` and `Budget` zero → `ApplyDefaults` fills the
  design defaults.
- `Unbudgeted == false` and `Budget` partly set → taken as written;
  a zero part is that part disabled.
- Negative values → validation error from `Run`, pre-subscription.

## Pure half (correlate.go)

```go
// ParentOf returns the op that provably triggered op (the WakeOpID
// binding) and the number of candidates that matched. 0 = op is a chain
// root; >1 is reported ambiguity, never silently resolved.
func ParentOf(view []Turn, op Turn) (Turn, int)

// ChainToRoot walks provable parents from op to its root. chain[0] is
// op, the last element the root. ambiguous counts multi-match links.
func ChainToRoot(view []Turn, op Turn) (chain []Turn, ambiguous int)

// ProvableHops is len(ChainToRoot)-1: the op's provable distance from a
// chain root.
func ProvableHops(view []Turn, op Turn) int

// BudgetDecision reports whether a wake for persona triggered by trigger
// must be refused under b, with a legible reason carrying the numbers.
// A zero b never refuses.
func BudgetDecision(b Budget, view []Turn, trigger Turn, persona string, now time.Time) (reason string, refuse bool)
```

`Turn` grows `Timestamp time.Time`.

## Behavior (wake.go / wrap.go)

- `handleWake` evaluates `BudgetDecision` after the self-skip and the
  outcome-existence pre-check, before the harness invocation. On refuse:
  log `wake_refused` (persona, topic, trigger op, reason) at Warn, count
  nothing into the record, return a new outcome kind `"refused"`.
- `Run` with `Unbudgeted` logs `wrap_unbudgeted` once before the
  subscription; with a zero-effect budget (both parts disabled by
  explicit values) it does the same.
- No other observable change: admitted wakes behave byte-identically to
  today.

## Compatibility

- Existing callers that zero-value `Config.Budget` get the design
  defaults (the intended breaking-by-default choice, named in the spec).
- `Unbudgeted: true` reproduces today's behavior exactly (SC-005).
- No core (soulstream-core) change of any kind.
