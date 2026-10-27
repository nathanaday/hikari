package launcher

import (
	"path"
	"path/filepath"
	"slices"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/vault"
)

// Mount is a bind mount of a host path.
type Mount struct {
	Source   string
	Target   string
	ReadOnly bool
}

// PartitionMounts returns the mounts of a partition agent's code (rules M1 to M3, M5,
// and M7): the scope read-write, each read path read-only, the private
// documents at /hikari/private, an empty directory over the scope's
// .hikari, and an empty directory over .git when the scope is the whole
// repo.
func PartitionMounts(repoPath, scope string, read []string, emptyDir string) []Mount {
	scopeTarget := api.WorkDir(scope)
	mounts := []Mount{{Source: filepath.Join(repoPath, scope), Target: scopeTarget}}
	for _, p := range read {
		mounts = append(mounts, Mount{Source: filepath.Join(repoPath, p), Target: path.Join(api.WorkspaceRoot, filepath.ToSlash(p)), ReadOnly: true})
	}
	mounts = append(mounts,
		Mount{Source: emptyDir, Target: path.Join(scopeTarget, ".hikari"), ReadOnly: true},
		Mount{Source: filepath.Join(repoPath, scope, ".hikari", "private"), Target: api.PrivateDir},
	)
	if scope == "" {
		mounts = append(mounts, Mount{Source: emptyDir, Target: path.Join(api.WorkspaceRoot, ".git"), ReadOnly: true})
	}
	return mounts
}

// VaultMounts returns the vault mounts of an agent (section 3 of the shared
// store spec). Every agent reads the shared area. A partition agent gets its own
// partition directory read-only with its staging area read-write. Each
// visible partition is mounted read-only with its staging area hidden, and
// each other granted partition gets its releases read-only. The public wiki
// is in the shared area (rule A1 of the wikis spec).
func VaultMounts(st vault.Vault, agent string, partitionAgent bool, visible, readable []string, emptyDir string) []Mount {
	mounts := []Mount{{Source: st.SharedDir(), Target: api.SharedDir, ReadOnly: true}}
	if partitionAgent {
		mounts = append(mounts,
			Mount{Source: st.PartitionDir(agent), Target: api.ExportDir, ReadOnly: true},
			Mount{Source: st.Staging(agent), Target: path.Join(api.ExportDir, "staging")},
		)
	}
	for _, sys := range readable {
		if slices.Contains(visible, sys) {
			target := path.Join(api.ReadonlyDir, "systems", sys)
			mounts = append(mounts,
				Mount{Source: st.PartitionDir(sys), Target: target, ReadOnly: true},
				Mount{Source: emptyDir, Target: path.Join(target, "staging"), ReadOnly: true},
			)
		} else {
			mounts = append(mounts, Mount{Source: st.ReleasesDir(sys), Target: api.ReleasesPath(sys), ReadOnly: true})
		}
	}
	return mounts
}
