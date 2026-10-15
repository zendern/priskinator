package policy

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestDefaultsAreValid(t *testing.T) {
	p, err := Parse(defaultsYAML)
	if err != nil {
		t.Fatalf("built-in defaults do not validate: %v", err)
	}
	if p.Version != 1 {
		t.Errorf("Version = %d", p.Version)
	}
	if d := Defaults(); d == p || d.Triggers["G1.ci-workflows"] == nil {
		t.Error("Defaults() must return a fresh, populated copy")
	}
}

// Every trigger group from ADR 0006 has at least one rule in the baseline.
func TestDefaultsCoverEveryTriggerGroup(t *testing.T) {
	d := Defaults()
	for g := 1; g <= 10; g++ {
		prefix := "G" + strconv.Itoa(g) + "."
		found := false
		for id := range d.Triggers {
			if strings.HasPrefix(id, prefix) {
				found = true
			}
		}
		if !found {
			t.Errorf("no default rule in group %s", prefix)
		}
	}
	for id, tr := range d.Triggers {
		if tr.Risk == "" {
			t.Errorf("%s has no risk floor", id)
		}
		if len(tr.Reviewers) == 0 {
			t.Errorf("%s has no reviewers", id)
		}
	}
}

func TestDefaultsPathExamples(t *testing.T) {
	d := Defaults()
	cases := map[string][]string{
		"G1.ci-workflows":     {".github/workflows/**", "**/action.yml"},
		"G2.gate-governance":  {".priskinator/**", ".github/chainguard/**", "**/.gitattributes"},
		"G3.lint-config":      {"**/.golangci.y*ml", "**/eslint.config.*"},
		"G4.hook-managers":    {"**/.pre-commit-config.yaml", "**/lefthook.yml", ".husky/**"},
		"G5.test-harness":     {"**/conftest.py", "**/jest.config.*"},
		"G6.build-scripts":    {"**/Makefile", "**/Taskfile.y*ml", "**/package.json"},
		"G7.security-paths":   {"**/auth/**"},
		"G8.registry-config":  {"**/.npmrc"},
		"G10.oversize-file":   nil,
		"G9.suppressions":     nil,
		"G7.security-content": nil,
	}
	for id, want := range cases {
		tr, ok := d.Triggers[id]
		if !ok {
			t.Errorf("missing default rule %s", id)
			continue
		}
		for _, p := range want {
			if !slices.Contains(tr.Paths, p) {
				t.Errorf("%s paths missing %q", id, p)
			}
		}
	}
}

func TestDefaultsAIOff(t *testing.T) {
	if Defaults().AI.Enabled {
		t.Error("AI must be disabled by default (ADR 0008)")
	}
}

