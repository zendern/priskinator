package policy

import (
	"encoding/json"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
)

func mustParse(t *testing.T, y string) *Policy {
	t.Helper()
	p, err := Parse([]byte(y))
	if err != nil {
		t.Fatalf("Parse: %v\n%s", err, y)
	}
	return p
}

// layered merges a repo policy over the defaults, optionally under an org baseline.
func layered(t *testing.T, org, repo string) (*Policy, []Weakening) {
	t.Helper()
	base := Defaults()
	if org != "" {
		base = Overlay(base, mustParse(t, org))
	}
	return Merge(base, mustParse(t, repo))
}

type direction int

const (
	weaken  direction = iota // repo tries to loosen the field: rejected, reported, base kept
	tighten                  // repo tightens the field: applied, nothing reported
)

// fieldCase exercises one policy field. For weaken cases, paths lists the
// JSON pointers that must be reported; for tighten cases nothing may be.
type fieldCase struct {
	field string // dotted name, matched against the schema by TestEveryFieldHasBothCases
	dir   direction
	name  string
	org   string
	repo  string
	paths []string
	check func(t *testing.T, got *Policy)
}

func trigger(t *testing.T, p *Policy, id string) *Trigger {
	t.Helper()
	tr := p.Triggers[id]
	if tr == nil {
		t.Fatalf("no rule %s", id)
	}
	return tr
}

