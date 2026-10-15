package triggers

import (
	"reflect"
	"slices"
	"testing"

	"github.com/zendern/priskinator/internal/changeset"
	"github.com/zendern/priskinator/internal/fixture"
	"github.com/zendern/priskinator/internal/policy"
)

// TestFixtures checks the positive, negative, and rename fixture for every
// path and G10 rule (ADR 0006, Confirmation).
func TestFixtures(t *testing.T) {
	fixture.Run(t, "testdata", func(f fixture.Fixture) (fixture.Result, error) {
		res, err := policy.Resolve(nil, f.PolicyYAML())
		if err != nil {
			return fixture.Result{}, err
		}
		if len(res.Triggers) > 0 {
			t.Fatalf("fixture policy fired %v: %v %v", res.Triggers, res.ConfigErr, res.Weakenings)
		}
		var got fixture.Result
		for _, h := range Evaluate(res.Policy, changeSet(f.Change)) {
			got.Triggers = append(got.Triggers, h.Rule)
			got.Reviewers = append(got.Reviewers, h.Reviewers...)
		}
		return got, nil
	})
}

func ptr[T any](v T) *T { return &v }

func rules(hits []Hit) []string {
	var out []string
	for _, h := range hits {
		out = append(out, h.Rule)
	}
	return out
}