func TestParseADRExample(t *testing.T) {
	p, err := Parse([]byte(`
version: 1
extends: org
triggers:
  G7.security-paths:
    add_paths:
      - "services/billing/**"
    reviewers: [security, payments]
  repo.feature-flags:
    paths: ["config/flags/**"]
    reviewers: [platform]
scoring:
  floors:
    - paths: ["db/migrations/**"]
      risk: high
ai:
  enabled: true
  provider: anthropic
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	g7 := p.Triggers["G7.security-paths"]
	if g7 == nil || !slices.Equal(g7.AddPaths, []string{"services/billing/**"}) || !slices.Equal(g7.Reviewers, []string{"security", "payments"}) {
		t.Errorf("G7 = %+v", g7)
	}
	if ff := p.Triggers["repo.feature-flags"]; ff == nil || !slices.Equal(ff.Paths, []string{"config/flags/**"}) {
		t.Errorf("repo.feature-flags = %+v", ff)
	}
	if len(p.Scoring.Floors) != 1 || p.Scoring.Floors[0].Risk != RiskHigh {
		t.Errorf("floors = %+v", p.Scoring.Floors)
	}
	if !p.AI.Enabled || p.AI.Provider != "anthropic" {
		t.Errorf("ai = %+v", p.AI)
	}
}

func TestParseRejects(t *testing.T) {
	cases := map[string]struct {
		yaml string
		want string // substring of the error
		path string // location of the problem, if any
	}{
		"empty document":       {"", "empty policy", ""},
		"not a mapping":        {"- version: 1\n", "expected object", ""},
		"missing version":      {"triggers: {}\n", "version", ""},
		"unknown top-level":    {"version: 1\ntrigger: {}\n", "trigger", ""},
		"unknown nested key":   {"version: 1\ntriggers:\n  G1.ci-workflows:\n    pathz: [a]\n", "pathz", "/triggers/G1.ci-workflows"},
		"paths not a list":     {"version: 1\ntriggers:\n  repo.x:\n    paths: config/**\n", "array", "/triggers/repo.x/paths"},
		"version as string":    {"version: \"1\"\n", "version", "/version"},
		"bad rule id":          {"version: 1\ntriggers:\n  custom-rule:\n    paths: [a]\n", "custom-rule", "/triggers"},
		"unknown group":        {"version: 1\ntriggers:\n  G11.extra:\n    paths: [a]\n", "G11.extra", "/triggers"},
		"invalid risk":         {"version: 1\ntriggers:\n  repo.x:\n    paths: [a]\n    risk: critical\n", "critical", "/triggers/repo.x/risk"},
		"enabled not bool":     {"version: 1\ntriggers:\n  G1.ci-workflows:\n    enabled: \"no\"\n", "boolean", "/triggers/G1.ci-workflows/enabled"},
		"invalid glob":         {"version: 1\ntriggers:\n  repo.x:\n    paths: [\"src/[abc\"]\n", "invalid glob", "/triggers/repo.x/paths/0"},
		"absolute glob":        {"version: 1\ntriggers:\n  repo.x:\n    paths: [\"/etc/**\"]\n", "relative", "/triggers/repo.x/paths/0"},
		"backslash glob":       {"version: 1\ntriggers:\n  repo.x:\n    paths: [\"src\\\\auth\\\\**\"]\n", "forward slashes", "/triggers/repo.x/paths/0"},
		"new rule needs paths": {"version: 1\ntriggers:\n  repo.x:\n    reviewers: [a]\n", "paths", "/triggers/repo.x"},
		"bad reviewer":         {"version: 1\ntriggers:\n  repo.x:\n    paths: [a]\n    reviewers: [\"@team\"]\n", "@team", "/triggers/repo.x/reviewers/0"},
		"thresholds order":     {"version: 1\nscoring:\n  thresholds: {medium: 50, high: 20}\n", "medium must be below high", "/scoring/thresholds"},
		"bands not ascending":  {"version: 1\nscoring:\n  factors:\n    size:\n      - {up_to: 100, points: 0}\n      - {up_to: 50, points: 5}\n      - {points: 10}\n", "ascending", "/scoring/factors/size/1"},
		"unbounded not last":   {"version: 1\nscoring:\n  factors:\n    size:\n      - {points: 0}\n      - {up_to: 50, points: 5}\n", "only the last band", "/scoring/factors/size/0"},
		"last band bounded":    {"version: 1\nscoring:\n  factors:\n    size:\n      - {up_to: 50, points: 5}\n", "last band", "/scoring/factors/size/0"},
		"points decrease":      {"version: 1\nscoring:\n  factors:\n    size:\n      - {up_to: 50, points: 10}\n      - {points: 5}\n", "points", "/scoring/factors/size/1"},
		"unknown factor":       {"version: 1\nscoring:\n  factors:\n    vibes:\n      - {points: 5}\n", "vibes", "/scoring/factors"},
		"floor without paths":  {"version: 1\nscoring:\n  floors:\n    - risk: high\n", "paths", "/scoring/floors/0"},
		"negative limit":       {"version: 1\nlimits:\n  max_file_bytes: -1\n", "minimum", "/limits/max_file_bytes"},
		"unknown ai provider":  {"version: 1\nai:\n  enabled: true\n  provider: skynet\n", "skynet", "/ai/provider"},
		"extends none":         {"version: 1\nextends: none\n", "none", "/extends"},
		"duplicate key":        {"version: 1\ntriggers:\n  repo.x:\n    paths: [a]\n  repo.x:\n    paths: [b]\n", "already defined", ""},
		"multiple documents":   {"version: 1\n---\nversion: 1\n", "single YAML document", ""},
		"non-string key":       {"version: 1\n1: x\n", "1", ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(tc.yaml))
			if err == nil {
				t.Fatal("Parse succeeded, want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
			if tc.path == "" {
				return
			}
			var perr *Error
			if !errors.As(err, &perr) {
				t.Fatalf("error %T is not *policy.Error", err)
			}
			found := false
			for _, p := range perr.Problems {
				if p.Path == tc.path {
					found = true
				}
			}
			if !found {
				t.Errorf("no problem at %s: %+v", tc.path, perr.Problems)
			}
		})
	}
}

func TestParseUnsupportedVersion(t *testing.T) {
	for _, v := range []string{"0", "2", "99"} {
		_, err := Parse([]byte("version: " + v + "\n"))
		if !errors.Is(err, ErrUnsupportedVersion) {
			t.Errorf("version %s: err = %v, want ErrUnsupportedVersion", v, err)
		}
	}
}

func TestParseRejectsOversizeInput(t *testing.T) {
	big := "version: 1\n# " + strings.Repeat("x", MaxPolicyBytes) + "\n"
	if _, err := Parse([]byte(big)); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Errorf("err = %v", err)
	}
}

func TestParseReportsEveryProblem(t *testing.T) {
	_, err := Parse([]byte("version: 1\nbogus: 1\ntriggers:\n  repo.x:\n    paths: [a]\n    risk: critical\n"))
	var perr *Error
	if !errors.As(err, &perr) {
		t.Fatalf("err = %v", err)
	}
	if len(perr.Problems) < 2 {
		t.Errorf("want several problems, got %+v", perr.Problems)
	}
	_, err = Parse([]byte("version: 1\ntriggers:\n  repo.x:\n    paths: [\"/abs\", \"[bad\"]\n"))
	if !errors.As(err, &perr) || len(perr.Problems) != 2 {
		t.Errorf("want both glob problems, got %v", err)
	}
}

func TestRiskOrder(t *testing.T) {
	if RiskLow.Rank() >= RiskMedium.Rank() || RiskMedium.Rank() >= RiskHigh.Rank() {
		t.Error("risk ranks out of order")
	}
	if MaxRisk(RiskLow, RiskHigh, RiskMedium) != RiskHigh {
		t.Error("MaxRisk")
	}
}
