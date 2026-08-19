// Package fleet is the placement plane (hq design
// 02-DESIGN/soulstream-workloads/0003-fleet.md, M3.1): several nodes,
// one realm, no coordinator and no consensus — **placement IS
// work.claim**. A submission is an ordinary work item carrying a
// declaration; every idle node races to claim it; the log decides, and
// exactly one node launches. Reclaim is the design's three-step:
// projection nominates a silent owner, a transient probe vetoes a live
// one, and an ordinary work.abandon reopens the item for a fresh race.
//
// Nothing here touches the runner, declaration, or backend seams: the
// winning node calls Runner.Launch exactly as a single node does, so
// the run's own lifecycle stays the runner's work item and the
// placement item is the fleet's.
package fleet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/impire-io/soulstream-core/topic"

	"github.com/impire-io/soulstream-workloads/declaration"
	"github.com/impire-io/soulstream-workloads/runner"
)

// submissionMarker prefixes the declaration JSON inside a placement
// item's body: machine-readable, and still legible to a human reading
// the topic.
const submissionMarker = "soulstream-workloads/placement v1\n"

// ProbeSubject is where a node answers liveness probes for one
// placement. Transient request/reply on soulstream-workloads' own
// service space — never the op-log (design §5: probe traffic appears
// nowhere in the stream).
func ProbeSubject(node string) string {
	return "SOULSTREAM.SVC.FLEET." + node
}

// Submit opens a placement item carrying the declaration. Any persona
// may submit; the fleet decides where it runs.
func Submit(ctx context.Context, h *topic.Handle, d declaration.Declaration) (string, error) {
	if err := d.Validate(); err != nil {
		return "", fmt.Errorf("fleet: invalid declaration: %w", err)
	}
	body, err := json.Marshal(d)
	if err != nil {
		return "", fmt.Errorf("fleet: encode declaration: %w", err)
	}
	title := fmt.Sprintf("place %s as %s", d.Artifact, d.Persona)
	return h.OpenWork(ctx, title, submissionMarker+string(body))
}

// placementOf reads a work item's declaration, or false when the item
// is an ordinary one (a runner's own lifecycle item, say).
func placementOf(item topic.WorkItem) (declaration.Declaration, bool) {
	rest, ok := strings.CutPrefix(item.Body, submissionMarker)
	if !ok {
		return declaration.Declaration{}, false
	}
	var d declaration.Declaration
	if err := json.Unmarshal([]byte(rest), &d); err != nil {
		return declaration.Declaration{}, false
	}
	return d, true
}

// Node is one fleet member: it races for placements, launches what it
// wins, answers probes for what it owns, and reclaims what a dead peer
// left behind.
type Node struct {
	// ID names this node — the persona its claims are attributed to
	// (the topic client's persona; kept here for probe addressing).
	ID string
	// Conn carries the transient probe traffic.
	Conn *nats.Conn
	// Runner launches what this node wins. Untouched by fleet-ness.
	Runner *runner.Runner
	// Reclaim bounds how long a claimed placement may go unanswered
	// before this node nominates it for reclaim. Zero: 10s.
	Reclaim time.Duration
	// ProbeTimeout bounds one liveness probe. Zero: 250ms.
	ProbeTimeout time.Duration

	// owned tracks placements this node is running, so it can answer
	// probes for them.
	owned map[string]struct{}
	sub   *nats.Subscription
}

func (n *Node) reclaimBound() time.Duration {
	if n.Reclaim > 0 {
		return n.Reclaim
	}
	return 10 * time.Second
}

func (n *Node) probeTimeout() time.Duration {
	if n.ProbeTimeout > 0 {
		return n.ProbeTimeout
	}
	return 250 * time.Millisecond
}

// Start begins answering liveness probes. Every node answers for the
// placements it owns; an unknown placement answers "no" so a stale
// owner cannot veto its own reclaim.
func (n *Node) Start() error {
	n.owned = map[string]struct{}{}
	sub, err := n.Conn.Subscribe(ProbeSubject(n.ID), func(msg *nats.Msg) {
		if _, ok := n.owned[string(msg.Data)]; ok {
			_ = msg.Respond([]byte("alive"))
			return
		}
		_ = msg.Respond([]byte("no"))
	})
	if err != nil {
		return fmt.Errorf("fleet: probe subscription: %w", err)
	}
	if err := n.Conn.Flush(); err != nil {
		return fmt.Errorf("fleet: probe flush: %w", err)
	}
	n.sub = sub
	return nil
}

// Stop ends probe answering — the node goes silent, which is what a
// death looks like to its peers.
func (n *Node) Stop() {
	if n.sub != nil {
		_ = n.sub.Drain()
		n.sub = nil
	}
}

// Placement is one won item: the placement work item and the running
// workload underneath it.
type Placement struct {
	ItemID string
	Decl   declaration.Declaration
	Run    *runner.Running
}

