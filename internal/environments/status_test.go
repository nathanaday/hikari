package environments

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// The status goes to agents as YAML: every field must marshal.
func TestStatusMarshalsAsYAML(t *testing.T) {
	for _, s := range []Status{{State: None}, {State: Running, Started: time.Now(), Stops: time.Now(), Services: []Service{{Name: "a", Replicas: 1}}}} {
		if _, err := yaml.Marshal(s); err != nil {
			t.Fatal(err)
		}
	}
}
