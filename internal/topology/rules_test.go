package topology

import (
	"encoding/json"
	"os"
	"testing"
)

// TestSharedRules runs the cases that the UI's field checks run too (rule
// SV4 of the structure view spec).
func TestSharedRules(t *testing.T) {
	data, err := os.ReadFile("testdata/rules.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases map[string][]struct {
		Value string `json:"value"`
		Valid bool   `json:"valid"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	rules := map[string]func(string) bool{
		"name":    validName.MatchString,
		"branch":  validBranchName,
		"url":     validURL,
		"project": validProjectName,
	}
	for rule, cs := range cases {
		check, ok := rules[rule]
		if !ok {
			t.Errorf("no rule %q", rule)
			continue
		}
		for _, c := range cs {
			if got := check(c.Value); got != c.Valid {
				t.Errorf("%s(%q) = %v, want %v", rule, c.Value, got, c.Valid)
			}
		}
	}
}