func TestEvaluateReportsRuleSettingsAndFileEvidence(t *testing.T) {
	p := &policy.Policy{Triggers: map[string]*policy.Trigger{
		"G1.ci-workflows": {Paths: []string{".github/workflows/**"}, Reviewers: []string{"platform"}, Risk: policy.RiskHigh},
	}}
	cs := changeset.ChangeSet{Files: []changeset.File{
		{Path: ".github/workflows/ci.yml", Status: changeset.StatusModified},
		{Path: "README.md", Status: changeset.StatusModified},
		{Path: "docs/old.yml", OldPath: ".github/workflows/old.yml", Status: changeset.StatusRenamed},
	}}

	got := Evaluate(p, cs)

	want := []Hit{{
		Rule:      "G1.ci-workflows",
		Risk:      policy.RiskHigh,
		Reviewers: []string{"platform"},
		Files: []Evidence{
			{Path: ".github/workflows/ci.yml", Reason: `matches ".github/workflows/**"`},
			{Path: "docs/old.yml", OldPath: ".github/workflows/old.yml", Reason: `old path matches ".github/workflows/**"`},
		},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Evaluate =\n%+v\nwant\n%+v", got, want)
	}
}

func TestEvaluateReturnsACopyOfReviewers(t *testing.T) {
	p := &policy.Policy{Triggers: map[string]*policy.Trigger{
		"G1.ci-workflows": {Paths: []string{"ci.yml"}, Reviewers: []string{"platform"}},
	}}
	hits := Evaluate(p, changeset.ChangeSet{Files: []changeset.File{{Path: "ci.yml"}}})
	hits[0].Reviewers[0] = "changed"
	if p.Triggers["G1.ci-workflows"].Reviewers[0] != "platform" {
		t.Error("changing a hit's reviewers changed the policy")
	}
}

func TestEvaluateSortsHitsByRule(t *testing.T) {
	p := &policy.Policy{Triggers: map[string]*policy.Trigger{
		"b.rule": {Paths: []string{"x"}},
		"a.rule": {Paths: []string{"x"}},
		"c.rule": {Paths: []string{"x"}},
	}}
	got := rules(Evaluate(p, changeset.ChangeSet{Files: []changeset.File{{Path: "x"}}}))
	if want := []string{"a.rule", "b.rule", "c.rule"}; !slices.Equal(got, want) {
		t.Errorf("rules = %v, want %v", got, want)
	}
}

func TestEvaluateReportsTheFirstMatchingGlob(t *testing.T) {
	p := &policy.Policy{Triggers: map[string]*policy.Trigger{
		"r": {Paths: []string{"nope/**", "a/**", "**"}},
	}}
	hits := Evaluate(p, changeset.ChangeSet{Files: []changeset.File{{Path: "a/b"}}})
	if got := hits[0].Files[0].Reason; got != `matches "a/**"` {
		t.Errorf("reason = %q", got)
	}
}

func TestEvaluateSkipsDisabledRules(t *testing.T) {
	p := &policy.Policy{
		Limits: policy.Limits{MaxFileBytes: ptr(int64(1))},
		Triggers: map[string]*policy.Trigger{
			"G1.ci-workflows":   {Enabled: ptr(false), Paths: []string{"**"}},
			RuleBinary:          {Enabled: ptr(false)},
			RuleOversizeFile:    {Enabled: ptr(false)},
			RuleWatchedPathMove: {Enabled: ptr(false)},
		},
	}
	cs := changeset.ChangeSet{Files: []changeset.File{{Path: "a", Status: changeset.StatusDeleted, Binary: true, NewSize: 5}}}
	if got := Evaluate(p, cs); len(got) != 0 {
		t.Errorf("Evaluate = %+v, want no hits", got)
	}
}

func TestEvaluateSkipsShapeRulesMissingFromPolicy(t *testing.T) {
	cs := changeset.ChangeSet{Files: []changeset.File{{Path: "a", Binary: true, LFS: true}}}
	if got := Evaluate(&policy.Policy{}, cs); len(got) != 0 {
		t.Errorf("Evaluate = %+v, want no hits", got)
	}
}

func TestEvaluateMatchesCaseSensitivelyWhenAsked(t *testing.T) {
	p := &policy.Policy{
		Match:    policy.Match{CaseSensitive: ptr(true)},
		Triggers: map[string]*policy.Trigger{"r": {Paths: []string{"Makefile"}}},
	}
	cs := changeset.ChangeSet{Files: []changeset.File{{Path: "makefile"}}}
	if got := Evaluate(p, cs); len(got) != 0 {
		t.Errorf("case-sensitive: Evaluate = %+v, want no hits", got)
	}
	p.Match.CaseSensitive = ptr(false)
	if got := rules(Evaluate(p, cs)); !slices.Equal(got, []string{"r"}) {
		t.Errorf("case-insensitive: rules = %v", got)
	}
	p.Match.CaseSensitive = nil
	if got := rules(Evaluate(p, cs)); !slices.Equal(got, []string{"r"}) {
		t.Errorf("unset: rules = %v", got)
	}
}

func TestEvaluateTreatsABadGlobAsAMatch(t *testing.T) {
	// Policy validation rejects bad globs; if one gets through, fail closed.
	p := &policy.Policy{Triggers: map[string]*policy.Trigger{"r": {Paths: []string{"a/["}}}}
	hits := Evaluate(p, changeset.ChangeSet{Files: []changeset.File{{Path: "a/b"}}})
	if len(hits) != 1 || hits[0].Files[0].Reason != `invalid glob "a/[" counts as a match` {
		t.Errorf("Evaluate = %+v", hits)
	}
}

func TestShapeEvidence(t *testing.T) {
	all := map[string]*policy.Trigger{}
	for _, id := range []string{RuleSubmodule, RuleSymlink, RuleExecutableMode, RuleBinary, RuleLFS, RuleOversizeFile, RuleWatchedPathMove} {
		all[id] = &policy.Trigger{}
	}
	all["G1.ci-workflows"] = &policy.Trigger{Paths: []string{".github/workflows/**"}}
	all["G6.build-scripts"] = &policy.Trigger{Paths: []string{"**/Makefile"}}
	all["G9.suppressions"] = &policy.Trigger{}
	p := &policy.Policy{Limits: policy.Limits{MaxFileBytes: ptr(int64(10))}, Triggers: all}

	tests := []struct {
		name string
		file changeset.File
		rule string
		want string
	}{
		{"submodule", changeset.File{Path: "lib", OldMode: changeset.ModeSubmodule, NewMode: changeset.ModeSubmodule}, RuleSubmodule, "submodule pointer"},
		{"submodule removed", changeset.File{Path: "lib", Status: changeset.StatusDeleted, OldMode: changeset.ModeSubmodule}, RuleSubmodule, "submodule pointer"},
		{"symlink", changeset.File{Path: "l", NewMode: changeset.ModeSymlink, SymlinkTarget: "/etc/passwd"}, RuleSymlink, `symlink to "/etc/passwd"`},
		{"executable", changeset.File{Path: "s", OldMode: changeset.ModeRegular, NewMode: changeset.ModeExecutable}, RuleExecutableMode, "mode 100644 to 100755"},
		{"binary", changeset.File{Path: "b", Binary: true}, RuleBinary, "binary content"},
		{"lfs", changeset.File{Path: "w", LFS: true}, RuleLFS, "Git LFS pointer"},
		{"oversize", changeset.File{Path: "big", NewSize: 11}, RuleOversizeFile, "11 bytes, over the 10-byte limit"},
		{"deleted", changeset.File{Path: ".github/workflows/ci.yml", Status: changeset.StatusDeleted}, RuleWatchedPathMove, "deleted from G1.ci-workflows"},
		{"moved", changeset.File{Path: "docs/Makefile.txt", OldPath: "Makefile", Status: changeset.StatusRenamed}, RuleWatchedPathMove, "moved out of G6.build-scripts"},
		{
			"moved out of several",
			changeset.File{Path: "docs/x", OldPath: ".github/workflows/Makefile", Status: changeset.StatusRenamed},
			RuleWatchedPathMove, "moved out of G1.ci-workflows, G6.build-scripts",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hits := Evaluate(p, changeset.ChangeSet{Files: []changeset.File{tt.file}})
			i := slices.IndexFunc(hits, func(h Hit) bool { return h.Rule == tt.rule })
			if i < 0 {
				t.Fatalf("%s did not fire; got %v", tt.rule, rules(hits))
			}
			if got := hits[i].Files[0].Reason; got != tt.want {
				t.Errorf("reason = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShapeRulesStayQuietOnPlainFiles(t *testing.T) {
	all := map[string]*policy.Trigger{}
	for _, id := range []string{RuleSubmodule, RuleSymlink, RuleExecutableMode, RuleBinary, RuleLFS, RuleOversizeFile, RuleWatchedPathMove} {
		all[id] = &policy.Trigger{}
	}
	all["G1.ci-workflows"] = &policy.Trigger{Paths: []string{".github/workflows/**"}}
	p := &policy.Policy{Limits: policy.Limits{MaxFileBytes: ptr(int64(10))}, Triggers: all}

	files := []changeset.File{
		{Path: "a.go", OldMode: changeset.ModeRegular, NewMode: changeset.ModeRegular, NewSize: 10},
		{Path: "new.sh", Status: changeset.StatusAdded, NewMode: changeset.ModeExecutable},
		{Path: "old.sh", Status: changeset.StatusModified, OldMode: changeset.ModeExecutable, NewMode: changeset.ModeExecutable},
		{Path: "gone", Status: changeset.StatusDeleted, OldMode: changeset.ModeSymlink},
		{Path: "docs/old.md", Status: changeset.StatusDeleted},
		{Path: "docs/ci.yml", OldPath: ".github/workflows/ci.yml", Status: changeset.StatusCopied},
		{Path: ".github/workflows/b.yml", OldPath: ".github/workflows/a.yml", Status: changeset.StatusRenamed},
	}
	for _, f := range files {
		hits := Evaluate(p, changeset.ChangeSet{Files: []changeset.File{f}})
		for _, h := range hits {
			if h.Rule != "G1.ci-workflows" {
				t.Errorf("%s: %s fired: %+v", f.Path, h.Rule, h.Files)
			}
		}
	}
}

func TestOversizeNeedsALimit(t *testing.T) {
	p := &policy.Policy{Triggers: map[string]*policy.Trigger{RuleOversizeFile: {}}}
	cs := changeset.ChangeSet{Files: []changeset.File{{Path: "big", NewSize: 1 << 40}}}
	if got := Evaluate(p, cs); len(got) != 0 {
		t.Errorf("Evaluate = %+v, want no hits without a limit", got)
	}
}

func TestEvaluateFiresOncePerFileWhenBothPathsMatch(t *testing.T) {
	p := &policy.Policy{Triggers: map[string]*policy.Trigger{"r": {Paths: []string{"ci/**"}}}}
	cs := changeset.ChangeSet{Files: []changeset.File{{Path: "ci/b", OldPath: "ci/a", Status: changeset.StatusRenamed}}}
	hits := Evaluate(p, cs)
	if len(hits) != 1 || len(hits[0].Files) != 1 || hits[0].Files[0].Reason != `matches "ci/**"` {
		t.Errorf("Evaluate = %+v, want one hit with one file on the new path", hits)
	}
}

func TestMoveChecksEveryRuleAfterOneStillMatches(t *testing.T) {
	p := &policy.Policy{Triggers: map[string]*policy.Trigger{
		"a.everything":      {Paths: []string{"**"}},
		"b.old":             {Paths: []string{"old/**"}},
		RuleWatchedPathMove: {},
	}}
	cs := changeset.ChangeSet{Files: []changeset.File{{Path: "new/x", OldPath: "old/x", Status: changeset.StatusRenamed}}}
	hits := Evaluate(p, cs)
	i := slices.IndexFunc(hits, func(h Hit) bool { return h.Rule == RuleWatchedPathMove })
	if i < 0 || hits[i].Files[0].Reason != "moved out of b.old" {
		t.Errorf("Evaluate = %+v, want a move out of b.old", hits)
	}
}
