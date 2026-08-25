# Data Model: Loop safety — the wake budget

No stored data: every value below is configuration or derived per wake
from the topic view (constitution soulstream-workloads I — the runtime
keeps no durable truth).

## Budget (configuration, on `wrap.Config`)

| Field | Type | Default | Zero means |
|---|---|---|---|
| `MaxHops` | int | 4 | depth bound disabled |
| `WindowMax` | int | 8 | window floor disabled |
| `WindowPer` | duration | 10m | (only meaningful with `WindowMax` > 0) |

Validation: negative values are refused by config validation (a teaching
error, pre-run). Both zero = unbudgeted; logged once at startup.

## Turn (wrap's local view slice — grown)

Existing `{OpID, Author, Type, Body}` grows `Timestamp` (the
contribution's server-stamped stream time). It is the window clock and
carries no other new duty. The mapping in `agentRealm.Read` is the only
producer; test fakes set it explicitly where a test exercises the window.

## Provable hop (derived)

An edge `outcome → trigger` that verifies as
`WakeOpID(trigger.OpID, outcome.Author) == outcome.OpID`. Not stored;
recomputed from the view on demand.

## Chain (derived)

`chain(op) = [op, parent(op), parent²(op), …, root]` where `root` has no
provable parent. Depth = `len(chain) - 1`. Ambiguity (an op with >1
parent match) is carried in the walk result, never absorbed.

## Admission decision (derived, per wake)

```
refuse iff (MaxHops > 0 and provableHops(trigger) + 1 > MaxHops)
        or (WindowMax > 0 and ownTurnPosts(view, persona, WindowPer) >= WindowMax)
```

Evaluated once per wake delivery, after self-skip and outcome-existence,
before the harness. The decision's reason (which part, with numbers) is
the `wake_refused` log payload. A refusal changes nothing in the record;
re-delivery of the same mention re-evaluates against the then-current
view.

## State transitions

None. A wake is admitted (existing outcome contract unchanged: exactly
one outcome op) or refused (nothing posted; the mention remains
re-presentable). There is no third state and no stored transition.
