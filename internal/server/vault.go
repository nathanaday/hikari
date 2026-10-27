package server

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/nathanaday/hikari/internal/vault"
)

// maxVaultFile is the largest vault file that the UI can read.
const maxVaultFile = 512 << 10

type releaseDetail struct {
	Manifest vault.Manifest `json:"manifest"`
	// Dir is the release's directory, relative to the vault.
	Dir string `json:"dir"`
	// Files lists the release's files, relative to the vault.
	Files []string `json:"files"`
	// Digests maps each product file, relative to the release, to its
	// SHA-256, from SHA256SUMS.
	Digests map[string]string `json:"digests"`
}

type sharedEntry struct {
	Name string `json:"name"`
	Path string `json:"path"` // relative to the vault
}

// skillEntry is a skill of the vault with the agents it is granted to.
type skillEntry struct {
	sharedEntry
	Holders []string `json:"holders"`
}

// deliverableEntry is a deliverable as the UI shows it (rule U12).
type deliverableEntry struct {
	Name        string         `json:"name"`
	Path        string         `json:"path"`  // its directory, relative to the vault
	Skill       string         `json:"skill"` // its SKILL.md, relative to the vault
	Description string         `json:"description"`
	Layout      vault.Layout   `json:"layout"`
	Targets     []vault.Target `json:"targets"`
	Holders     []string       `json:"holders"`
	Error       string         `json:"error,omitempty"`
}

// sharedListing is the public area and your skills (rule V13).
type sharedListing struct {
	Policies     []sharedEntry      `json:"policies"`
	Skills       []skillEntry       `json:"skills"`
	Deliverables []deliverableEntry `json:"deliverables"`
}

// vaultRoutes serves the vault to the UI: a release, the public area and
// your skills, and single text files (rule U10).
func (s *Server) vaultRoutes(mux *http.ServeMux, ui func(http.HandlerFunc) http.HandlerFunc) {
	st := s.sys.Vault()

	mux.HandleFunc("GET /api/ui/vault/releases/{partition}/{deliverable}/{version}", ui(func(w http.ResponseWriter, r *http.Request) {
		partition, deliverable, version := r.PathValue("partition"), r.PathValue("deliverable"), r.PathValue("version")
		releases, err := st.Releases(partition)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		i := slices.IndexFunc(releases, func(m vault.Manifest) bool { return m.Deliverable == deliverable && m.Version == version })
		if i < 0 {
			writeError(w, http.StatusNotFound, "no release "+deliverable+" "+version+" of "+partition)
			return
		}
		dir := st.ReleaseDir(partition, deliverable, version)
		detail := releaseDetail{Manifest: releases[i], Dir: st.Rel(dir), Files: []string{}, Digests: map[string]string{}}
		filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				rel, _ := filepath.Rel(st.Root, p)
				detail.Files = append(detail.Files, filepath.ToSlash(rel))
			}
			return nil
		})
		sums, _ := os.ReadFile(filepath.Join(dir, vault.SumsFile))
		for _, line := range strings.Split(string(sums), "\n") {
			if sum, path, ok := strings.Cut(line, "  "); ok {
				detail.Digests[path] = sum
			}
		}
		writeJSON(w, http.StatusOK, detail)
	}))

	mux.HandleFunc("GET /api/ui/vault/public", ui(func(w http.ResponseWriter, r *http.Request) {
		listing := sharedListing{
			Policies: sharedFiles(st, st.Public(vault.Policies), ".md"),
			Skills:   []skillEntry{},
		}
		entries, _ := os.ReadDir(st.SkillsRoot())
		for _, e := range entries {
			skill := filepath.Join(st.SkillDir(e.Name()), vault.SkillFile)
			if _, err := os.Stat(skill); err == nil {
				entry := skillEntry{sharedEntry{Name: e.Name(), Path: st.Rel(skill)}, []string{}}
				for _, agent := range s.topo.AgentNames() {
					if slices.Contains(s.topo.Agents[agent].Grants.Skills, e.Name()) {
						entry.Holders = append(entry.Holders, agent)
					}
				}
				listing.Skills = append(listing.Skills, entry)
			}
		}
		listing.Deliverables = s.deliverables(st)
		writeJSON(w, http.StatusOK, listing)
	}))

	mux.HandleFunc("GET /api/ui/vault/file", ui(func(w http.ResponseWriter, r *http.Request) {
		data, code, err := readVaultFile(st.Root, r.URL.Query().Get("path"))
		if err != nil {
			writeError(w, code, err.Error())
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write(data)
	}))
}

// deliverables lists every deliverable in the vault with the agents that
// hold it. An invalid one is listed with its problems.
func (s *Server) deliverables(st vault.Vault) []deliverableEntry {
	out := []deliverableEntry{}
	names, _ := st.DeliverableNames()
	for _, name := range names {
		dir, _ := filepath.Rel(st.Root, st.DeliverableDir(name))
		entry := deliverableEntry{
			Name: name, Path: filepath.ToSlash(dir), Skill: filepath.ToSlash(filepath.Join(dir, vault.SkillFile)),
			Targets: []vault.Target{}, Holders: []string{},
		}
		d, err := st.LoadDeliverable(name)
		if err != nil {
			entry.Error = err.Error()
		} else {
			entry.Description, entry.Layout = d.Description, d.Layout
			if d.Targets != nil {
				entry.Targets = d.Targets
			}
		}
		for _, agent := range s.topo.AgentNames() {
			if slices.Contains(s.topo.Agents[agent].Grants.Deliverables, name) {
				entry.Holders = append(entry.Holders, agent)
			}
		}
		out = append(out, entry)
	}
	return out
}

func sharedFiles(st vault.Vault, dir, ext string) []sharedEntry {
	out := []sharedEntry{}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ext) {
			out = append(out, sharedEntry{Name: strings.TrimSuffix(e.Name(), ext), Path: st.Rel(filepath.Join(dir, e.Name()))})
		}
	}
	return out
}

// readVaultFile reads a text file of the vault. It refuses paths that leave
// the vault, also through links, files over the size limit, and files that
// are not text.
func readVaultFile(root, rel string) ([]byte, int, error) {
	if rel == "" || !filepath.IsLocal(filepath.Clean(rel)) {
		return nil, http.StatusBadRequest, errors.New("the path must be relative to the vault and stay inside it")
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	real, err := filepath.EvalSymlinks(filepath.Join(root, filepath.Clean(rel)))
	if err != nil {
		return nil, http.StatusNotFound, errors.New("no such file in the vault")
	}
	if !strings.HasPrefix(real, realRoot+string(filepath.Separator)) {
		return nil, http.StatusBadRequest, errors.New("the path leaves the vault")
	}
	info, err := os.Stat(real)
	if err != nil || info.IsDir() {
		return nil, http.StatusNotFound, errors.New("no such file in the vault")
	}
	if info.Size() > maxVaultFile {
		return nil, http.StatusRequestEntityTooLarge, errors.New("the file is too large to show")
	}
	data, err := os.ReadFile(real)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	if !utf8.Valid(data) || slices.Contains(data, 0) {
		return nil, http.StatusUnsupportedMediaType, errors.New("the file is not text")
	}
	return data, http.StatusOK, nil
}
