package topoedit

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nathanaday/hikari/internal/topology"
)

// Op is one operation of section 4 of the topology API spec. Ids name the
// entities, as in the file. A pointer field that is absent leaves its key,
// "" removes it, and any other value sets it (rule TE2).
type Op struct {
	Op       string `json:"op"`
	ID       string `json:"id,omitempty"`
	NewID    string `json:"new_id,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Hub      string `json:"hub,omitempty"`
	Agent    string `json:"agent,omitempty"`
	Template string `json:"template,omitempty"`

	Name         *string `json:"name,omitempty"`
	Description  *string `json:"description,omitempty"`
	Instructions *string `json:"instructions,omitempty"`
	Model        *string `json:"model,omitempty"`
	Tests        *string `json:"tests,omitempty"`
	Orchestrator *string `json:"orchestrator,omitempty"`
	URL          *string `json:"url,omitempty"`
	Path         *string `json:"path,omitempty"`
	Branch       *string `json:"branch,omitempty"`
	Store        *string `json:"store,omitempty"`
	Repo         *string `json:"repo,omitempty"`
	Scope        *string `json:"scope,omitempty"`
	Test         *string `json:"test,omitempty"`
	CPUs         *string `json:"cpus,omitempty"`
	Memory       *string `json:"memory,omitempty"`
	Serves       *string `json:"serves,omitempty"`
	Image        *string `json:"image,omitempty"`

	Members      *[]string `json:"members,omitempty"`
	Hubs         *[]string `json:"hubs,omitempty"`
	Read         *[]string `json:"read,omitempty"`
	Releases     *[]string `json:"releases,omitempty"`
	Deliverables *[]string `json:"deliverables,omitempty"`
}

// Rename is a change of the name of an agent or a hub, which a plan can
// propagate (section 5).
type Rename struct {
	Kind string `json:"kind"`
	From string `json:"from"`
	To   string `json:"to"`
}

// Result is what a list of operations does.
type Result struct {
	Text    []byte
	Effects []string
	Renames []Rename
	Summary []string
}

// OpError is the refusal of one operation of a list (rule TE7).
type OpError struct {
	Index int
	Op    string
	Err   error
}

func (e *OpError) Error() string { return fmt.Sprintf("operation %d (%s): %v", e.Index+1, e.Op, e.Err) }
func (e *OpError) Unwrap() error { return e.Err }

// ErrNotFound is the refusal of an operation on an entity that the file
// does not have.
var ErrNotFound = errors.New("not found")

var validID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// Apply applies ops to text in order, as a whole. dir is the directory of
// the compose file.
func Apply(text []byte, ops []Op, dir string) (*Result, error) {
	e := &editor{text: text, dir: dir}
	for i, op := range ops {
		e.model = ReadModel(e.text, dir)
		if e.model == nil {
			return nil, &OpError{i, op.Op, errors.New("the file is not valid YAML; fix it in the YAML view")}
		}
		if err := e.apply(op); err != nil {
			return nil, &OpError{i, op.Op, err}
		}
	}
	return &Result{Text: e.text, Effects: e.effects, Renames: e.renames, Summary: e.summary}, nil
}

type editor struct {
	text    []byte
	dir     string
	model   *Model
	effects []string
	renames []Rename
	summary []string
}

func (e *editor) edit(fn func(d *doc) ([]byte, error)) error {
	d, err := parse(e.text)
	if err != nil {
		return err
	}
	out, err := fn(d)
	if err != nil {
		return err
	}
	e.text = out
	return nil
}

func (e *editor) setValue(p string, key string, v *yaml.Node, first bool) error {
	return e.edit(func(d *doc) ([]byte, error) { return d.setValue(parsePath(p), key, v, first) })
}

func (e *editor) deleteKey(p, key string) error {
	return e.edit(func(d *doc) ([]byte, error) { return d.deleteKey(parsePath(p), key) })
}

// setField applies a pointer field to key of the mapping at p (rule TE2).
func (e *editor) setField(p, key string, v *string, escape bool) error {
	switch {
	case v == nil:
		return nil
	case *v == "":
		return e.deleteKey(p, key)
	case escape:
		return e.setValue(p, key, scalar(escapeText(*v)), false)
	}
	return e.setValue(p, key, scalar(*v), false)
}

func (e *editor) setList(p, key string, v *[]string) error {
	switch {
	case v == nil:
		return nil
	case len(*v) == 0:
		return e.deleteKey(p, key)
	}
	return e.setValue(p, key, list(*v), false)
}

// ensure makes the mapping key of the mapping at p when it is missing.
func (e *editor) ensure(p, key string) error {
	d, err := parse(e.text)
	if err != nil {
		return err
	}
	if v, ok := d.find(append(parsePath(p), key)); ok && v.value.Kind == yaml.MappingNode {
		return nil
	}
	return e.setValue(p, key, &yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle}, false)
}

func (e *editor) agent(id string) (*Agent, error) {
	for i := range e.model.Agents {
		if e.model.Agents[i].ID == id {
			return &e.model.Agents[i], nil
		}
	}
	return nil, fmt.Errorf("%w: no agent %q", ErrNotFound, id)
}

// agentName returns the name of the agent whose id is id, for the summary
// that becomes a commit message.
func (e *editor) agentName(id string) string {
	if a, err := e.agent(id); err == nil {
		return a.Name
	}
	return id
}

func (e *editor) hub(id string) (*Hub, error) {
	for i := range e.model.Hubs {
		if e.model.Hubs[i].ID == id {
			return &e.model.Hubs[i], nil
		}
	}
	return nil, fmt.Errorf("%w: no hub %q", ErrNotFound, id)
}

func (e *editor) repo(id string) (*Repo, error) {
	for i := range e.model.Repos {
		if e.model.Repos[i].ID == id {
			return &e.model.Repos[i], nil
		}
	}
	return nil, fmt.Errorf("%w: no repo %q", ErrNotFound, id)
}

func (e *editor) sidecar(id string) (*Sidecar, error) {
	for i := range e.model.Sidecars {
		if e.model.Sidecars[i].ID == id {
			return &e.model.Sidecars[i], nil
		}
	}
	return nil, fmt.Errorf("%w: no sidecar %q", ErrNotFound, id)
}

// nodeTaken reports whether id is the id or the name of an agent, a hub, or
// a service, or is reserved.
func (e *editor) nodeTaken(id string) bool {
	if id == topology.User {
		return true
	}
	for _, a := range e.model.Agents {
		if a.ID == id || a.Name == id {
			return true
		}
	}
	for _, h := range e.model.Hubs {
		if h.ID == id || h.Name == id {
			return true
		}
	}
	for _, s := range e.model.Sidecars {
		if s.ID == id {
			return true
		}
	}
	return false
}

func checkID(id string) error {
	if !validID.MatchString(id) {
		return fmt.Errorf("%q is not a valid id: use a lowercase letter, then lowercase letters, digits, and dashes, up to 32 characters", id)
	}
	return nil
}

func (e *editor) store() string {
	return storeDir(e.model.Project.Store, e.dir)
}

// leaves records that a removal leaves a store directory in place (rule
// TE6).
func (e *editor) leaves(rel string) {
	if store := e.store(); store != "" && exists(filepath.Join(store, rel)) {
		e.effects = append(e.effects, fmt.Sprintf("leaves %s in the store; delete it in a commit of your own when you no longer need it", filepath.ToSlash(filepath.Join(filepath.Base(store), rel))))
	}
}

func (e *editor) apply(op Op) error {
	switch op.Op {
	case "set_project":
		return e.setProject(op)
	case "enable_project":
		if e.model.Project.Repo {
			return errors.New("the project repository is already on")
		}
		branch := "${HIKARI_BRANCH:-hikari}"
		if op.Branch != nil && *op.Branch != "" {
			branch = *op.Branch
		}
		e.summary = append(e.summary, "turn the project repository on")
		return e.setValue("x-hikari", "project", mapping("branch", branch), true)
	case "disable_project":
		if !e.model.Project.Repo {
			return errors.New("the project repository is already off")
		}
		for _, a := range e.model.Agents {
			if a.System != nil && a.System.Repo == topology.ProjectRepo {
				return fmt.Errorf("the system of %q is in the project repository", a.ID)
			}
		}
		for _, r := range e.model.Repos {
			if r.URL != "" {
				return fmt.Errorf("the repo %q has a url, and its clone lives in the project repository", r.ID)
			}
		}
		e.summary = append(e.summary, "turn the project repository off")
		return e.deleteKey("x-hikari", "project")
	case "add_repo":
		return e.addRepo(op)
	case "set_repo":
		r, err := e.repo(op.ID)
		if err != nil {
			return err
		}
		p := "x-hikari.repos." + r.ID
		for _, f := range []struct {
			key string
			v   *string
		}{{"url", op.URL}, {"path", op.Path}, {"branch", op.Branch}} {
			if err := e.setField(p, f.key, f.v, false); err != nil {
				return err
			}
		}
		if op.URL != nil && *op.URL != r.URL && *op.URL != "" {
			e.effects = append(e.effects, fmt.Sprintf("the repo %s has a new url; run hikari sync to clone it", r.ID))
		}
		e.summary = append(e.summary, "change repo "+r.Name)
		return nil
	case "remove_repo":
		r, err := e.repo(op.ID)
		if err != nil {
			return err
		}
		for _, a := range e.model.Agents {
			if a.System != nil && a.System.Repo == r.ID {
				return fmt.Errorf("the system of %q is in the repo %q", a.ID, r.ID)
			}
		}
		if r.URL != "" && exists(filepath.Join(e.dir, topology.CloneDir, r.ID)) {
			e.effects = append(e.effects, fmt.Sprintf("leaves the clone %s/%s, which git ignores", topology.CloneDir, r.ID))
		}
		e.summary = append(e.summary, "remove repo "+r.Name)
		return e.deleteKey("x-hikari.repos", r.ID)
	case "add_hub":
		return e.addHub(op)
	case "set_hub":
		h, err := e.hub(op.ID)
		if err != nil {
			return err
		}
		p := "x-hikari.hubs." + h.ID
		if err := e.setField(p, "description", op.Description, true); err != nil {
			return err
		}
		if op.Orchestrator != nil && *op.Orchestrator != "" {
			if err := e.setValue(p, "orchestrator", scalar(*op.Orchestrator), false); err != nil {
				return err
			}
		}
		e.summary = append(e.summary, "change #"+h.Name)
		return nil
	case "add_member":
		h, err := e.hub(op.Hub)
		if err != nil {
			return err
		}
		if h.Orchestrator == op.Agent || slices.Contains(h.Members, op.Agent) {
			return fmt.Errorf("%q is already in #%s", op.Agent, h.ID)
		}
		e.summary = append(e.summary, fmt.Sprintf("add %s to #%s", e.agentName(op.Agent), h.Name))
		return e.edit(func(d *doc) ([]byte, error) {
			return d.appendItem(parsePath("x-hikari.hubs."+h.ID+".members"), scalar(op.Agent))
		})
	case "remove_member":
		h, err := e.hub(op.Hub)
		if err != nil {
			return err
		}
		i := slices.Index(h.Members, op.Agent)
		if i < 0 {
			return fmt.Errorf("%q is not a member of #%s", op.Agent, h.ID)
		}
		if a, err := e.agent(op.Agent); err == nil && a.Tests == h.ID {
			return fmt.Errorf("%q tests #%s, so it must stay a member; change its tests first", op.Agent, h.ID)
		}
		e.summary = append(e.summary, fmt.Sprintf("remove %s from #%s", e.agentName(op.Agent), h.Name))
		return e.edit(func(d *doc) ([]byte, error) {
			return d.removeItem(parsePath("x-hikari.hubs."+h.ID+".members"), i)
		})
	case "remove_hub":
		return e.removeHub(op)
	case "add_agent":
		return e.addAgent(op)
	case "set_agent":
		a, err := e.agent(op.ID)
		if err != nil {
			return err
		}
		if err := e.ensure("services."+a.ID, "x-hikari"); err != nil {
			return err
		}
		p := "services." + a.ID + ".x-hikari"
		for _, f := range []struct {
			key    string
			v      *string
			escape bool
		}{{"description", op.Description, true}, {"instructions", op.Instructions, true}, {"model", op.Model, false}, {"tests", op.Tests, false}} {
			if err := e.setField(p, f.key, f.v, f.escape); err != nil {
				return err
			}
		}
		e.summary = append(e.summary, "change "+a.Name)
		return nil
	case "set_system":
		return e.setSystem(op)
	case "clear_system":
		a, err := e.agent(op.ID)
		if err != nil {
			return err
		}
		if a.System == nil {
			return fmt.Errorf("%q owns no system", a.ID)
		}
		e.leaves("systems/" + a.ID)
		e.summary = append(e.summary, "clear the system of "+a.Name)
		return e.deleteKey("services."+a.ID+".x-hikari", "system")
	case "set_grants":
		a, err := e.agent(op.ID)
		if err != nil {
			return err
		}
		if err := e.ensure("services."+a.ID, "x-hikari"); err != nil {
			return err
		}
		if err := e.ensure("services."+a.ID+".x-hikari", "grants"); err != nil {
			return err
		}
		p := "services." + a.ID + ".x-hikari.grants"
		if err := e.setList(p, "releases", op.Releases); err != nil {
			return err
		}
		if err := e.setList(p, "deliverables", op.Deliverables); err != nil {
			return err
		}
		if m := ReadModel(e.text, e.dir); m != nil {
			for _, x := range m.Agents {
				if x.ID == a.ID && len(x.Releases) == 0 && len(x.Deliverables) == 0 {
					if err := e.deleteKey("services."+a.ID+".x-hikari", "grants"); err != nil {
						return err
					}
				}
			}
		}
		e.summary = append(e.summary, "change the grants of "+a.Name)
		return nil
	case "set_e2e":
		a, err := e.agent(op.ID)
		if err != nil {
			return err
		}
		p := "services." + a.ID + ".x-hikari"
		cpus, memory := deref(op.CPUs), deref(op.Memory)
		if cpus == "" && memory == "" {
			return e.deleteKey(p, "e2e")
		}
		kv := []any{}
		if cpus != "" {
			kv = append(kv, "cpus", cpus)
		}
		if memory != "" {
			kv = append(kv, "memory", memory)
		}
		e.summary = append(e.summary, "change the e2e limits of "+a.Name)
		return e.setValue(p, "e2e", mapping(kv...), false)
	case "remove_agent":
		return e.removeAgent(op)
	case "add_sidecar":
		if err := checkID(op.ID); err != nil {
			return err
		}
		if e.nodeTaken(op.ID) {
			return fmt.Errorf("the id %q is taken", op.ID)
		}
		image := deref(op.Image)
		if image == "" {
			return errors.New("a sidecar needs an image")
		}
		e.summary = append(e.summary, fmt.Sprintf("add sidecar %s of %s", op.ID, e.agentName(deref(op.Serves))))
		return e.addService(op.ID, "    image: "+mustRender(scalar(image))+"\n    x-hikari: "+mustRender(mapping("serves", deref(op.Serves)))+"\n")
	case "set_serves":
		s, err := e.sidecar(op.ID)
		if err != nil {
			return err
		}
		e.summary = append(e.summary, fmt.Sprintf("sidecar %s serves %s", s.ID, e.agentName(op.Agent)))
		return e.setValue("services."+s.ID+".x-hikari", "serves", scalar(op.Agent), false)
	case "remove_sidecar":
		s, err := e.sidecar(op.ID)
		if err != nil {
			return err
		}
		e.summary = append(e.summary, "remove sidecar "+s.ID)
		return e.deleteKey("services", s.ID)
	case "rename":
		return e.rename(op)
	case "change_id":
		return e.changeID(op)
	}
	return fmt.Errorf("unknown operation %q", op.Op)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func mustRender(n *yaml.Node) string {
	s, err := render(n)
	if err != nil {
		panic(err)
	}
	return s
}

func (e *editor) setProject(op Op) error {
	if op.Name != nil {
		name := strings.TrimSpace(*op.Name)
		if name == "" || name == e.model.Project.ID {
			if err := e.deleteKey("x-hikari", "name"); err != nil {
				return err
			}
		} else if err := e.setValue("x-hikari", "name", scalar(name), true); err != nil {
			return err
		}
		e.summary = append(e.summary, "name the project "+strconv.Quote(nameOr(name, e.model.Project.ID)))
	}
	if op.Branch != nil {
		if !e.model.Project.Repo {
			return errors.New("the project branch needs the project repository")
		}
		if *op.Branch == "" {
			return errors.New("the project repository needs a branch")
		}
		if err := e.setValue("x-hikari.project", "branch", scalar(*op.Branch), false); err != nil {
			return err
		}
		e.summary = append(e.summary, "set the project branch to "+*op.Branch)
	}
	if op.Store != nil {
		if err := e.setField("x-hikari", "store", op.Store, false); err != nil {
			return err
		}
		e.effects = append(e.effects, "the store moves to a new directory; hikari makes it empty at the next start, and the old one stays")
		e.summary = append(e.summary, "move the store")
	}
	return nil
}

func (e *editor) addRepo(op Op) error {
	if err := checkID(op.ID); err != nil {
		return err
	}
	if op.ID == topology.ProjectRepo {
		return fmt.Errorf("the repo id %q is reserved for the project repository", op.ID)
	}
	for _, r := range e.model.Repos {
		if r.ID == op.ID || r.Name == op.ID {
			return fmt.Errorf("the repo id %q is taken", op.ID)
		}
	}
	url, local := deref(op.URL), deref(op.Path)
	if (url == "") == (local == "") {
		return errors.New("a repo has a url or a path, and not both")
	}
	kv := []any{}
	if url != "" {
		kv = append(kv, "url", url)
		e.effects = append(e.effects, fmt.Sprintf("run hikari sync to clone %s into %s/%s", op.ID, topology.CloneDir, op.ID))
	} else {
		kv = append(kv, "path", local)
	}
	if b := deref(op.Branch); b != "" {
		kv = append(kv, "branch", b)
	}
	e.summary = append(e.summary, "add repo "+op.ID)
	if e.model == nil || len(e.model.Repos) == 0 {
		// The repos go before the hubs, as the README writes them.
		empty := &yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle}
		if err := e.edit(func(d *doc) ([]byte, error) {
			if _, ok := d.find(path{"x-hikari", "repos"}); ok {
				return d.text, nil
			}
			return d.setValueBefore(path{"x-hikari"}, "repos", empty, "hubs")
		}); err != nil {
			return err
		}
	}
	return e.setValue("x-hikari.repos", op.ID, mapping(kv...), false)
}

func (e *editor) addHub(op Op) error {
	if err := checkID(op.ID); err != nil {
		return err
	}
	if e.nodeTaken(op.ID) {
		return fmt.Errorf("the id %q is taken", op.ID)
	}
	orch := deref(op.Orchestrator)
	if orch == "" {
		return errors.New("a hub needs an orchestrator")
	}
	kv := []any{}
	if d := deref(op.Description); d != "" {
		kv = append(kv, "description", escapeText(d))
	}
	members := []string{}
	if op.Members != nil {
		members = *op.Members
	}
	kv = append(kv, "orchestrator", orch, "members", list(members))
	e.summary = append(e.summary, "add hub #"+op.ID)
	if err := e.ensure("x-hikari", "hubs"); err != nil {
		return err
	}
	return e.setValue("x-hikari.hubs", op.ID, mapping(kv...), false)
}

func (e *editor) removeHub(op Op) error {
	h, err := e.hub(op.ID)
	if err != nil {
		return err
	}
	for _, a := range e.model.Agents {
		if a.Tests == h.ID {
			return fmt.Errorf("%q tests #%s; change its tests first", a.ID, h.ID)
		}
		in := 0
		for _, other := range e.model.Hubs {
			if other.Orchestrator == a.ID || slices.Contains(other.Members, a.ID) {
				in++
			}
		}
		if in == 1 && (h.Orchestrator == a.ID || slices.Contains(h.Members, a.ID)) {
			return fmt.Errorf("%q would be in no hub; add it to another hub first, or remove it", a.ID)
		}
	}
	e.leaves("hubs/" + h.ID)
	e.summary = append(e.summary, "remove hub #"+h.Name)
	return e.deleteKey("x-hikari.hubs", h.ID)
}

const defaultService = "    image: hikari-agent\n    environment: [CLAUDE_CODE_OAUTH_TOKEN, ANTHROPIC_API_KEY]\n"

var anchor = regexp.MustCompile(`\s&[A-Za-z0-9_-]+`)

func (e *editor) addAgent(op Op) error {
	if err := checkID(op.ID); err != nil {
		return err
	}
	if e.nodeTaken(op.ID) {
		return fmt.Errorf("the id %q is taken", op.ID)
	}
	body := defaultService
	if op.Template != "" {
		if _, err := e.agent(op.Template); err != nil {
			return err
		}
		d, err := parse(e.text)
		if err != nil {
			return err
		}
		t, _ := d.find(path{"services", op.Template})
		var copied string
		if isFlow(t.value) {
			// A service on one line: its keys become block lines.
			indent := strings.Repeat(" ", serviceIndent(e.text))
			for i := 0; i+1 < len(t.value.Content); i += 2 {
				if k := t.value.Content[i]; k.Value != "x-hikari" {
					copied += indent + mustRender(clean(k)) + ": " + mustRender(t.value.Content[i+1]) + "\n"
				}
			}
		} else {
			start, end := d.entrySpan(t)
			copied = string(d.text[d.lineEnd(start):end])
			if x, ok := d.find(path{"services", op.Template, "x-hikari"}); ok {
				xs, xe := d.entrySpan(x)
				copied = string(d.text[d.lineEnd(start):xs]) + string(d.text[xe:end])
			}
		}
		body = anchor.ReplaceAllString(copied, "")
		if body != "" && !strings.HasSuffix(body, "\n") {
			body += "\n"
		}
	}
	kv := []any{}
	if d := deref(op.Description); d != "" {
		kv = append(kv, "description", escapeText(d))
	}
	indent := serviceIndent(e.text)
	body += strings.Repeat(" ", indent) + "x-hikari: " + mustRender(mapping(kv...)) + "\n"
	if err := e.addService(op.ID, body); err != nil {
		return err
	}
	summary := "add agent " + op.ID
	if op.Hubs != nil && len(*op.Hubs) > 0 {
		for _, hub := range *op.Hubs {
			e.model = ReadModel(e.text, e.dir)
			if err := e.apply(Op{Op: "add_member", Hub: hub, Agent: op.ID}); err != nil {
				return err
			}
			e.summary = e.summary[:len(e.summary)-1]
		}
		names := make([]string, len(*op.Hubs))
		for i, h := range *op.Hubs {
			names[i] = h
			if x, err := e.hub(h); err == nil {
				names[i] = x.Name
			}
		}
		summary += " in #" + strings.Join(names, ", #")
	}
	e.summary = append(e.summary, summary)
	return nil
}

// serviceIndent returns the indentation of the keys inside a service, from
// the first service of text, or 4.
func serviceIndent(text []byte) int {
	d, err := parse(text)
	if err != nil {
		return 4
	}
	s, ok := d.find(path{"services"})
	if !ok || s.value.Kind != yaml.MappingNode || len(s.value.Content) < 2 {
		return 4
	}
	svc := s.value.Content[1]
	if svc.Kind == yaml.MappingNode && !isFlow(svc) && len(svc.Content) > 0 {
		return svc.Content[0].Column - 1
	}
	return s.value.Content[0].Column - 1 + 2
}

// addService adds a service with the body under its key, after the last
// service, or in a new services mapping.
func (e *editor) addService(id, body string) error {
	d, err := parse(e.text)
	if err != nil {
		return err
	}
	s, ok := d.find(path{"services"})
	if !ok || s.value.Kind != yaml.MappingNode || isFlow(s.value) || len(s.value.Content) == 0 {
		if ok {
			return errors.New("the services mapping is not a block; add the service in the YAML view")
		}
		sep := "\n"
		if len(e.text) > 0 && e.text[len(e.text)-1] != '\n' {
			sep = "\n\n"
		}
		e.text = append(e.text, []byte(sep+"services:\n  "+id+":\n"+body)...)
		return nil
	}
	keyIndent := s.value.Content[0].Column - 1
	last := len(s.value.Content) - 2
	_, end := d.entrySpan(entry{key: s.value.Content[last], value: s.value.Content[last+1]})
	block := strings.Repeat(" ", keyIndent) + id + ":\n" + body
	// Keep the blank line that the file puts between services.
	if prev := s.value.Content[last].Line; prev > 1 && d.indent(prev-1) < 0 {
		block = "\n" + block
	}
	if end > 0 && d.text[end-1] != '\n' {
		block = "\n" + block
	}
	e.text = d.splice(end, end, block)
	return nil
}

func (e *editor) setSystem(op Op) error {
	a, err := e.agent(op.ID)
	if err != nil {
		return err
	}
	if err := e.ensure("services."+a.ID, "x-hikari"); err != nil {
		return err
	}
	p := "services." + a.ID + ".x-hikari"
	if a.System == nil {
		repo := deref(op.Repo)
		if repo == "" {
			return errors.New("a system needs a repo")
		}
		kv := []any{"repo", repo}
		if s := deref(op.Scope); s != "" {
			kv = append(kv, "scope", s)
		}
		if op.Read != nil && len(*op.Read) > 0 {
			kv = append(kv, "read", list(*op.Read))
		}
		if t := deref(op.Test); t != "" {
			kv = append(kv, "test", t)
		}
		e.summary = append(e.summary, "give "+a.Name+" a system")
		return e.setValue(p, "system", mapping(kv...), false)
	}
	if op.Repo != nil && *op.Repo == "" {
		return errors.New("a system needs a repo; clear the system instead")
	}
	p += ".system"
	for _, f := range []struct {
		key string
		v   *string
	}{{"repo", op.Repo}, {"scope", op.Scope}, {"test", op.Test}} {
		if err := e.setField(p, f.key, f.v, false); err != nil {
			return err
		}
	}
	if err := e.setList(p, "read", op.Read); err != nil {
		return err
	}
	e.summary = append(e.summary, "change the system of "+a.Name)
	return nil
}

func (e *editor) removeAgent(op Op) error {
	a, err := e.agent(op.ID)
	if err != nil {
		return err
	}
	for _, h := range e.model.Hubs {
		if h.Orchestrator == a.ID {
			return fmt.Errorf("%q orchestrates #%s; give the hub another orchestrator first", a.ID, h.ID)
		}
	}
	for _, other := range e.model.Agents {
		if slices.Contains(other.Releases, a.ID) {
			return fmt.Errorf("%q is granted the releases of %q; change its grants first", other.ID, a.ID)
		}
	}
	for _, s := range e.model.Sidecars {
		if s.Serves == a.ID {
			return fmt.Errorf("the sidecar %q serves %q; remove it first", s.ID, a.ID)
		}
	}
	for _, h := range e.model.Hubs {
		if slices.Contains(h.Members, a.ID) {
			if err := e.apply(Op{Op: "remove_member", Hub: h.ID, Agent: a.ID}); err != nil {
				return err
			}
			e.summary = e.summary[:len(e.summary)-1]
			e.model = ReadModel(e.text, e.dir)
		}
	}
	if a.System != nil {
		e.leaves("systems/" + a.ID)
	}
	e.summary = append(e.summary, "remove agent "+a.Name)
	return e.deleteKey("services", a.ID)
}

func (e *editor) rename(op Op) error {
	name := deref(op.Name)
	if err := checkID(name); err != nil {
		return fmt.Errorf("%q is not a valid name: use a lowercase letter, then lowercase letters, digits, and dashes, up to 32 characters", name)
	}
	var p, id, old string
	switch op.Kind {
	case "agent":
		a, err := e.agent(op.ID)
		if err != nil {
			return err
		}
		if err := e.ensure("services."+a.ID, "x-hikari"); err != nil {
			return err
		}
		p, id, old = "services."+a.ID+".x-hikari", a.ID, a.Name
	case "hub":
		h, err := e.hub(op.ID)
		if err != nil {
			return err
		}
		p, id, old = "x-hikari.hubs."+h.ID, h.ID, h.Name
	case "repo":
		r, err := e.repo(op.ID)
		if err != nil {
			return err
		}
		if name == topology.ProjectRepo {
			return fmt.Errorf("the name %q is reserved", name)
		}
		for _, other := range e.model.Repos {
			if other.ID != r.ID && (other.ID == name || other.Name == name) {
				return fmt.Errorf("the name %q is taken", name)
			}
		}
		p, id, old = "x-hikari.repos."+r.ID, r.ID, r.Name
	default:
		return fmt.Errorf("kind must be agent, hub, or repo, not %q", op.Kind)
	}
	if name == old {
		return nil
	}
	if op.Kind != "repo" && e.nodeTaken(name) && name != id {
		return fmt.Errorf("the name %q is taken", name)
	}
	var err error
	if name == id {
		err = e.deleteKey(p, "name")
	} else {
		err = e.setValue(p, "name", scalar(name), true)
	}
	if err != nil {
		return err
	}
	if op.Kind != "repo" {
		e.renames = append(e.renames, Rename{Kind: op.Kind, From: old, To: name})
	}
	sign := map[string]string{"agent": "", "hub": "#", "repo": ""}[op.Kind]
	e.summary = append(e.summary, fmt.Sprintf("rename %s %s%s to %s%s", op.Kind, sign, old, sign, name))
	return nil
}

func (e *editor) changeID(op Op) error {
	if err := checkID(op.NewID); err != nil {
		return err
	}
	var locked bool
	var name string
	switch op.Kind {
	case "agent":
		a, err := e.agent(op.ID)
		if err != nil {
			return err
		}
		locked, name = a.IDLocked, a.Name
	case "hub":
		h, err := e.hub(op.ID)
		if err != nil {
			return err
		}
		locked, name = h.IDLocked, h.Name
	case "repo":
		r, err := e.repo(op.ID)
		if err != nil {
			return err
		}
		locked, name = r.IDLocked, r.Name
		for _, other := range e.model.Repos {
			if other.ID == op.NewID || (other.ID != r.ID && other.Name == op.NewID) {
				return fmt.Errorf("the id %q is taken", op.NewID)
			}
		}
	default:
		return fmt.Errorf("kind must be agent, hub, or repo, not %q", op.Kind)
	}
	if locked {
		return fmt.Errorf("the store or a clone is keyed by %q, so its id cannot change; rename it instead", op.ID)
	}
	if op.Kind != "repo" && e.nodeTaken(op.NewID) && op.NewID != name {
		return fmt.Errorf("the id %q is taken", op.NewID)
	}
	type ref struct {
		p   string
		key string
		i   int // a list index, or -1
	}
	var refs []ref
	var parent string
	switch op.Kind {
	case "agent":
		parent = "services"
		for _, h := range e.model.Hubs {
			if h.Orchestrator == op.ID {
				refs = append(refs, ref{"x-hikari.hubs." + h.ID, "orchestrator", -1})
			}
			if i := slices.Index(h.Members, op.ID); i >= 0 {
				refs = append(refs, ref{"x-hikari.hubs." + h.ID + ".members", "", i})
			}
		}
		for _, a := range e.model.Agents {
			if i := slices.Index(a.Releases, op.ID); i >= 0 {
				refs = append(refs, ref{"services." + a.ID + ".x-hikari.grants.releases", "", i})
			}
		}
		for _, s := range e.model.Sidecars {
			if s.Serves == op.ID {
				refs = append(refs, ref{"services." + s.ID + ".x-hikari", "serves", -1})
			}
		}
	case "hub":
		parent = "x-hikari.hubs"
		for _, a := range e.model.Agents {
			if a.Tests == op.ID {
				refs = append(refs, ref{"services." + a.ID + ".x-hikari", "tests", -1})
			}
		}
	case "repo":
		parent = "x-hikari.repos"
		for _, a := range e.model.Agents {
			if a.System != nil && a.System.Repo == op.ID {
				refs = append(refs, ref{"services." + a.ID + ".x-hikari.system", "repo", -1})
			}
		}
	}
	// The references first: renaming the key changes their paths.
	for _, r := range refs {
		var err error
		if r.i >= 0 {
			err = e.edit(func(d *doc) ([]byte, error) {
				return d.setItem(parsePath(fmt.Sprintf("%s[%d]", r.p, r.i)), scalar(op.NewID))
			})
		} else {
			err = e.setValue(r.p, r.key, scalar(op.NewID), false)
		}
		if err != nil {
			return err
		}
	}
	if err := e.edit(func(d *doc) ([]byte, error) { return d.renameKey(parsePath(parent), op.ID, op.NewID) }); err != nil {
		return err
	}
	// An entity without a name field was named by its id, so its name
	// changes too.
	if name == op.ID && op.Kind != "repo" {
		e.renames = append(e.renames, Rename{Kind: op.Kind, From: op.ID, To: op.NewID})
	}
	e.summary = append(e.summary, fmt.Sprintf("change the id of %s %s to %s", op.Kind, op.ID, op.NewID))
	return nil
}
