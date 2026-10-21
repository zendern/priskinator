package fixture

import "go.yaml.in/yaml/v3"

// PolicyYAML returns the fixture's repo policy as YAML, or nil when the
// fixture sets none.
func (f Fixture) PolicyYAML() []byte {
	if f.Policy == nil {
		return nil
	}
	// The policy was decoded from YAML, so it holds only maps, slices, and
	// scalars, which always encode.
	data, _ := yaml.Marshal(f.Policy)
	return data
}
