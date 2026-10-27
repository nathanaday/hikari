package launcher

import (
	"path"
	"path/filepath"
	"slices"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/topology"
	"github.com/nathanaday/hikari/internal/workspace"
)

// Mount is a bind mount of a host path.
type Mount struct {
	Source   string
	Target   string
	ReadOnly bool
}

// PartitionMounts returns the mounts of a partition agent's workspace
// (rules F4 and M7): each folder at /workspace/<place>, writable or
// read-only, an empty directory over the .git of a folder that is a whole
// repo and over the .hikari of each writable folder, and the first
// writable folder's private documents at /hikari/private.
func PartitionMounts(ws workspace.Workspace, emptyDir string) []Mount {
	var mounts []Mount
	for _, f := range ws {
		target := api.WorkspacePath(f.Place)
		mounts = append(mounts, Mount{Source: f.Host, Target: target, ReadOnly: f.ReadOnly})
		if f.Path == "" {
			mounts = append(mounts, Mount{Source: emptyDir, Target: path.Join(target, ".git"), ReadOnly: true})
		}
		if !f.ReadOnly {
			mounts = append(mounts, Mount{Source: emptyDir, Target: path.Join(target, ".hikari"), ReadOnly: true})
		}
	}
	if home := ws.Home(); home.Host != "" {
		mounts = append(mounts, Mount{Source: filepath.Join(home.Host, ".hikari", "private"), Target: api.PrivateDir})
	}
	return mounts
}

// VaultMounts returns the vault mounts of an agent (rule V3 of the vault
// layout spec). Each mount is one directory, whose place in the vault
// decides its mode: the public area and the agent's read-only part are
// read-only, and its can-edit part and staging area are writable. The
// orchestration agent reads every partition's read-only part, and a
// partition agent reads the releases of each partition it is granted.
func VaultMounts(t *topology.Topology, agent string) []Mount {
	st := t.VaultOf()
	mounts := []Mount{{Source: st.PublicRoot(), Target: api.PublicDir, ReadOnly: true}}
	if t.Agents[agent].Partition != nil {
		mounts = append(mounts,
			Mount{Source: st.ReadOnly(agent), Target: api.SelfDir, ReadOnly: true},
			Mount{Source: st.CanEdit(agent), Target: api.CanEditDir},
			Mount{Source: st.Staging(agent), Target: api.StagingDir},
		)
	} else if agent == t.Orchestration {
		mounts = append(mounts,
			Mount{Source: st.OrchestrationReadOnly(), Target: api.SelfDir, ReadOnly: true},
			Mount{Source: st.OrchestrationCanEdit(), Target: api.CanEditDir},
		)
	}
	visible := t.VisiblePartitions(agent)
	for _, p := range visible {
		mounts = append(mounts, Mount{Source: st.ReadOnly(p), Target: api.PartitionPath(p), ReadOnly: true})
	}
	for _, p := range t.ReadableReleases(agent) {
		if !slices.Contains(visible, p) {
			mounts = append(mounts, Mount{Source: st.ReleasesOf(p), Target: api.ReleasesPath(p), ReadOnly: true})
		}
	}
	for _, sk := range t.Agents[agent].Grants.Skills {
		mounts = append(mounts, Mount{Source: st.SkillDir(sk), Target: api.SkillPath(sk), ReadOnly: true})
	}
	return mounts
}
