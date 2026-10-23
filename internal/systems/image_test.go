package systems

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/images"
	"github.com/nathanaday/hikari/internal/testmap"
)

// needImage skips a test when Docker or the image it builds from is
// missing.
func needImage(t *testing.T, image string) {
	t.Helper()
	if err := exec.Command("docker", "image", "inspect", image).Run(); err != nil {
		t.Skipf("needs Docker with the image %s", image)
	}
}

func TestImageRelease(t *testing.T) {
	needImage(t, "hikari-agent")
	f := newFixture(t)
	defineDeliverable(t, f.dir, "python-library", pythonLibrary+"image: {dockerfile: Dockerfile}\n")
	write(t, f.repo, "services/orders/Dockerfile", "FROM hikari-agent\nCOPY create.py /app/create.py\n")
	f.describeOrders(t)
	f.runOrders(t, testmap.Passed)
	staging := filepath.Join(f.dir, "store", "systems", "orders", "staging", "python-library")
	write(t, staging, "README.md", "# orders\n")
	write(t, staging, "orders/create.py", "def create(): pass\n")
	_, err := f.m.MarkCurrent("orders")
	must(t, err)
	if _, err := f.m.CheckRelease("orders", "python-library"); err != nil {
		t.Fatalf("IM4: %v", err)
	}
	rel, _, err := f.m.PublishRelease("orders", api.PublishRequest{Deliverable: "python-library", Version: "1.0.0", Notes: "First."})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { exec.Command("docker", "image", "rm", "--force", rel.Image.Tag).Run() })
	if rel.Image == nil || rel.Image.Tag != images.Tag("test", "orders", "python-library", "1.0.0") || !strings.HasPrefix(rel.Image.ID, "sha256:") || rel.Image.Platform != images.HostPlatform() {
		t.Fatalf("IM3: image = %+v", rel.Image)
	}
	id, _, err := images.Inspect(context.Background(), rel.Image.Tag)
	if err != nil || id != rel.Image.ID {
		t.Errorf("IM2: the image is in Docker: %s, %v", id, err)
	}
	out, _ := exec.Command("docker", "run", "--rm", "--entrypoint", "cat", rel.Image.Tag, "/app/create.py").Output()
	if string(out) != "def create(): pass\n" {
		t.Errorf("IM2: the image holds the described commit's file: %q", out)
	}

	// A broken Dockerfile fails the publish and changes nothing.
	write(t, f.repo, "services/orders/Dockerfile", "FROM hikari-agent\nCOPY nowhere /x\n")
	write(t, staging, "README.md", "# orders\n")
	write(t, staging, "orders/create.py", "def create(): pass\n")
	_, err = f.m.MarkCurrent("orders")
	must(t, err)
	if _, _, err := f.m.PublishRelease("orders", api.PublishRequest{Deliverable: "python-library", Version: "1.0.1", Notes: "Broken."}); err == nil || !strings.Contains(err.Error(), "IM2") {
		t.Errorf("IM2: a failed build = %v", err)
	}
	if versions, _ := f.m.Store().Versions("orders", "python-library"); len(versions) != 1 {
		t.Errorf("IM2: a failed build published %v", versions)
	}
}
