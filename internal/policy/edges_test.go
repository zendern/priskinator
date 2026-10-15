package policy

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestParseRejectsValuesJSONCannotHold(t *testing.T) {
	tests := []struct {
		name, yaml, want string
	}{
		{"non-string key", "version: 1\n1: x\n", "/: key 1 is not a string"},
		{"nested non-string key", "version: 1\nmatch: {1: x}\n", "/match: key 1 is not a string"},
		{"non-string key in a list", "version: 1\nscoring:\n  floors: [{2: x}]\n", "/scoring/floors/0: key 2 is not a string"},
		{"timestamp", "version: 1\nscoring: {exclude_paths: [2001-12-14]}\n", "/scoring/exclude_paths/0: unsupported value of type time.Time"},
		{"infinity", "version: 1\nlimits: {max_file_bytes: .inf}\n", "parse policy"},
		{"duplicate key", "version: 1\nversion: 1\n", "parse policy"},
		{"empty rule", "version: 1\ntriggers:\n  G1.ci-workflows: ~\n", "/triggers/G1.ci-workflows: null"},
		{"new rule with add_paths", "version: 1\ntriggers:\n  repo.x: {paths: [a], add_paths: [b]}\n", "/triggers/repo.x/add_paths: add_paths extends a baseline rule"},
		{"integer too large for the policy type", "version: 1\nlimits: {max_file_bytes: 99999999999999999999}\n", "decode policy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.yaml))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Parse error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestToJSONValue(t *testing.T) {
	got, err := toJSONValue(map[any]any{"a": []any{1, map[any]any{"b": "c"}}}, "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"a":[1,{"b":"c"}]}`; string(b) != want {
		t.Errorf("toJSONValue = %s, want %s", b, want)
	}

	_, err = toJSONValue(map[any]any{"a": map[any]any{"b": time.Time{}}}, "")
	var perr *Error
	if !errors.As(err, &perr) || perr.Problems[0].Path != "/a/b" {
		t.Errorf("error = %v, want an unsupported value at /a/b", err)
	}
}

func TestJSONType(t *testing.T) {
	tests := map[string]any{
		"array":   []any{},
		"string":  "x",
		"boolean": true,
		"number":  json.Number("1"),
		"<nil>":   nil,
	}
	for want, v := range tests {
		if got := jsonType(v); got != want {
			t.Errorf("jsonType(%#v) = %q, want %q", v, got, want)
		}
	}
}

func TestParseTopLevelNull(t *testing.T) {
	if _, err := Parse([]byte("~\n")); err == nil || err.Error() != "empty policy" {
		t.Errorf("err = %v, want empty policy", err)
	}
}

func TestOverlayAppliesOrgDescriptionAndBands(t *testing.T) {
	org := mustParse(t, "version: 1\n"+
		"triggers:\n  G1.ci-workflows: {description: org pipelines}\n"+
		"scoring:\n  factors:\n    files: [{up_to: 3, points: 0}, {points: 7}]\n")
	got := Overlay(Defaults(), org)
	if d := trigger(t, got, "G1.ci-workflows").Description; d != "org pipelines" {
		t.Errorf("description = %q", d)
	}
	f := got.Scoring.Factors.Files
	if len(f) != 2 || f[1].Points != 7 || f[0].UpTo == nil || *f[0].UpTo != 3 {
		t.Errorf("files bands = %+v", f)
	}
	if d := trigger(t, Defaults(), "G1.ci-workflows").Description; d == "org pipelines" {
		t.Error("Overlay changed the defaults")
	}
}

func TestMergeThresholdsOverABaselineWithout(t *testing.T) {
	base := mustParse(t, "version: 1\n")
	got, ws := Merge(base, mustParse(t, "version: 1\nscoring:\n  thresholds: {medium: 5, high: 9}\n"))
	if len(ws) != 0 {
		t.Errorf("weakenings = %+v", ws)
	}
	if th := got.Scoring.Thresholds; th == nil || th.Medium != 5 || th.High != 9 {
		t.Errorf("thresholds = %+v", th)
	}
	if base.Scoring.Thresholds != nil {
		t.Error("Merge changed the baseline")
	}
}

func TestMergeFillsAMissingRuleDescription(t *testing.T) {
	base := mustParse(t, "version: 1\ntriggers:\n  G1.ci-workflows: {paths: [a/**]}\n")
	got, ws := Merge(base, mustParse(t, "version: 1\ntriggers:\n  G1.ci-workflows: {description: from repo}\n"))
	if len(ws) != 0 {
		t.Errorf("weakenings = %+v", ws)
	}
	if d := trigger(t, got, "G1.ci-workflows").Description; d != "from repo" {
		t.Errorf("description = %q, want the repo's", d)
	}

	base = mustParse(t, "version: 1\ntriggers:\n  G1.ci-workflows: {description: from org, paths: [a/**]}\n")
	got, _ = Merge(base, mustParse(t, "version: 1\ntriggers:\n  G1.ci-workflows: {description: from repo}\n"))
	if d := trigger(t, got, "G1.ci-workflows").Description; d != "from org" {
		t.Errorf("description = %q, want the baseline's", d)
	}
}

func TestMergeAICapOverAnUncappedBaseline(t *testing.T) {
	got, ws := Merge(Defaults(), mustParse(t, "version: 1\nai: {max_input_tokens: 1000, timeout_seconds: 30}\n"))
	if len(ws) != 0 {
		t.Errorf("weakenings = %+v", ws)
	}
	if got.AI.MaxInputTokens != 1000 || got.AI.TimeoutSeconds != 30 {
		t.Errorf("ai = %+v", got.AI)
	}
}

func TestCloneKeepsUnsetBandsNil(t *testing.T) {
	p := &Policy{}
	p.Scoring.Factors.Size = Bands{{Points: 1}}
	got := clone(p)
	if got.Scoring.Factors.Files != nil {
		t.Errorf("files = %#v, want nil", got.Scoring.Factors.Files)
	}
	if len(got.Scoring.Factors.Size) != 1 {
		t.Errorf("size = %#v", got.Scoring.Factors.Size)
	}
	got.Scoring.Factors.Size[0].Points = 9
	if p.Scoring.Factors.Size[0].Points != 1 {
		t.Error("clone shares bands with its input")
	}
}

func TestCheckReportsAnEmptyRule(t *testing.T) {
	// The schema rejects null rules before check runs; check still guards
	// policies built in code, such as a merged result.
	got := check(&Policy{Triggers: map[string]*Trigger{"G1.ci-workflows": nil}})
	if len(got) != 1 || got[0].Path != "/triggers/G1.ci-workflows" || got[0].Message != "rule G1.ci-workflows is empty" {
		t.Errorf("problems = %+v", got)
	}
}

func TestMergeAcceptsAnEqualRiskFloor(t *testing.T) {
	base := mustParse(t, "version: 1\ntriggers:\n  G1.ci-workflows: {risk: medium}\n")
	got, ws := Merge(base, mustParse(t, "version: 1\ntriggers:\n  G1.ci-workflows: {risk: medium}\n"))
	if len(ws) != 0 {
		t.Errorf("weakenings = %+v, want none for an equal risk", ws)
	}
	if r := trigger(t, got, "G1.ci-workflows").Risk; r != RiskMedium {
		t.Errorf("risk = %q", r)
	}
}

func TestFloorsAreAddedOnce(t *testing.T) {
	const floors = "version: 1\nscoring:\n  floors:\n" +
		"    - {paths: [db/**], risk: high}\n" +
		"    - {paths: [api/**], risk: medium}\n"
	base := mustParse(t, "version: 1\nscoring:\n  floors:\n    - {paths: [db/**], risk: high}\n")
	for name, got := range map[string]*Policy{
		"overlay": Overlay(base, mustParse(t, floors)),
		"merge":   func() *Policy { p, _ := Merge(base, mustParse(t, floors)); return p }(),
	} {
		t.Run(name, func(t *testing.T) {
			fl := got.Scoring.Floors
			if len(fl) != 2 || fl[0].Paths[0] != "db/**" || fl[1].Paths[0] != "api/**" || fl[1].Risk != RiskMedium {
				t.Errorf("floors = %+v, want db/** once then api/**", fl)
			}
		})
	}
}

func TestCloneCopiesBoolPointers(t *testing.T) {
	p := &Policy{}
	p.Match.CaseSensitive = addr(true)
	p.AI.Enabled = addr(true)
	got := clone(p)
	if !isTrue(got.Match.CaseSensitive) || !isTrue(got.AI.Enabled) || got.AI.Required != nil {
		t.Fatalf("clone = %+v %+v", got.Match, got.AI)
	}
	if got.Match.CaseSensitive == p.Match.CaseSensitive {
		t.Error("clone shares a pointer with its input")
	}
}

func TestProblemsAtOnePathKeepTheirOrder(t *testing.T) {
	_, err := Parse([]byte("version: 1\nscoring:\n  factors:\n    size: [{points: 5}, {up_to: 3, points: 1}]\n"))
	want := "invalid policy: " +
		"/scoring/factors/size/0: only the last band may omit up_to; " +
		"/scoring/factors/size/1: the last band must omit up_to so every value has points; " +
		"/scoring/factors/size/1: points must not decrease as values grow (1 after 5)"
	if err == nil || err.Error() != want {
		t.Errorf("err = %v\nwant  %s", err, want)
	}
}

func TestSchemaProblemsAtOnePathKeepTheirOrder(t *testing.T) {
	_, err := Parse([]byte("a: 1\n"))
	want := "invalid policy: /: missing property 'version'; /: additional properties 'a' not allowed"
	if err == nil || err.Error() != want {
		t.Errorf("err = %v\nwant  %s", err, want)
	}
}

func TestParseNamesTheBadRuleID(t *testing.T) {
	_, err := Parse([]byte("version: 1\ntriggers:\n  bad: {paths: [a]}\n"))
	want := `invalid policy: /triggers: invalid propertyName 'bad': 'bad' does not match pattern '^(G([1-9]|10)|org|repo)\\.[a-z0-9]+(-[a-z0-9]+)*$'`
	if err == nil || err.Error() != want {
		t.Errorf("err = %v\nwant  %s", err, want)
	}
}

// Restating what the baseline already says, or relaxing a setting the
// baseline leaves loose, is not a weakening.
func TestMergeRestatingTheBaselineIsNotAWeakening(t *testing.T) {
	tests := []struct {
		name, org, repo string
		check           func(t *testing.T, got *Policy)
	}{
		{
			name: "case-sensitive over a case-sensitive baseline",
			org:  "version: 1\nmatch: {case_sensitive: true}\n",
			repo: "version: 1\nmatch: {case_sensitive: true}\n",
			check: func(t *testing.T, got *Policy) {
				if !isTrue(got.Match.CaseSensitive) {
					t.Error("matching is no longer case-sensitive")
				}
			},
		},
		{
			name: "case-insensitive over the default",
			org:  "version: 1\n",
			repo: "version: 1\nmatch: {case_sensitive: false}\n",
			check: func(t *testing.T, got *Policy) {
				if c := got.Match.CaseSensitive; c == nil || *c {
					t.Errorf("case_sensitive = %v, want false", c)
				}
			},
		},
		{
			name: "enable an enabled rule",
			org:  "version: 1\n",
			repo: "version: 1\ntriggers:\n  G1.ci-workflows: {enabled: true}\n",
			check: func(t *testing.T, got *Policy) {
				if !trigger(t, got, "G1.ci-workflows").IsEnabled() {
					t.Error("rule is disabled")
				}
			},
		},
		{
			name: "disable a rule the org disabled",
			org:  "version: 1\ntriggers:\n  G1.ci-workflows: {enabled: false}\n",
			repo: "version: 1\ntriggers:\n  G1.ci-workflows: {enabled: false}\n",
			check: func(t *testing.T, got *Policy) {
				if trigger(t, got, "G1.ci-workflows").IsEnabled() {
					t.Error("rule is enabled")
				}
			},
		},
		{
			name: "not require an optional AI",
			org:  "version: 1\nai: {enabled: true}\n",
			repo: "version: 1\nai: {required: false}\n",
			check: func(t *testing.T, got *Policy) {
				if got.AI.IsRequired() {
					t.Error("AI is required")
				}
			},
		},
		{
			name: "enable a required AI",
			org:  "version: 1\nai: {enabled: true, required: true}\n",
			repo: "version: 1\nai: {enabled: true}\n",
			check: func(t *testing.T, got *Policy) {
				if !got.AI.IsEnabled() || !got.AI.IsRequired() {
					t.Errorf("ai = %+v", got.AI)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ws := Merge(Overlay(Defaults(), mustParse(t, tt.org)), mustParse(t, tt.repo))
			if len(ws) != 0 {
				t.Errorf("weakenings = %+v, want none", ws)
			}
			tt.check(t, got)
		})
	}
}

func TestMergeKeepsGoingAfterARuleOrFactor(t *testing.T) {
	got, ws := Merge(Defaults(), mustParse(t, "version: 1\n"+
		"triggers:\n  repo.a: {paths: [a/**]}\n  repo.b: {paths: [b/**]}\n"+
		"scoring:\n  factors:\n    size: [{points: 0}]\n    files: [{points: 99}]\n"))
	if len(ws) != 1 || ws[0].Path != "/scoring/factors/size" {
		t.Errorf("weakenings = %+v, want only the size bands", ws)
	}
	for _, id := range []string{"repo.a", "repo.b"} {
		trigger(t, got, id)
	}
	if f := got.Scoring.Factors.Files; len(f) != 1 || f[0].Points != 99 {
		t.Errorf("files bands = %+v, want the repo's", f)
	}
}

func TestFloorsWithTheSameRiskAreDistinct(t *testing.T) {
	base := mustParse(t, "version: 1\nscoring:\n  floors:\n    - {paths: [db/**], risk: high}\n")
	got := Overlay(base, mustParse(t, "version: 1\nscoring:\n  floors:\n    - {paths: [api/**], risk: high}\n"))
	if fl := got.Scoring.Floors; len(fl) != 2 || fl[1].Paths[0] != "api/**" {
		t.Errorf("floors = %+v, want db/** and api/**", fl)
	}
}

func TestCheckReportsEveryRuleAndGlob(t *testing.T) {
	got := check(&Policy{Triggers: map[string]*Trigger{
		"G1.ci-workflows": nil,
		"G2.secrets":      {Paths: []string{`a\b`, "/c"}},
	}})
	var paths []string
	for _, p := range got {
		paths = append(paths, p.Path)
	}
	want := []string{"/triggers/G1.ci-workflows", "/triggers/G2.secrets/paths/0", "/triggers/G2.secrets/paths/1"}
	if !slices.Equal(paths, want) {
		t.Errorf("problem paths = %q, want %q", paths, want)
	}
}
