package sysmap

import (
	"bytes"

	"gopkg.in/yaml.v3"
)

// marshalYAML encodes v with two-space indentation, like the documents that
// owners write by hand.
func marshalYAML(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
