package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestReplay(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("30\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("# warm-up\n21.5\n\n22\n"), 0o644)
	got, err := Replay(dir, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Value != 21.5 || got[2].Value != 30 || got[0].Sensor != "s1" {
		t.Errorf("readings = %+v", got)
	}
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("hot\n"), 0o644)
	if _, err := Replay(dir, "s1"); err == nil {
		t.Error("a line that is no number is refused")
	}
}

func TestSend(t *testing.T) {
	var got Reading
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readings" {
			http.NotFound(w, r)
			return
		}
		json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	if err := Send(srv.Client(), srv.URL, Reading{Sensor: "s1", Value: 42}); err != nil || got.Value != 42 {
		t.Errorf("send = %v, got %+v", err, got)
	}
	if err := Send(srv.Client(), srv.URL+"/nowhere", Reading{}); err == nil {
		t.Error("an error status fails the send")
	}
}