func intsEq[T comparable](t *testing.T, what string, got *T, want T) {
	t.Helper()
	if got == nil || *got != want {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

func fieldCases() []fieldCase {
	const g7 = "G7.security-paths"
	return []fieldCase{
		// match
		{
			field: "match.case_sensitive", dir: weaken, name: "case-sensitive matching skips paths",
			repo: "version: 1\nmatch: {case_sensitive: true}\n", paths: []string{"/match/case_sensitive"},
			check: func(t *testing.T, got *Policy) { intsEq(t, "case_sensitive", got.Match.CaseSensitive, false) },
		},
		{
			field: "match.case_sensitive", dir: tighten, name: "case-insensitive over a case-sensitive baseline",
			org:   "version: 1\nmatch: {case_sensitive: true}\n",
			repo:  "version: 1\nmatch: {case_sensitive: false}\n",
			check: func(t *testing.T, got *Policy) { intsEq(t, "case_sensitive", got.Match.CaseSensitive, false) },
		},

		// limits: lower is stricter, because an oversize diff gets human review
		{
			field: "limits.max_file_bytes", dir: weaken, name: "higher file cap",
			repo: "version: 1\nlimits: {max_file_bytes: 2097152}\n", paths: []string{"/limits/max_file_bytes"},
			check: func(t *testing.T, got *Policy) { intsEq(t, "max_file_bytes", got.Limits.MaxFileBytes, 1048576) },
		},
		{
			field: "limits.max_file_bytes", dir: tighten, name: "lower file cap",
			repo:  "version: 1\nlimits: {max_file_bytes: 1024}\n",
			check: func(t *testing.T, got *Policy) { intsEq(t, "max_file_bytes", got.Limits.MaxFileBytes, 1024) },
		},
		{
			field: "limits.max_changed_files", dir: weaken, name: "higher file count cap",
			repo: "version: 1\nlimits: {max_changed_files: 301}\n", paths: []string{"/limits/max_changed_files"},
			check: func(t *testing.T, got *Policy) { intsEq(t, "max_changed_files", got.Limits.MaxChangedFiles, 300) },
		},
		{
			field: "limits.max_changed_files", dir: tighten, name: "lower file count cap",
			repo:  "version: 1\nlimits: {max_changed_files: 50}\n",
			check: func(t *testing.T, got *Policy) { intsEq(t, "max_changed_files", got.Limits.MaxChangedFiles, 50) },
		},
		{
			field: "limits.max_changed_lines", dir: weaken, name: "higher line cap",
			repo: "version: 1\nlimits: {max_changed_lines: 9000}\n", paths: []string{"/limits/max_changed_lines"},
			check: func(t *testing.T, got *Policy) { intsEq(t, "max_changed_lines", got.Limits.MaxChangedLines, 5000) },
		},
		{
			field: "limits.max_changed_lines", dir: tighten, name: "lower line cap",
			repo:  "version: 1\nlimits: {max_changed_lines: 100}\n",
			check: func(t *testing.T, got *Policy) { intsEq(t, "max_changed_lines", got.Limits.MaxChangedLines, 100) },
		},

		// triggers
		{
			field: "triggers.enabled", dir: weaken, name: "disable a baseline rule",
			repo: "version: 1\ntriggers:\n  G1.ci-workflows: {enabled: false}\n", paths: []string{"/triggers/G1.ci-workflows/enabled"},
			check: func(t *testing.T, got *Policy) {
				if !trigger(t, got, "G1.ci-workflows").IsEnabled() {
					t.Error("rule is disabled")
				}
			},
		},
		{
			field: "triggers.enabled", dir: tighten, name: "enable a rule the org turned off",
			org:  "version: 1\ntriggers:\n  G9.suppressions: {enabled: false}\n",
			repo: "version: 1\ntriggers:\n  G9.suppressions: {enabled: true}\n",
			check: func(t *testing.T, got *Policy) {
				if !trigger(t, got, "G9.suppressions").IsEnabled() {
					t.Error("rule is still disabled")
				}
			},
		},
		{
			field: "triggers.paths", dir: weaken, name: "drop baseline paths",
			repo: "version: 1\ntriggers:\n  " + g7 + ": {paths: [\"**/auth/**\"]}\n", paths: []string{"/triggers/" + g7 + "/paths"},
			check: func(t *testing.T, got *Policy) {
				if !slices.Equal(trigger(t, got, g7).Paths, Defaults().Triggers[g7].Paths) {
					t.Errorf("paths = %v", trigger(t, got, g7).Paths)
				}
			},
		},
		{
			field: "triggers.paths", dir: tighten, name: "baseline paths plus more",
			repo: "version: 1\ntriggers:\n  " + g7 + ": {paths: [\"**/auth/**\", \"**/authn/**\", \"**/authz/**\", \"**/security/**\", \"**/crypto/**\", \"**/session*/**\", \"**/middleware/*auth*\", \"services/billing/**\"]}\n",
			check: func(t *testing.T, got *Policy) {
				if !slices.Contains(trigger(t, got, g7).Paths, "services/billing/**") {
					t.Errorf("paths = %v", trigger(t, got, g7).Paths)
				}
			},
		},
		{
			field: "triggers.add_paths", dir: tighten, name: "extend a baseline rule",
			repo: "version: 1\ntriggers:\n  " + g7 + ": {add_paths: [\"services/billing/**\"]}\n",
			check: func(t *testing.T, got *Policy) {
				tr := trigger(t, got, g7)
				base := Defaults().Triggers[g7].Paths
				if !slices.Contains(tr.Paths, "services/billing/**") || len(tr.Paths) != len(base)+1 || len(tr.AddPaths) != 0 {
					t.Errorf("paths = %v, add_paths = %v", tr.Paths, tr.AddPaths)
				}
			},
		},
		{
			field: "triggers.reviewers", dir: weaken, name: "drop a required reviewer",
			repo: "version: 1\ntriggers:\n  " + g7 + ": {reviewers: [payments]}\n", paths: []string{"/triggers/" + g7 + "/reviewers"},
			check: func(t *testing.T, got *Policy) {
				if !slices.Contains(trigger(t, got, g7).Reviewers, "security") {
					t.Errorf("reviewers = %v", trigger(t, got, g7).Reviewers)
				}
			},
		},
		{
			field: "triggers.reviewers", dir: tighten, name: "add a reviewer",
			repo: "version: 1\ntriggers:\n  " + g7 + ": {reviewers: [security, payments]}\n",
			check: func(t *testing.T, got *Policy) {
				if r := trigger(t, got, g7).Reviewers; !slices.Equal(r, []string{"security", "payments"}) {
					t.Errorf("reviewers = %v", r)
				}
			},
		},
		{
			field: "triggers.risk", dir: weaken, name: "lower a risk floor",
			repo: "version: 1\ntriggers:\n  G3.lint-config: {risk: low}\n", paths: []string{"/triggers/G3.lint-config/risk"},
			check: func(t *testing.T, got *Policy) {
				if r := trigger(t, got, "G3.lint-config").Risk; r != RiskMedium {
					t.Errorf("risk = %s", r)
				}
			},
		},
		{
			field: "triggers.risk", dir: tighten, name: "raise a risk floor",
			repo: "version: 1\ntriggers:\n  G3.lint-config: {risk: high}\n",
			check: func(t *testing.T, got *Policy) {
				if r := trigger(t, got, "G3.lint-config").Risk; r != RiskHigh {
					t.Errorf("risk = %s", r)
				}
			},
		},
		{
			field: "triggers.description", dir: tighten, name: "description cannot change a baseline rule",
			repo: "version: 1\ntriggers:\n  G1.ci-workflows: {description: \"nothing to see\"}\n",
			check: func(t *testing.T, got *Policy) {
				if d := trigger(t, got, "G1.ci-workflows").Description; d != Defaults().Triggers["G1.ci-workflows"].Description {
					t.Errorf("description = %q", d)
				}
			},
		},
		{
			field: "triggers.new", dir: tighten, name: "add a repo rule",
			repo: "version: 1\ntriggers:\n  repo.flags: {paths: [\"config/flags/**\"], reviewers: [platform], risk: high}\n",
			check: func(t *testing.T, got *Policy) {
				tr := trigger(t, got, "repo.flags")
				if !slices.Equal(tr.Paths, []string{"config/flags/**"}) || tr.Risk != RiskHigh {
					t.Errorf("rule = %+v", tr)
				}
			},
		},

		// scoring.thresholds: lower is stricter
		{
			field: "scoring.thresholds.medium", dir: weaken, name: "higher medium threshold",
			repo: "version: 1\nscoring:\n  thresholds: {medium: 30, high: 40}\n", paths: []string{"/scoring/thresholds/medium"},
			check: func(t *testing.T, got *Policy) {
				intsEq(t, "medium", &got.Scoring.Thresholds.Medium, 20)
				intsEq(t, "high", &got.Scoring.Thresholds.High, 40)
			},
		},
		{
			field: "scoring.thresholds.medium", dir: tighten, name: "lower medium threshold",
			repo:  "version: 1\nscoring:\n  thresholds: {medium: 10, high: 50}\n",
			check: func(t *testing.T, got *Policy) { intsEq(t, "medium", &got.Scoring.Thresholds.Medium, 10) },
		},
		{
			field: "scoring.thresholds.high", dir: weaken, name: "higher high threshold",
			repo: "version: 1\nscoring:\n  thresholds: {medium: 10, high: 80}\n", paths: []string{"/scoring/thresholds/high"},
			check: func(t *testing.T, got *Policy) {
				intsEq(t, "medium", &got.Scoring.Thresholds.Medium, 10)
				intsEq(t, "high", &got.Scoring.Thresholds.High, 50)
			},
		},
		{
			field: "scoring.thresholds.high", dir: tighten, name: "lower high threshold",
			repo:  "version: 1\nscoring:\n  thresholds: {medium: 20, high: 30}\n",
			check: func(t *testing.T, got *Policy) { intsEq(t, "high", &got.Scoring.Thresholds.High, 30) },
		},

		// scoring.factors: more points for the same value is stricter
		{
			field: "scoring.factors.size", dir: weaken, name: "fewer points for big diffs",
			repo: "version: 1\nscoring:\n  factors:\n    size:\n      - {points: 0}\n", paths: []string{"/scoring/factors/size"},
			check: func(t *testing.T, got *Policy) {
				if !reflect.DeepEqual(got.Scoring.Factors.Size, Defaults().Scoring.Factors.Size) {
					t.Errorf("size = %+v", got.Scoring.Factors.Size)
				}
			},
		},
		{
			field: "scoring.factors.size", dir: tighten, name: "different breakpoints that never score lower",
			repo: "version: 1\nscoring:\n  factors:\n    size:\n      - {up_to: 10, points: 0}\n      - {points: 40}\n",
			check: func(t *testing.T, got *Policy) {
				if b := got.Scoring.Factors.Size; len(b) != 2 || b.Points(11) != 40 {
					t.Errorf("size = %+v", b)
				}
			},
		},
		{
			field: "scoring.factors.files", dir: weaken, name: "fewer points for many files",
			repo: "version: 1\nscoring:\n  factors:\n    files:\n      - {up_to: 10, points: 0}\n      - {up_to: 30, points: 5}\n      - {up_to: 100, points: 15}\n      - {points: 20}\n", paths: []string{"/scoring/factors/files"},
			check: func(t *testing.T, got *Policy) {
				if got.Scoring.Factors.Files.Points(1000) != 25 {
					t.Errorf("files = %+v", got.Scoring.Factors.Files)
				}
			},
		},
		{
			field: "scoring.factors.files", dir: tighten, name: "more points for many files",
			repo: "version: 1\nscoring:\n  factors:\n    files:\n      - {up_to: 10, points: 5}\n      - {points: 30}\n",
			check: func(t *testing.T, got *Policy) {
				if got.Scoring.Factors.Files.Points(1000) != 30 {
					t.Errorf("files = %+v", got.Scoring.Factors.Files)
				}
			},
		},
		{
			field: "scoring.factors.diffusion", dir: weaken, name: "fewer points for wide diffs",
			repo: "version: 1\nscoring:\n  factors:\n    diffusion:\n      - {points: 0}\n", paths: []string{"/scoring/factors/diffusion"},
			check: func(t *testing.T, got *Policy) {
				if got.Scoring.Factors.Diffusion.Points(50) != 25 {
					t.Errorf("diffusion = %+v", got.Scoring.Factors.Diffusion)
				}
			},
		},
		{
			field: "scoring.factors.diffusion", dir: tighten, name: "more points for wide diffs",
			repo: "version: 1\nscoring:\n  factors:\n    diffusion:\n      - {up_to: 2, points: 1}\n      - {points: 40}\n",
			check: func(t *testing.T, got *Policy) {
				if got.Scoring.Factors.Diffusion.Points(50) != 40 {
					t.Errorf("diffusion = %+v", got.Scoring.Factors.Diffusion)
				}
			},
		},
		{
			field: "scoring.factors.complexity_delta", dir: weaken, name: "fewer points for complexity",
			repo: "version: 1\nscoring:\n  factors:\n    complexity_delta:\n      - {points: 0}\n", paths: []string{"/scoring/factors/complexity_delta"},
			check: func(t *testing.T, got *Policy) {
				if got.Scoring.Factors.ComplexityDelta.Points(50) != 25 {
					t.Errorf("complexity_delta = %+v", got.Scoring.Factors.ComplexityDelta)
				}
			},
		},
		{
			field: "scoring.factors.complexity_delta", dir: tighten, name: "more points for complexity",
			repo: "version: 1\nscoring:\n  factors:\n    complexity_delta:\n      - {up_to: 5, points: 0}\n      - {points: 30}\n",
			check: func(t *testing.T, got *Policy) {
				if got.Scoring.Factors.ComplexityDelta.Points(6) != 30 {
					t.Errorf("complexity_delta = %+v", got.Scoring.Factors.ComplexityDelta)
				}
			},
		},
		{
			field: "scoring.factors.test_signal", dir: weaken, name: "fewer points for untested changes",
			repo: "version: 1\nscoring:\n  factors:\n    test_signal:\n      - {points: 0}\n", paths: []string{"/scoring/factors/test_signal"},
			check: func(t *testing.T, got *Policy) {
				if got.Scoring.Factors.TestSignal.Points(500) != 15 {
					t.Errorf("test_signal = %+v", got.Scoring.Factors.TestSignal)
				}
			},
		},
		{
			field: "scoring.factors.test_signal", dir: tighten, name: "more points for untested changes",
			repo: "version: 1\nscoring:\n  factors:\n    test_signal:\n      - {up_to: 20, points: 0}\n      - {points: 20}\n",
			check: func(t *testing.T, got *Policy) {
				if got.Scoring.Factors.TestSignal.Points(21) != 20 {
					t.Errorf("test_signal = %+v", got.Scoring.Factors.TestSignal)
				}
			},
		},

		// scoring.floors: additive only, so a repo cannot weaken them
		{
			field: "scoring.floors", dir: tighten, name: "floors add to the baseline's",
			org:  "version: 1\nscoring:\n  floors:\n    - {paths: [\"infra/**\"], risk: high}\n",
			repo: "version: 1\nscoring:\n  floors:\n    - {paths: [\"db/migrations/**\"], risk: medium}\n",
			check: func(t *testing.T, got *Policy) {
				if len(got.Scoring.Floors) != 2 || got.Scoring.Floors[0].Paths[0] != "infra/**" || got.Scoring.Floors[1].Paths[0] != "db/migrations/**" {
					t.Errorf("floors = %+v", got.Scoring.Floors)
				}
			},
		},

		// scoring.exclude_paths: excluding more paths from scoring is looser
		{
			field: "scoring.exclude_paths", dir: weaken, name: "exclude more paths",
			repo: "version: 1\nscoring:\n  exclude_paths: [\"**/vendor/**\", \"src/**\"]\n", paths: []string{"/scoring/exclude_paths"},
			check: func(t *testing.T, got *Policy) {
				if slices.Contains(got.Scoring.ExcludePaths, "src/**") || !slices.Contains(got.Scoring.ExcludePaths, "**/vendor/**") {
					t.Errorf("exclude_paths = %v", got.Scoring.ExcludePaths)
				}
			},
		},
		{
			field: "scoring.exclude_paths", dir: tighten, name: "exclude fewer paths",
			repo: "version: 1\nscoring:\n  exclude_paths: [\"**/vendor/**\"]\n",
			check: func(t *testing.T, got *Policy) {
				if !slices.Equal(got.Scoring.ExcludePaths, []string{"**/vendor/**"}) {
					t.Errorf("exclude_paths = %v", got.Scoring.ExcludePaths)
				}
			},
		},

		// ai
		{
			field: "ai.enabled", dir: weaken, name: "disable a required AI",
			org:  "version: 1\nai: {enabled: true, required: true, provider: anthropic}\n",
			repo: "version: 1\nai: {enabled: false}\n", paths: []string{"/ai/enabled"},
			check: func(t *testing.T, got *Policy) {
				if !got.AI.IsEnabled() {
					t.Error("AI was disabled")
				}
			},
		},
		{
			field: "ai.enabled", dir: tighten, name: "enable the AI",
			repo: "version: 1\nai: {enabled: true, provider: anthropic}\n",
			check: func(t *testing.T, got *Policy) {
				if !got.AI.IsEnabled() {
					t.Error("AI was not enabled")
				}
			},
		},
		{
			field: "ai.required", dir: weaken, name: "unset required",
			org:  "version: 1\nai: {enabled: true, required: true}\n",
			repo: "version: 1\nai: {required: false}\n", paths: []string{"/ai/required"},
			check: func(t *testing.T, got *Policy) {
				if !got.AI.IsRequired() {
					t.Error("AI is no longer required")
				}
			},
		},
		{
			field: "ai.required", dir: tighten, name: "require the AI",
			repo: "version: 1\nai: {enabled: true, required: true}\n",
			check: func(t *testing.T, got *Policy) {
				if !got.AI.IsRequired() {
					t.Error("AI is not required")
				}
			},
		},
		{
			field: "ai.provider", dir: weaken, name: "switch the org's provider",
			org:  "version: 1\nai: {enabled: true, provider: bedrock}\n",
			repo: "version: 1\nai: {provider: openai-compatible}\n", paths: []string{"/ai/provider"},
			check: func(t *testing.T, got *Policy) {
				if got.AI.Provider != "bedrock" {
					t.Errorf("provider = %q", got.AI.Provider)
				}
			},
		},
		{
			field: "ai.provider", dir: tighten, name: "choose a provider the org left open",
			repo: "version: 1\nai: {enabled: true, provider: anthropic}\n",
			check: func(t *testing.T, got *Policy) {
				if got.AI.Provider != "anthropic" {
					t.Errorf("provider = %q", got.AI.Provider)
				}
			},
		},
		{
			field: "ai.model", dir: weaken, name: "switch the org's model",
			org:  "version: 1\nai: {enabled: true, model: big-model}\n",
			repo: "version: 1\nai: {model: small-model}\n", paths: []string{"/ai/model"},
			check: func(t *testing.T, got *Policy) {
				if got.AI.Model != "big-model" {
					t.Errorf("model = %q", got.AI.Model)
				}
			},
		},
		{
			field: "ai.model", dir: tighten, name: "choose a model the org left open",
			repo: "version: 1\nai: {enabled: true, model: some-model}\n",
			check: func(t *testing.T, got *Policy) {
				if got.AI.Model != "some-model" {
					t.Errorf("model = %q", got.AI.Model)
				}
			},
		},
		{
			field: "ai.endpoint", dir: weaken, name: "send code to another endpoint",
			org:  "version: 1\nai: {enabled: true, endpoint: \"https://llm.corp.example\"}\n",
			repo: "version: 1\nai: {endpoint: \"https://elsewhere.example\"}\n", paths: []string{"/ai/endpoint"},
			check: func(t *testing.T, got *Policy) {
				if got.AI.Endpoint != "https://llm.corp.example" {
					t.Errorf("endpoint = %q", got.AI.Endpoint)
				}
			},
		},
		{
			field: "ai.endpoint", dir: tighten, name: "same endpoint as the org",
			org:  "version: 1\nai: {enabled: true, endpoint: \"https://llm.corp.example\"}\n",
			repo: "version: 1\nai: {endpoint: \"https://llm.corp.example\"}\n",
			check: func(t *testing.T, got *Policy) {
				if got.AI.Endpoint != "https://llm.corp.example" {
					t.Errorf("endpoint = %q", got.AI.Endpoint)
				}
			},
		},
		{
			field: "ai.max_input_tokens", dir: weaken, name: "raise the input cap",
			org:  "version: 1\nai: {enabled: true, max_input_tokens: 50000}\n",
			repo: "version: 1\nai: {max_input_tokens: 90000}\n", paths: []string{"/ai/max_input_tokens"},
			check: func(t *testing.T, got *Policy) { intsEq(t, "max_input_tokens", &got.AI.MaxInputTokens, 50000) },
		},
		{
			field: "ai.max_input_tokens", dir: tighten, name: "lower the input cap",
			org:   "version: 1\nai: {enabled: true, max_input_tokens: 50000}\n",
			repo:  "version: 1\nai: {max_input_tokens: 20000}\n",
			check: func(t *testing.T, got *Policy) { intsEq(t, "max_input_tokens", &got.AI.MaxInputTokens, 20000) },
		},
		{
			field: "ai.timeout_seconds", dir: weaken, name: "raise the timeout",
			org:  "version: 1\nai: {enabled: true, timeout_seconds: 60}\n",
			repo: "version: 1\nai: {timeout_seconds: 600}\n", paths: []string{"/ai/timeout_seconds"},
			check: func(t *testing.T, got *Policy) { intsEq(t, "timeout_seconds", &got.AI.TimeoutSeconds, 60) },
		},
		{
			field: "ai.timeout_seconds", dir: tighten, name: "lower the timeout",
			org:   "version: 1\nai: {enabled: true, timeout_seconds: 60}\n",
			repo:  "version: 1\nai: {timeout_seconds: 30}\n",
			check: func(t *testing.T, got *Policy) { intsEq(t, "timeout_seconds", &got.AI.TimeoutSeconds, 30) },
		},
	}
}

// For every field: weakening is rejected and reported, tightening is applied
// and not reported (issue #21 acceptance criterion).
func TestMergeEveryField(t *testing.T) {
	for _, tc := range fieldCases() {
		label := map[direction]string{weaken: "weaken", tighten: "tighten"}[tc.dir]
		t.Run(tc.field+"/"+label+"/"+tc.name, func(t *testing.T) {
			got, ws := layered(t, tc.org, tc.repo)
			var paths []string
			for _, w := range ws {
				paths = append(paths, w.Path)
				if w.Message == "" {
					t.Errorf("weakening at %s has no message", w.Path)
				}
			}
			sort.Strings(paths)
			want := slices.Clone(tc.paths)
			sort.Strings(want)
			if !slices.Equal(paths, want) {
				t.Errorf("weakenings at %v, want %v", paths, want)
			}
			tc.check(t, got)
		})
	}
}

// Every policy field in the schema has a weakening and a tightening case, or
// is listed here as unable to do one of them.
func TestEveryFieldHasBothCases(t *testing.T) {
	var s map[string]any
	if err := json.Unmarshal(SchemaJSON(), &s); err != nil {
		t.Fatal(err)
	}
	fields := map[string]bool{}
	var walk func(prefix string, node map[string]any)
	walk = func(prefix string, node map[string]any) {
		props, _ := node["properties"].(map[string]any)
		for name, raw := range props {
			if prefix == "" && (name == "version" || name == "extends") {
				continue
			}
			child := raw.(map[string]any)
			path := name
			if prefix != "" {
				path = prefix + "." + name
			}
			switch {
			case name == "triggers":
				def := s["$defs"].(map[string]any)["trigger"].(map[string]any)
				walk(path, def)
				fields[path+".new"] = true
			case name == "factors":
				walk(path, child)
			case child["properties"] != nil:
				walk(path, child)
			default:
				fields[path] = true
			}
		}
	}
	walk("", s)

	// Additive-only or inherited fields cannot be weakened.
	noWeakening := map[string]bool{
		"scoring.floors":       true,
		"triggers.add_paths":   true,
		"triggers.description": true,
		"triggers.new":         true,
	}
	have := map[string]map[direction]bool{}
	for _, tc := range fieldCases() {
		if have[tc.field] == nil {
			have[tc.field] = map[direction]bool{}
		}
		have[tc.field][tc.dir] = true
		if !fields[tc.field] {
			t.Errorf("case for %s, which is not a schema field", tc.field)
		}
	}
	for f := range fields {
		if !have[f][tighten] {
			t.Errorf("%s has no tightening case", f)
		}
		if !have[f][weaken] && !noWeakening[f] {
			t.Errorf("%s has no weakening case", f)
		}
	}
}

func TestMergeLeavesInputsAlone(t *testing.T) {
	base := Defaults()
	repo := mustParse(t, `
version: 1
limits: {max_changed_lines: 10}
triggers:
  G7.security-paths: {add_paths: ["a/**"], reviewers: [security, payments], risk: high}
  repo.x: {paths: ["x/**"]}
scoring:
  thresholds: {medium: 5, high: 10}
  exclude_paths: ["**/vendor/**"]
ai: {enabled: true, provider: anthropic}
`)
	Merge(base, repo)
	if !reflect.DeepEqual(base, Defaults()) {
		t.Error("Merge changed the base policy")
	}
	merged, _ := Merge(base, repo)
	merged.Triggers["G1.ci-workflows"].Paths[0] = "mutated"
	merged.Scoring.Factors.Size[0].Points = 99
	if !reflect.DeepEqual(base, Defaults()) {
		t.Error("the merged policy shares memory with the base")
	}
}

func TestMergeEmptyRepoChangesNothing(t *testing.T) {
	got, ws := Merge(Defaults(), mustParse(t, "version: 1\n"))
	if len(ws) != 0 {
		t.Errorf("weakenings = %+v", ws)
	}
	if !reflect.DeepEqual(got, Defaults()) {
		t.Error("an empty repo policy changed the baseline")
	}
}

// A repo that weakens one field and tightens another gets the tightening and
// a report for the weakening.
func TestMergeMixed(t *testing.T) {
	got, ws := layered(t, "", `
version: 1
limits: {max_changed_lines: 100, max_changed_files: 1000}
triggers:
  G3.lint-config: {risk: low, reviewers: [platform, lint-owners]}
`)
	intsEq(t, "max_changed_lines", got.Limits.MaxChangedLines, 100)
	intsEq(t, "max_changed_files", got.Limits.MaxChangedFiles, 300)
	tr := trigger(t, got, "G3.lint-config")
	if tr.Risk != RiskMedium || !slices.Equal(tr.Reviewers, []string{"platform", "lint-owners"}) {
		t.Errorf("rule = %+v", tr)
	}
	if len(ws) != 2 {
		t.Errorf("weakenings = %+v", ws)
	}
	for _, w := range ws {
		if strings.HasPrefix(w.Path, "/triggers/") && w.Rule != "G3.lint-config" {
			t.Errorf("Rule = %q for %s", w.Rule, w.Path)
		}
	}
}

func TestOverlayLetsOrgReplaceBaselineValues(t *testing.T) {
	org := mustParse(t, `
version: 1
limits: {max_changed_lines: 9000}
triggers:
  G1.ci-workflows: {reviewers: [acme/platform-eng], add_paths: ["ci/**"]}
  G9.suppressions: {enabled: false}
  org.pci: {paths: ["pci/**"], reviewers: [acme/pci], risk: high}
scoring:
  thresholds: {medium: 25, high: 60}
  exclude_paths: ["**/vendor/**"]
ai: {enabled: true, required: true, provider: bedrock}
`)
	got := Overlay(Defaults(), org)
	intsEq(t, "max_changed_lines", got.Limits.MaxChangedLines, 9000)
	g1 := trigger(t, got, "G1.ci-workflows")
	if !slices.Equal(g1.Reviewers, []string{"acme/platform-eng"}) || !slices.Contains(g1.Paths, "ci/**") || !slices.Contains(g1.Paths, ".github/workflows/**") || len(g1.AddPaths) != 0 {
		t.Errorf("G1 = %+v", g1)
	}
	if trigger(t, got, "G9.suppressions").IsEnabled() {
		t.Error("org could not disable a default rule")
	}
	if trigger(t, got, "org.pci").Risk != RiskHigh {
		t.Error("org rule missing")
	}
	intsEq(t, "medium", &got.Scoring.Thresholds.Medium, 25)
	if !slices.Equal(got.Scoring.ExcludePaths, []string{"**/vendor/**"}) {
		t.Errorf("exclude_paths = %v", got.Scoring.ExcludePaths)
	}
	if !got.AI.IsEnabled() || !got.AI.IsRequired() || got.AI.Provider != "bedrock" {
		t.Errorf("ai = %+v", got.AI)
	}
	// Untouched fields keep the defaults.
	if !reflect.DeepEqual(got.Scoring.Factors, Defaults().Scoring.Factors) {
		t.Error("factors changed")
	}
}

func TestResolve(t *testing.T) {
	t.Run("no org and no repo policy is the defaults", func(t *testing.T) {
		res, err := Resolve(nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(res.Policy, Defaults()) || len(res.Triggers) != 0 || len(res.Weakenings) != 0 {
			t.Errorf("res = %+v", res)
		}
	})

	t.Run("a tightening repo policy fires nothing", func(t *testing.T) {
		res, err := Resolve(nil, []byte("version: 1\nlimits: {max_changed_lines: 10}\n"))
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Triggers) != 0 {
			t.Errorf("triggers = %v", res.Triggers)
		}
		intsEq(t, "max_changed_lines", res.Policy.Limits.MaxChangedLines, 10)
	})

	t.Run("a weakening repo policy fires policy-weakening-attempt and keeps the baseline", func(t *testing.T) {
		res, err := Resolve(nil, []byte("version: 1\ntriggers:\n  G1.ci-workflows: {enabled: false}\n"))
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(res.Triggers, []string{TriggerPolicyWeakeningAttempt}) || len(res.Weakenings) != 1 {
			t.Errorf("res = %+v", res)
		}
		if !trigger(t, res.Policy, "G1.ci-workflows").IsEnabled() {
			t.Error("rule was disabled")
		}
	})

	t.Run("an invalid repo policy falls back to the baseline and fires config-invalid", func(t *testing.T) {
		org := "version: 1\nlimits: {max_changed_lines: 9000}\n"
		for name, repo := range map[string]string{
			"unknown key":         "version: 1\nbogus: 1\n",
			"unsupported version": "version: 2\n",
			"not yaml":            "version: [\n",
			"empty":               "",
		} {
			t.Run(name, func(t *testing.T) {
				res, err := Resolve([]byte(org), []byte(repo))
				if err != nil {
					t.Fatal(err)
				}
				if !slices.Equal(res.Triggers, []string{TriggerConfigInvalid}) || res.ConfigErr == nil {
					t.Errorf("res = %+v", res)
				}
				intsEq(t, "max_changed_lines", res.Policy.Limits.MaxChangedLines, 9000)
				if len(res.Weakenings) != 0 {
					t.Errorf("weakenings = %+v", res.Weakenings)
				}
			})
		}
	})

	t.Run("an absent repo policy is not config-invalid", func(t *testing.T) {
		res, err := Resolve([]byte("version: 1\n"), nil)
		if err != nil || len(res.Triggers) != 0 {
			t.Errorf("res = %+v, err = %v", res, err)
		}
	})

	t.Run("an invalid org baseline is an error, not a fallback", func(t *testing.T) {
		if _, err := Resolve([]byte("version: 1\nbogus: 1\n"), nil); err == nil || !strings.Contains(err.Error(), "org baseline") {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("an org rule without paths is rejected", func(t *testing.T) {
		if _, err := Resolve([]byte("version: 1\ntriggers:\n  org.x: {reviewers: [a]}\n"), nil); err == nil {
			t.Error("a path-less org rule must be rejected")
		}
	})
}
