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

	"github.com/nathanaday/hikari/internal/store"
)

// maxStoreFile is the largest store file that the UI can read.
const maxStoreFile = 512 << 10

type releaseDetail struct {
	Manifest store.Manifest `json:"manifest"`
	// Files lists the release's files, relative to the store.
	Files []string `json:"files"`
	// Digests maps each product file, relative to the release, to its
	// SHA-256, from SHA256SUMS.
	Digests map[string]string `json:"digests"`
}

type sharedEntry struct {
	Name string `json:"name"`
	Path string `json:"path"` // relative to the store
}

// deliverableEntry is a deliverable as the UI shows it (rule U12).
type deliverableEntry struct {
	Name        string         `json:"name"`
	Path        string         `json:"path"`  // its directory, relative to the store
	Skill       string         `json:"skill"` // its SKILL.md, relative to the store
	Description string         `json:"description"`
	Layout      store.Layout   `json:"layout"`
	Targets     []store.Target `json:"targets"`
	Holders     []string       `json:"holders"`
	Error       string         `json:"error,omitempty"`
}

type sharedListing struct {
	Policies     []sharedEntry      `json:"policies"`
	Skills       []sharedEntry      `json:"skills"`
	Agents       []sharedEntry      `json:"agents"`
	Deliverables []deliverableEntry `json:"deliverables"`
}

// storeRoutes serves the store to the UI: a release, the shared area, and
// single text files (rule U10).
func (s *Server) storeRoutes(mux *http.ServeMux, ui func(http.HandlerFunc) http.HandlerFunc) {
	st := s.sys.Store()

	mux.HandleFunc("GET /api/ui/store/releases/{system}/{deliverable}/{version}", ui(func(w http.ResponseWriter, r *http.Request) {
		system, deliverable, version := r.PathValue("system"), r.PathValue("deliverable"), r.PathValue("version")
		releases, err := st.Releases(system)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		i := slices.IndexFunc(releases, func(m store.Manifest) bool { return m.Deliverable == deliverable && m.Version == version })
		if i < 0 {
			writeError(w, http.StatusNotFound, "no release "+deliverable+" "+version+" of "+system)
			return
		}
		detail := releaseDetail{Manifest: releases[i], Files: []string{}, Digests: map[string]string{}}
		dir := st.ReleaseDir(system, deliverable, version)
		filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				rel, _ := filepath.Rel(st.Root, p)
				detail.Files = append(detail.Files, filepath.ToSlash(rel))
			}
			return nil
		})
		sums, _ := os.ReadFile(filepath.Join(dir, store.SumsFile))
		for _, line := range strings.Split(string(sums), "\n") {
			if sum, path, ok := strings.Cut(line, "  "); ok {
				detail.Digests[path] = sum
			}
		}
		writeJSON(w, http.StatusOK, detail)
	}))

	mux.HandleFunc("GET /api/ui/store/shared", ui(func(w http.ResponseWriter, r *http.Request) {
		listing := sharedListing{
			Policies: sharedFiles(st, store.Policies, ".md"),
			Agents:   sharedFiles(st, store.Agents, ".md"),
			Skills:   []sharedEntry{},
		}
		entries, _ := os.ReadDir(st.Shared(store.Skills))
		for _, e := range entries {
			skill := filepath.Join(st.Shared(store.Skills), e.Name(), "SKILL.md")
			if _, err := os.Stat(skill); err == nil {
				rel, _ := filepath.Rel(st.Root, skill)
				listing.Skills = append(listing.Skills, sharedEntry{Name: e.Name(), Path: filepath.ToSlash(rel)})
			}
		}
		listing.Deliverables = s.deliverables(st)
		writeJSON(w, http.StatusOK, listing)
	}))

	mux.HandleFunc("GET /api/ui/store/file", ui(func(w http.ResponseWriter, r *http.Request) {
		data, code, err := readStoreFile(st.Root, r.URL.Query().Get("path"))
		if err != nil {
			writeError(w, code, err.Error())
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write(data)
	}))
}

// deliverables lists every deliverable in the store with the agents that
// hold it. An invalid one is listed with its problems.
func (s *Server) deliverables(st store.Store) []deliverableEntry {
	out := []deliverableEntry{}
	names, _ := st.DeliverableNames()
	for _, name := range names {
		dir, _ := filepath.Rel(st.Root, st.DeliverableDir(name))
		entry := deliverableEntry{
			Name: name, Path: filepath.ToSlash(dir), Skill: filepath.ToSlash(filepath.Join(dir, store.SkillFile)),
			Targets: []store.Target{}, Holders: []string{},
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

func sharedFiles(st store.Store, kind, ext string) []sharedEntry {
	out := []sharedEntry{}
	entries, _ := os.ReadDir(st.Shared(kind))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ext) {
			rel, _ := filepath.Rel(st.Root, filepath.Join(st.Shared(kind), e.Name()))
			out = append(out, sharedEntry{Name: strings.TrimSuffix(e.Name(), ext), Path: filepath.ToSlash(rel)})
		}
	}
	return out
}

// readStoreFile reads a text file of the store. It refuses paths that leave
// the store, also through links, files over the size limit, and files that
// are not text.
func readStoreFile(root, rel string) ([]byte, int, error) {
	if rel == "" || !filepath.IsLocal(filepath.Clean(rel)) {
		return nil, http.StatusBadRequest, errors.New("the path must be relative to the store and stay inside it")
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	real, err := filepath.EvalSymlinks(filepath.Join(root, filepath.Clean(rel)))
	if err != nil {
		return nil, http.StatusNotFound, errors.New("no such file in the store")
	}
	if !strings.HasPrefix(real, realRoot+string(filepath.Separator)) {
		return nil, http.StatusBadRequest, errors.New("the path leaves the store")
	}
	info, err := os.Stat(real)
	if err != nil || info.IsDir() {
		return nil, http.StatusNotFound, errors.New("no such file in the store")
	}
	if info.Size() > maxStoreFile {
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
