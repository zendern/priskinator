package policy

import (
	"os"
	"path/filepath"
	"testing"
)

// The repository's own policy must stay valid and only tighten the defaults,
// or dogfooding evaluates every PR against config-invalid.
func TestRepoPolicy(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".priskinator", "policy.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := Resolve(nil, data)
	if err != nil {
		t.Fatal(err)
	}
	if res.ConfigErr != nil || len(res.Weakenings) > 0 || len(res.Triggers) > 0 {
		t.Errorf("repo policy: config error %v, weakenings %+v, triggers %v", res.ConfigErr, res.Weakenings, res.Triggers)
	}
	if len(res.Policy.Scoring.Floors) != 2 {
		t.Errorf("floors = %+v, want the two repo floors", res.Policy.Scoring.Floors)
	}
}
