package launcher

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/store"
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

// SystemMounts returns the mounts of an owner's code (rules M1 to M5 and
// M7): the scope read-write, each read path read-only, a volume for each
// container-only path, the private documents at /hikari/private, an empty
// directory over the scope's .hikari, and an empty directory over .git when
// the scope is the whole repo.
func SystemMounts(repoPath, scope string, read, containerOnly []string, emptyDir string) []Mount {
	scopeTarget := api.WorkDir(scope)
	mounts := []Mount{{Source: filepath.Join(repoPath, scope), Target: scopeTarget}}
	for _, p := range read {
		mounts = append(mounts, Mount{Source: filepath.Join(repoPath, p), Target: path.Join(api.WorkspaceRoot, filepath.ToSlash(p)), ReadOnly: true})
	}
	mounts = append(mounts,
		Mount{Source: emptyDir, Target: path.Join(scopeTarget, ".hikari"), ReadOnly: true},
		Mount{Source: filepath.Join(repoPath, scope, ".hikari", "private"), Target: api.PrivateDir},
	)
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

// StoreMounts returns the store mounts of an agent (section 3 of the shared
// store spec). Every agent reads the shared area. An owner gets its own
// system directory read-only with its staging area read-write. Each
// visible system is mounted read-only with its staging area hidden, and
// each other granted system gets its releases read-only.
func StoreMounts(st store.Store, agent string, owner bool, visible, readable []string, emptyDir string) []Mount {
	mounts := []Mount{{Source: st.SharedDir(), Target: api.SharedDir, ReadOnly: true}}
	if owner {
		mounts = append(mounts,
			Mount{Source: st.SystemDir(agent), Target: api.ExportDir, ReadOnly: true},
			Mount{Source: st.Staging(agent), Target: path.Join(api.ExportDir, "staging")},
		)
	}
	for _, sys := range readable {
		if slices.Contains(visible, sys) {
			target := path.Join(api.ReadonlyDir, "systems", sys)
			mounts = append(mounts,
				Mount{Source: st.SystemDir(sys), Target: target, ReadOnly: true},
				Mount{Source: emptyDir, Target: path.Join(target, "staging"), ReadOnly: true},
			)
		} else {
			mounts = append(mounts, Mount{Source: st.ReleasesDir(sys), Target: api.ReleasesPath(sys), ReadOnly: true})
		}
	}
	return mounts
}

// VolumeName names the volume of a container-only path. The name stays the
// same between runs, so installed dependencies persist.
func VolumeName(repoPath, scope, p string) string {
	sum := sha256.Sum256([]byte(repoPath + "\x00" + scope + "\x00" + p))
	return "hikari-cache-" + hex.EncodeToString(sum[:])[:12]
}
