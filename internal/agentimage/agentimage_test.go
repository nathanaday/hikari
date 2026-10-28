package agentimage

import (
	"strings"
	"testing"
)

// TestStateOf checks CP9 of the plugin spec: the label decides a rebuild.
func TestStateOf(t *testing.T) {
	if StateOf(false, "") != Missing || StateOf(true, "") != Stale || StateOf(true, "old") != Stale || StateOf(true, Sum()) != Current {
		t.Error("CP9 a missing image or one of another Dockerfile is built again")
	}
	if !strings.Contains(string(dockerfile), "USER agent") || !strings.Contains(string(dockerfile), "claude.ai/install.sh") {
		t.Error("the Dockerfile meets the image contract: Claude Code, and a user that is not root")
	}
	if strings.Contains(string(dockerfile), "COPY") || strings.Contains(string(dockerfile), "ADD") {
		t.Error("the Dockerfile builds with no context")
	}
}
