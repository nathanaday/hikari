package topology

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestExamples loads each example topology file through compose. The shop
// needs the demo repo, which `make demo-repo` creates.
func TestExamples(t *testing.T) {
	if exec.Command("docker", "compose", "version").Run() != nil {
		t.Skip("docker compose is not available")
	}
	files, _ := filepath.Glob("../../examples/*/topology.yaml")
	if len(files) == 0 {
		t.Fatal("no examples found")
	}
	for _, f := range files {
		t.Run(filepath.Base(filepath.Dir(f)), func(t *testing.T) {
			topo, err := Load(t.Context(), f)
			if err != nil {
				if _, statErr := os.Stat("../../.hikari/demo/shop"); os.IsNotExist(statErr) && filepath.Base(filepath.Dir(f)) == "shop" {
					t.Skip("run `make demo-repo` first")
				}
				t.Fatal(err)
			}
			if topo.File == "" || len(topo.Agents) == 0 {
				t.Errorf("%+v", topo)
			}
		})
	}
}
