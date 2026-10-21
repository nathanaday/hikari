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
}

type sharedEntry struct {
	Name string `json:"name"`
	Path string `json:"path"` // relative to the store
}

type sharedListing struct {
	Policies []sharedEntry `json:"policies"`
	Skills   []sharedEntry `json:"skills"`
	Agents   []sharedEntry `json:"agents"`
}

// storeRoutes serves the store to the UI: a release, the shared area, and
// single text files (rule U10).
func (s *Server) storeRoutes(mux *http.ServeMux, ui func(http.HandlerFunc) http.HandlerFunc) {
	st := s.sys.Store()

	mux.HandleFunc("GET /api/ui/store/releases/{system}/{version}", ui(func(w http.ResponseWriter, r *http.Request) {
		system, version := r.PathValue("system"), r.PathValue("version")
		releases, err := st.Releases(system)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		i := slices.IndexFunc(releases, func(m store.Manifest) bool { return m.Version == version })
		if i < 0 {
			writeError(w, http.StatusNotFound, "no release "+version+" of "+system)
			return
		}
		detail := releaseDetail{Manifest: releases[i], Files: []string{}}
		dir := st.ReleaseDir(system, version)
		filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				rel, _ := filepath.Rel(st.Root, p)
				detail.Files = append(detail.Files, filepath.ToSlash(rel))
			}
			return nil
		})
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
