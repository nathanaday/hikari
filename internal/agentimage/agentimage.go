// Package agentimage builds hikari-agent, the reference image that agents run
// in or build from. The binary embeds its Dockerfile, so a machine without a
// checkout of hikari can build it, and labels the image with the Dockerfile's
// sha256, so that hikari up builds it again only when the Dockerfile changed.
// The image holds no hikari binary: hikari mounts its own at run time.
package agentimage

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

//go:embed Dockerfile
var dockerfile []byte

// Name is the image's name. Topology files and partition Dockerfiles name it.
const Name = "hikari-agent"

// Label holds the sha256 of the Dockerfile that built the image.
const Label = "hikari.dockerfile"

// Sum is the sha256 of the embedded Dockerfile.
func Sum() string {
	h := sha256.Sum256(dockerfile)
	return hex.EncodeToString(h[:])
}

// State is how the image on this machine compares with the embedded
// Dockerfile.
type State string

const (
	Missing State = "missing"
	Stale   State = "stale"
	Current State = "current"
)

// StateOf decides the state from the label of the image on this machine;
// found is false when there is no image.
func StateOf(found bool, label string) State {
	switch {
	case !found:
		return Missing
	case label != Sum():
		return Stale
	}
	return Current
}

// Check reads the image on this machine.
func Check(ctx context.Context) (State, error) {
	out, err := exec.CommandContext(ctx, "docker", "image", "inspect", "--format", `{{index .Config.Labels "`+Label+`"}}`, Name).CombinedOutput()
	if err != nil {
		if strings.Contains(strings.ToLower(string(out)), "no such image") {
			return Missing, nil
		}
		return "", fmt.Errorf("docker image inspect %s: %s", Name, strings.TrimSpace(string(out)))
	}
	return StateOf(true, strings.TrimSpace(string(out))), nil
}

// Build builds the image from the embedded Dockerfile, with no build
// context, and writes docker's output to out.
func Build(ctx context.Context, out io.Writer) error {
	cmd := exec.CommandContext(ctx, "docker", "build", "--tag", Name, "--label", Label+"="+Sum(), "-")
	cmd.Stdin = bytes.NewReader(dockerfile)
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker build %s: %w", Name, err)
	}
	return nil
}

// Ensure builds the image when it is missing or stale, and reports the
// state it found.
func Ensure(ctx context.Context, out io.Writer) (State, error) {
	s, err := Check(ctx)
	if err != nil || s == Current {
		return s, err
	}
	return s, Build(ctx, out)
}
