package modules

import (
	"slices"
	"testing"

	"github.com/nathanaday/hikari/internal/topology"
)

// TestKnown holds topology.KnownModules equal to the registry, since the
// topology cannot import the modules.
func TestKnown(t *testing.T) {
	var names []string
	for _, m := range All {
		names = append(names, m.Name())
	}
	if !slices.Equal(names, topology.KnownModules) {
		t.Errorf("All = %v, topology.KnownModules = %v", names, topology.KnownModules)
	}
}
