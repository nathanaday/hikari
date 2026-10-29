package linked

import (
	"context"
	"fmt"

	"github.com/nathanaday/hikari/internal/topology"
)

// PullPins pulls the product of every pin that is absent on this machine
// (rule PN5 of the releases spec), and reports one line for each pin. A
// pin that cannot be pulled is a warning; a pull that fails is a failure.
func PullPins(ctx context.Context, topo *topology.Topology) []Result {
	var out []Result
	for _, r := range topo.VaultOf().PullPinned(ctx, topo.PartitionAgents()) {
		text := r.Text
		if r.Pin.Partition != "" {
			text = fmt.Sprintf("pins %s %s %s: %s", r.Pin.Partition, r.Pin.Deliverable, r.Pin.Ref(), r.Text)
		}
		out = append(out, Result{Repo: r.Consumer, Text: text, Failed: r.Failed, Warning: r.Warning})
	}
	return out
}

// PinFindings turns the pin problems of a vault into findings of a check
// (rule PN6): warnings, which stop nothing.
func PinFindings(topo *topology.Topology) []Finding {
	var out []Finding
	for _, p := range topo.VaultOf().PinProblems(topo.PartitionAgents()) {
		out = append(out, Finding{Text: p, Warning: true})
	}
	return out
}
