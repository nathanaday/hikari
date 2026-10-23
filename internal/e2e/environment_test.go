package e2e

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const environment = `services:
  cloud:
    x-hikari: {release: cloud/http-service}
    depends_on: {db: {condition: service_healthy}}
    environment: {DATABASE_URL: "postgres://app:app@db/app", PRICE: "$$5"}
  edge:
    image: debian:bookworm-slim
    x-hikari:
      mount:
        edge/tarball: /opt/edge
        scope:fixtures/readings: /data
    command: [/opt/edge/bin/edge]
    volumes: [cache:/var/cache, {type: tmpfs, target: /tmp}]
  db:
    image: postgres:16
`

func TestParseEnvironment(t *testing.T) {
	env, err := ParseEnvironment([]byte(environment))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(env.Names(), ","); got != "cloud,db,edge" {
		t.Errorf("names = %s", got)
	}
	if env.Services["cloud"].Release != "cloud/http-service" || env.Services["edge"].Mounts["scope:fixtures/readings"] != "/data" {
		t.Errorf("services = %+v", env.Services)
	}
	for name, c := range map[string]struct{ text, want string }{
		"EN1 top key":     {"name: x\nservices: {a: {image: i}}\n", `top-level key "name"`},
		"EN1 no services": {"services: {}\n", "no services"},
		"EN2 both":        {"services: {a: {image: i, x-hikari: {release: a/b}}}\n", "both a release and an image"},
		"EN2 neither":     {"services: {a: {command: [x]}}\n", "needs an image"},
		"EN2 bad release": {"services: {a: {x-hikari: {release: ab}}}\n", "<system>/<deliverable>"},
		"EN2 bad field":   {"services: {a: {image: i, x-hikari: {serves: x}}}\n", "field serves not found"},
		"EN2 escaping":    {"services: {a: {image: i, x-hikari: {mount: {'scope:../up': /x}}}}\n", "not a directory inside your scope"},
		"EN2 relative":    {"services: {a: {image: i, x-hikari: {mount: {a/b: data}}}}\n", "not an absolute path"},
		"EN3 build":       {"services: {a: {build: ., image: i}}\n", "sets build"},
		"EN3 ports":       {"services: {a: {image: i, ports: [80:80]}}\n", "sets ports"},
		"EN3 privileged":  {"services: {a: {image: i, privileged: true}}\n", "sets privileged"},
		"EN3 bind short":  {"services: {a: {image: i, volumes: [/etc:/etc]}}\n", "bind mounts /etc"},
		"EN3 bind long":   {"services: {a: {image: i, volumes: [{type: bind, source: ./x, target: /x}]}}\n", "bind mounts ./x"},
		"CR1 list":        {"services: {a: {image: i, environment: [TOKEN]}}\n", "passes a variable of the host"},
		"CR1 map":         {"services: {a: {image: i, environment: {TOKEN: }}}\n", "passes a variable of the host"},
		"CR1 interpolate": {"services: {a: {image: i, environment: {A: \"${TOKEN}\"}}}\n", "reads a variable of the host"},
	} {
		if _, err := ParseEnvironment([]byte(c.text)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", name, err, c.want)
		}
	}
}

func TestEnvironmentAgainstPlan(t *testing.T) {
	env, _ := ParseEnvironment([]byte(environment))
	p, _, err := ParsePlan(`---
components:
  cloud: {release: cloud/http-service}
  edge: {release: edge/tarball}
  db: {image: postgres:16, substitutes: the managed database}
  sensor: {simulates: a temperature sensor}
actors:
  gateways: {runs: edge, count: 5}
  operator: {runs: tests, count: 1}
scenarios:
  smoke: {stories: [s], actors: {operator: 1}}
  load: {stories: [s], actors: {gateways: 2, operator: 1}}
---
`)
	if err != nil {
		t.Fatal(err)
	}
	findings := strings.Join(env.PlanFindings(p, map[string]bool{"cloud/http-service": true}), "\n")
	if !strings.Contains(findings, `the component "sensor" runs, so environment.yaml needs a service`) || strings.Contains(findings, `"edge"`) {
		t.Errorf("EN4 findings:\n%s", findings)
	}
	if f := env.PlanFindings(p, map[string]bool{}); !strings.Contains(strings.Join(f, "\n"), "which is no image release") {
		t.Errorf("EN4: a release that is no image: %v", f)
	}
	r, err := env.Replicas(p, "load")
	if err != nil || r["edge"] != 10 || r["cloud"] != 1 || r["db"] != 1 {
		t.Errorf("EN5 replicas = %v, %v", r, err)
	}
	if r, _ := env.Replicas(p, "smoke"); r["edge"] != 1 {
		t.Errorf("EN5 smoke = %v", r)
	}
	if _, err := env.Replicas(p, "ghost"); err == nil {
		t.Error("EN5: no such scenario")
	}
}

func TestOverride(t *testing.T) {
	env, _ := ParseEnvironment([]byte(environment))
	data, err := env.Override(OverrideInput{
		Network:  "p-e2e-main",
		Labels:   map[string]string{"hikari.run": "r1"},
		Images:   map[string]string{"cloud/http-service": "hikari/p-cloud-http-service:1.0.0"},
		Products: map[string]string{"edge/tarball": "/store/systems/edge/releases/tarball/1.0.0/product"},
		Scope:    "/repo/e2e/main",
		CPUs:     "2",
	})
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Services map[string]struct {
			Image    string   `yaml:"image"`
			Networks []string `yaml:"networks"`
			Volumes  []struct {
				Source   string `yaml:"source"`
				Target   string `yaml:"target"`
				ReadOnly bool   `yaml:"read_only"`
			} `yaml:"volumes"`
			Deploy struct {
				Resources struct {
					Limits struct {
						CPUs string `yaml:"cpus"`
					} `yaml:"limits"`
				} `yaml:"resources"`
			} `yaml:"deploy"`
		} `yaml:"services"`
		Networks map[string]struct {
			Name string `yaml:"name"`
		} `yaml:"networks"`
	}
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out.Services["cloud"].Image != "hikari/p-cloud-http-service:1.0.0" || out.Networks["e2e"].Name != "p-e2e-main" || out.Services["db"].Networks[0] != "e2e" {
		t.Errorf("ST2 override:\n%s", data)
	}
	edge := out.Services["edge"]
	if len(edge.Volumes) != 2 || edge.Volumes[0].Source != "/store/systems/edge/releases/tarball/1.0.0/product" || !edge.Volumes[0].ReadOnly || edge.Volumes[1].Source != "/repo/e2e/main/fixtures/readings" || edge.Deploy.Resources.Limits.CPUs != "2" {
		t.Errorf("ST2 mounts:\n%s", data)
	}
	if _, err := env.Override(OverrideInput{}); err == nil {
		t.Error("an image release without an image")
	}
}
