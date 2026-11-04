package vocab

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vocabulary", "terms.yaml")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("V1: the file must not exist before the first change")
	}
	_, v0 := s.Get()

	if err := s.DefineTerm(Term{Term: "order", Definition: "A request to buy.", Avoid: []string{"purchase"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.DefineInterface(Interface{Name: "POST /orders", Kind: "http", Description: "Creates an order."}); err != nil {
		t.Fatal(err)
	}
	if err := s.DefineTerm(Term{Term: "order", Definition: "A customer's request to buy items."}); err != nil {
		t.Fatal(err)
	}
	v, v1 := s.Get()
	if len(v.Terms) != 1 || v.Terms[0].Definition != "A customer's request to buy items." {
		t.Errorf("V4: defining an existing term must replace it: %+v", v.Terms)
	}
	if v1 != v0+3 {
		t.Errorf("V5: version went from %d to %d after three changes", v0, v1)
	}

	data, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(data), "# The vocabulary of the project.") || !strings.Contains(string(data), "name: POST /orders") {
		t.Errorf("file:\n%s", data)
	}
	again, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := again.Get(); len(got.Terms) != 1 || len(got.Interfaces) != 1 {
		t.Errorf("reloaded vocabulary: %+v", got)
	}

	if err := s.Remove("POST /orders"); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove("ghost"); err == nil {
		t.Error("removing an unknown entry must fail")
	}
	bad := filepath.Join(t.TempDir(), "terms.yaml")
	os.WriteFile(bad, []byte("version: 1\nowner: shop\n"), 0o644)
	if _, err := Open(bad); err == nil {
		t.Error("an unknown key is an error")
	}
}

func TestDefineRejectsIncompleteEntries(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "terms.yaml"))
	for _, err := range []error{
		s.DefineTerm(Term{Term: "order"}),
		s.DefineInterface(Interface{Name: "x", Kind: "http"}),
		s.DefineInterface(Interface{Name: "x", Kind: "HTTP API", Description: "d"}),
	} {
		if err == nil {
			t.Error("an incomplete entry was accepted")
		}
	}
	if _, version := s.Get(); version != 1 {
		t.Errorf("a rejected change bumped the version to %d", version)
	}
}

func TestCheck(t *testing.T) {
	known := map[string]bool{"POST /orders": true, "inventory.reserve": true}
	provided := map[string]bool{"POST /orders": true}
	missing := map[string]bool{"inventory.reserve": true}
	flags := Check([]string{"POST /orders", "order.created"}, []string{"inventory.reserve", "order.created"}, known, provided, missing)
	want := []Flag{
		{"order.created", NotInVocabulary},
		{"inventory.reserve", NoProvider},
		{"order.created", NoProvider},
		{"inventory.reserve", ContractMissing},
	}
	if len(flags) != len(want) {
		t.Fatalf("flags = %+v", flags)
	}
	for i := range want {
		if flags[i] != want[i] {
			t.Errorf("flag %d = %+v, want %+v", i, flags[i], want[i])
		}
	}
}

func TestContracts(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "terms.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"/etc/passwd", "../x.yaml", "a/../../b", "dir/"} {
		if err := s.DefineInterface(Interface{Name: "x", Kind: "http", Description: "d", Contract: bad}); err == nil {
			t.Errorf("contract %q is accepted", bad)
		}
	}
	if err := s.DefineInterface(Interface{Name: "POST /orders", Kind: "http", Description: "d", Contract: " orders/openapi.yaml "}); err != nil {
		t.Fatal(err)
	}
	if err := s.DefineInterface(Interface{Name: "order.created", Kind: "event", Description: "d", Contract: "events/order-created.json"}); err != nil {
		t.Fatal(err)
	}
	if err := s.DefineInterface(Interface{Name: "Stock.reserve", Kind: "function", Description: "d"}); err != nil {
		t.Fatal(err)
	}
	v, _ := s.Get()
	if v.Interfaces[0].Contract != "orders/openapi.yaml" {
		t.Errorf("contract = %q", v.Interfaces[0].Contract)
	}
	missing := MissingContracts(v, func(c string) bool { return c == "orders/openapi.yaml" })
	if len(missing) != 1 || !missing["order.created"] {
		t.Errorf("missing = %v", missing)
	}
}
