package launcher

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"path/filepath"
	"strings"

	"github.com/nathanaday/hikari/internal/api"
)

// Mount is a bind mount of a host path, or a named volume when Volume is
// set.
type Mount struct {
	Source   string
	Target   string
	ReadOnly bool
	Volume   bool
}

func (m Mount) flag() string {
	kind := "bind"
	if m.Volume {
		kind = "volume"
	}
	fields := []string{"type=" + kind, csvField("source=" + m.Source), csvField("target=" + m.Target)}
	if m.ReadOnly {
		fields = append(fields, "readonly")
	}
	return strings.Join(fields, ",")
}

// csvField quotes a --mount field that holds a comma or a quote.
func csvField(f string) string {
	if !strings.ContainsAny(f, `,"`) {
		return f
	}
	return `"` + strings.ReplaceAll(f, `"`, `""`) + `"`
}

// SystemMounts returns the mounts of an owner's container (rules M1 to M5):
// the scope read-write, each read path and the map directory read-only, a
// volume for each container-only path, and an empty directory over .git when
// the scope is the whole repo.
func SystemMounts(repoPath, scope string, read, containerOnly []string, emptyDir string) []Mount {
	scopeTarget := api.WorkDir(scope)
	mounts := []Mount{{Source: filepath.Join(repoPath, scope), Target: scopeTarget}}
	for _, p := range read {
		mounts = append(mounts, Mount{Source: filepath.Join(repoPath, p), Target: path.Join(api.WorkspaceRoot, filepath.ToSlash(p)), ReadOnly: true})
	}
	mounts = append(mounts, Mount{
		Source:   filepath.Join(repoPath, scope, ".hikari"),
		Target:   path.Join(scopeTarget, ".hikari"),
		ReadOnly: true,
	})
	for _, p := range containerOnly {
		mounts = append(mounts, Mount{
			Source: VolumeName(repoPath, scope, p),
			Target: path.Join(scopeTarget, filepath.ToSlash(p)),
			Volume: true,
		})
	}
	if scope == "" {
		mounts = append(mounts, Mount{Source: emptyDir, Target: path.Join(api.WorkspaceRoot, ".git"), ReadOnly: true})
	}
	return mounts
}

// VolumeName names the volume of a container-only path. The name stays the
// same between runs, so installed dependencies persist.
func VolumeName(repoPath, scope, p string) string {
	sum := sha256.Sum256([]byte(repoPath + "\x00" + scope + "\x00" + p))
	return "hikari-cache-" + hex.EncodeToString(sum[:])[:12]
}
