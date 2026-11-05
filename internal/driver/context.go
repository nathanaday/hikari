package driver

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/nathanaday/hikari/internal/api"
	"github.com/nathanaday/hikari/internal/modules"
	"github.com/nathanaday/hikari/internal/vault"
)

// Context is what an agent's container puts before Claude Code on each
// turn, built on the host for the UI (the agent context view): the system
// prompt as the driver writes it, with the policies and the granted styles
// read from the vault, and the names of what shapes it.
type Context struct {
	Prompt   string   `json:"prompt"`
	Pack     []string `json:"pack"`
	Skills   []string `json:"skills"`
	Styles   []string `json:"styles"`
	Policies []string `json:"policies"`
	Profile  string   `json:"profile,omitempty"`
	Model    string   `json:"model,omitempty"`
	Effort   string   `json:"effort,omitempty"`
	Review   string   `json:"review,omitempty"`
	// Missing names the granted styles whose file the vault lacks.
	Missing []string `json:"missing,omitempty"`
}

// BuildContext builds the context of self from the vault at root. It reads
// the same files as the container, from the host's side of the mounts.
func BuildContext(self api.Self, root string) Context {
	st := vault.Vault{Root: root}
	shared := &sharedArea{public: st.PublicRoot(), styles: st.StylesRoot(), grantedStyles: self.Styles, warned: map[string]bool{}}
	c := Context{
		Prompt:   systemPrompt(self) + shared.policies() + shared.stylesSection(),
		Pack:     nonNil(modules.Pack(packRole(self), self.Modules)),
		Skills:   nonNil(self.Skills),
		Styles:   nonNil(self.Styles),
		Policies: []string{},
		Profile:  self.Profile, Model: self.Model, Effort: self.Effort, Review: self.Review,
	}
	for _, name := range shared.files(vault.Policies, ".md") {
		c.Policies = append(c.Policies, strings.TrimSuffix(name, ".md"))
	}
	for _, name := range self.Styles {
		if _, err := os.Stat(filepath.Join(st.StylesRoot(), name+".md")); err != nil {
			c.Missing = append(c.Missing, name)
		}
	}
	return c
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
