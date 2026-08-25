package wrap

import (
	"context"

	"github.com/impire-io/soulstream-core/realm"

	"github.com/impire-io/soulstream-workloads/artifact"
)

// recordInstructions is the shipped InstructionSource: the declared artefact
// lineage on the soul topic, materialised tip-fresh on every call with the
// engine's own connection (the runtime-side-reads decision). Nothing is
// cached — a revision through ordinary ops reaches the next wake with no
// redeploy, and a runtime death between wakes loses scratch only.
type recordInstructions struct {
	client   *realm.Client
	topic    string
	artefact string
}

// NewRecordInstructions returns the record-backed instruction source for a
// declaration's instructions {topic, artefact} reference.
func NewRecordInstructions(c *realm.Client, topicPath, artefactRef string) InstructionSource {
	return &recordInstructions{client: c, topic: topicPath, artefact: artefactRef}
}

func (r *recordInstructions) Materialise(ctx context.Context) (string, error) {
	data, _, err := artifact.Fetch(ctx, r.client, r.topic, r.artefact)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
