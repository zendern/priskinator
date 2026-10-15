package fixture

import "testing"

func TestPolicyYAML(t *testing.T) {
	if got := (Fixture{}).PolicyYAML(); got != nil {
		t.Errorf("no policy: got %q, want nil", got)
	}
	if got := (Fixture{Policy: map[string]any{"version": 1}}).PolicyYAML(); string(got) != "version: 1\n" {
		t.Errorf("policy: got %q", got)
	}
}
