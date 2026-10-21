package launcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ImageSpec lists what an agent adds to the base image.
type ImageSpec struct {
	Packages []string
	Setup    string
	Path     []string
}

func (s ImageSpec) empty() bool {
	return len(s.Packages) == 0 && strings.TrimSpace(s.Setup) == "" && len(s.Path) == 0
}

// EnsureImage returns the image for an agent (rules I1 to I3). It returns
// the base image when spec adds nothing, and builds the derived image when
// its tag does not exist yet.
func (l *Launcher) EnsureImage(ctx context.Context, agent string, spec ImageSpec) (string, error) {
	if spec.empty() {
		return l.Image, nil
	}
	baseID, err := docker(ctx, "image", "inspect", "--format", "{{.Id}}", l.Image).Output()
	if err != nil {
		return "", fmt.Errorf("inspect image %s: %w", l.Image, err)
	}
	tag := imageTag(l.Image, strings.TrimSpace(string(baseID)), spec)
	if docker(ctx, "image", "inspect", tag).Run() == nil {
		return tag, nil
	}
	dir, err := os.MkdirTemp("", "hikari-image-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile(l.Image, spec)), 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "setup.sh"), []byte(spec.Setup+"\n"), 0o755); err != nil {
		return "", err
	}
	cmd := docker(ctx, "build", "--progress", "plain", "--tag", tag, dir)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return "", err
	}
	prefix("build "+agent, out)
	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("build the image of %q: %w", agent, err)
	}
	return tag, nil
}

// imageTag names a derived image after a hash of its base image and of what
// it adds, so that it is built again only when one of them changes.
func imageTag(base, baseID string, spec ImageSpec) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s", baseID,
		strings.Join(spec.Packages, "\x01"), spec.Setup, strings.Join(spec.Path, "\x01"))
	name, _, _ := strings.Cut(base, ":")
	return name + "-" + hex.EncodeToString(h.Sum(nil))[:12]
}

func dockerfile(base string, spec ImageSpec) string {
	var b strings.Builder
	fmt.Fprintf(&b, "FROM %s\nUSER root\n", base)
	if len(spec.Packages) > 0 {
		fmt.Fprintf(&b, "RUN apt-get update \\\n && apt-get install -y --no-install-recommends %s \\\n && rm -rf /var/lib/apt/lists/*\n",
			strings.Join(spec.Packages, " "))
	}
	if strings.TrimSpace(spec.Setup) != "" {
		b.WriteString("COPY setup.sh /tmp/hikari-setup.sh\nRUN bash -euo pipefail /tmp/hikari-setup.sh && rm /tmp/hikari-setup.sh\n")
	}
	if len(spec.Path) > 0 {
		fmt.Fprintf(&b, "ENV PATH=%s:$PATH\n", strings.Join(spec.Path, ":"))
	}
	b.WriteString("USER agent\n")
	return b.String()
}
