package forge

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var (
	tokenMu    sync.Mutex
	tokenCache = map[string]string{}
	// ghToken runs gh auth token; tests replace it.
	ghToken = func(host string) string {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "gh", "auth", "token", "-h", host).Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
)

// TokenFor returns the GitHub token of host: GITHUB_TOKEN, else GH_TOKEN,
// else what gh auth token prints, else "" (rule FG4). The gh answer is
// cached for the run.
func TokenFor(host string) string {
	for _, v := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if t := strings.TrimSpace(os.Getenv(v)); t != "" {
			return t
		}
	}
	tokenMu.Lock()
	defer tokenMu.Unlock()
	if t, ok := tokenCache[host]; ok {
		return t
	}
	t := ghToken(host)
	tokenCache[host] = t
	return t
}
