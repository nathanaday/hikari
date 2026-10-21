package sysmap

// View is the public view of a map: the map with current health and its
// state. It is what other agents see, and it holds no path into the code.
type View struct {
	Map   `yaml:",inline"`
	State string `yaml:"state" json:"state"`
}

// OwnerView is what the owner sees of its own system: the public view, the
// paths that changed since the described commit, and the problems of the
// map file and of the private documents.
type OwnerView struct {
	View          `yaml:",inline"`
	Changed       []string `yaml:"changed,omitempty" json:"changed,omitempty"`
	Error         string   `yaml:"error,omitempty" json:"error,omitempty"`
	PrivateErrors []string `yaml:"private_errors,omitempty" json:"private_errors,omitempty"`
}

// NewView computes the public view of m. hash returns the current content
// hash of a feature's files.
func NewView(m *Map, priv Private, changed []string, hash func(id string) string) View {
	m = m.Clone()
	for i := range m.Features {
		f := &m.Features[i]
		f.Health = Health(*f, priv.Tested(f.ID), hash(f.ID))
	}
	if m.Features == nil {
		m.Features = []Feature{}
	}
	if m.System.Interfaces.Provides == nil {
		m.System.Interfaces.Provides = []Interface{}
	}
	if m.System.Interfaces.Consumes == nil {
		m.System.Interfaces.Consumes = []Interface{}
	}
	return View{Map: *m, State: State(m, changed)}
}
