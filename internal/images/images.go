// Package images builds and inspects the container images of image
// releases on the host (section 1 of the environments spec).
package images

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/nathanaday/hikari/internal/store"
)

// BuildInput is what a build needs: the repo, the commit and scope whose
// files are the build's source, how to build, and the tag.
type BuildInput struct {
	RepoPath string
	Commit   string
	Scope    string
	Spec     store.ImageSpec
	Tag      string
}

// Tag returns the tag of the image of a release (rule IM3).
func Tag(project, system, deliverable, version string) string {
	return fmt.Sprintf("hikari/%s-%s-%s:%s", project, system, deliverable, version)
}

// HostPlatform returns the platform of this machine's Docker containers.
func HostPlatform() string { return "linux/" + runtime.GOARCH }

// Build extracts the scope at the commit and builds the image there (rule
// IM2). It returns the image's tag, id, and platform.
func Build(ctx context.Context, in BuildInput) (store.ImageInfo, error) {
	dir, err := os.MkdirTemp("", "hikari-image-")
	if err != nil {
		return store.ImageInfo{}, err
	}
	defer os.RemoveAll(dir)
	spec := in.Scope
	if spec == "" {
		spec = "."
	}
	archive := exec.CommandContext(ctx, "git", "-C", in.RepoPath, "archive", "--format=tar", in.Commit, "--", spec)
	untar := exec.CommandContext(ctx, "tar", "-x", "-C", dir)
	pipe, err := archive.StdoutPipe()
	if err != nil {
		return store.ImageInfo{}, err
	}
	untar.Stdin = pipe
	var archiveErr bytes.Buffer
	archive.Stderr = &archiveErr
	if err := untar.Start(); err != nil {
		return store.ImageInfo{}, err
	}
	if err := archive.Run(); err != nil {
		return store.ImageInfo{}, fmt.Errorf("git archive %s: %v %s", in.Commit, err, strings.TrimSpace(archiveErr.String()))
	}
	if err := untar.Wait(); err != nil {
		return store.ImageInfo{}, fmt.Errorf("extract the scope at %s: %w", in.Commit, err)
	}
	root := filepath.Join(dir, filepath.FromSlash(in.Scope))
	platform := in.Spec.Platform
	if platform == "" {
		platform = HostPlatform()
	}
	args := []string{"build", "--quiet", "--platform", platform, "-t", in.Tag,
		"-f", filepath.Join(root, filepath.FromSlash(in.Spec.Dockerfile)), filepath.Join(root, filepath.FromSlash(in.Spec.ContextDir()))}
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return store.ImageInfo{}, fmt.Errorf("docker build: %s", lastLines(out.String(), 15))
	}
	id, _, err := Inspect(ctx, in.Tag)
	if err != nil {
		return store.ImageInfo{}, err
	}
	return store.ImageInfo{Tag: in.Tag, ID: id, Platform: platform}, nil
}

// Inspect returns the id and the platform of an image in the host's
// Docker.
func Inspect(ctx context.Context, ref string) (id, platform string, err error) {
	out, err := exec.CommandContext(ctx, "docker", "image", "inspect", "--format", "{{.Id}}\t{{.Os}}/{{.Architecture}}", ref).Output()
	if err != nil {
		return "", "", fmt.Errorf("the image %s is not in this machine's Docker", ref)
	}
	id, platform, _ = strings.Cut(strings.TrimSpace(string(out)), "\t")
	return id, platform, nil
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