// ErrNotWon means the log gave this placement to someone else — the
// ordinary outcome of a contested race, never an error worth retrying.
var ErrNotWon = errors.New("fleet: another node won this placement")

// TryPlace races for one open placement: claim, re-read the log, and
// launch only if the projection says this node owns it. The log is the
// arbiter — first claim in stream order wins, the rest void.
func (n *Node) TryPlace(ctx context.Context, h *topic.Handle, itemID string) (*Placement, error) {
	mt, err := h.Materialise(ctx)
	if err != nil {
		return nil, err
	}
	item, ok := findItem(mt, itemID)
	if !ok {
		return nil, fmt.Errorf("fleet: no placement %s", itemID)
	}
	d, ok := placementOf(item)
	if !ok {
		return nil, fmt.Errorf("fleet: work item %s is not a placement", itemID)
	}
	if item.Status != topic.WorkOpen {
		return nil, ErrNotWon
	}
	if _, err := h.ClaimWork(ctx, itemID); err != nil {
		return nil, fmt.Errorf("fleet: claim %s: %w", itemID, err)
	}
	// Publishing a claim never means winning it: read the log back.
	mt, err = h.Materialise(ctx)
	if err != nil {
		return nil, err
	}
	item, ok = findItem(mt, itemID)
	if !ok || item.Owner != n.ID {
		return nil, ErrNotWon
	}

	run, err := n.Runner.Launch(ctx, h, d)
	if err != nil {
		// Won it and could not run it: hand it straight back, so another
		// node may try rather than the item hanging on a dead claim.
		if _, aerr := h.AbandonWork(ctx, itemID); aerr != nil {
			return nil, fmt.Errorf("fleet: launch failed (%v) and abandon failed: %w", err, aerr)
		}
		return nil, fmt.Errorf("fleet: launch %s: %w", itemID, err)
	}
	n.owned[itemID] = struct{}{}
	return &Placement{ItemID: itemID, Decl: d, Run: run}, nil
}

// Release ends a placement this node owns: the workload stops and the
// placement item closes as done.
func (n *Node) Release(ctx context.Context, h *topic.Handle, p *Placement) error {
	delete(n.owned, p.ItemID)
	if err := p.Run.Stop(ctx); err != nil {
		return err
	}
	_, err := h.CompleteWork(ctx, p.ItemID)
	return err
}

// Sweep is the reclaim pass (design §6): for every placement claimed by
// a peer and unanswered past the bound, probe the owner — a live owner
// vetoes, a silent one is abandoned back into the race. Returns the
// items this pass reopened.
func (n *Node) Sweep(ctx context.Context, h *topic.Handle) ([]string, error) {
	mt, err := h.Materialise(ctx)
	if err != nil {
		return nil, err
	}
	var reopened []string
	for _, item := range mt.WorkItems {
		if item.Status != topic.WorkClaimed || item.Owner == n.ID {
			continue
		}
		if _, ok := placementOf(item); !ok {
			continue
		}
		if claimedAt, ok := lastClaimAt(item); !ok || time.Since(claimedAt) < n.reclaimBound() {
			continue
		}
		// The projection nominated it; the probe decides. A live owner
		// vetoes at zero cost on the record.
		if n.alive(item.Owner, item.ID) {
			continue
		}
		if _, err := h.AbandonWork(ctx, item.ID); err != nil {
			return reopened, fmt.Errorf("fleet: reclaim %s: %w", item.ID, err)
		}
		reopened = append(reopened, item.ID)
	}
	return reopened, nil
}

// alive probes one owner for one placement over the transient service
// subject. No answer, a refusal, or an error all read as not-alive:
// the projection already nominated it, and the probe only vetoes.
func (n *Node) alive(owner, itemID string) bool {
	msg, err := n.Conn.Request(ProbeSubject(owner), []byte(itemID), n.probeTimeout())
	if err != nil {
		return false
	}
	return string(msg.Data) == "alive"
}

// OpenPlacements lists the placement items currently up for grabs.
func OpenPlacements(mt *topic.MaterializedTopic) []topic.WorkItem {
	var out []topic.WorkItem
	for _, item := range mt.WorkItems {
		if item.Status != topic.WorkOpen {
			continue
		}
		if _, ok := placementOf(item); ok {
			out = append(out, item)
		}
	}
	return out
}

func findItem(mt *topic.MaterializedTopic, id string) (topic.WorkItem, bool) {
	for _, item := range mt.WorkItems {
		if item.ID == id {
			return item, true
		}
	}
	return topic.WorkItem{}, false
}

// lastClaimAt is when the current owner's claim landed — the clock the
// reclaim bound runs against.
func lastClaimAt(item topic.WorkItem) (time.Time, bool) {
	for i := len(item.Timeline) - 1; i >= 0; i-- {
		ev := item.Timeline[i]
		if ev.Kind == "claim" && !ev.Void {
			return ev.Timestamp, true
		}
	}
	return time.Time{}, false
}
