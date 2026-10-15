package fixture

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLoadParsesFixture(t *testing.T) {
	fixtures, err := Load("testdata/valid")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(fixtures) != 1 {
		t.Fatalf("got %d fixtures, want 1", len(fixtures))
	}
	f := fixtures[0]

	if f.Name != "workflow change requires human review" {
		t.Errorf("Name = %q", f.Name)
	}
	if f.Path != filepath.Join("testdata", "valid", "workflow-change.yaml") {
		t.Errorf("Path = %q", f.Path)
	}
	if len(f.Change.Files) != 2 {
		t.Fatalf("got %d files, want 2", len(f.Change.Files))
	}
	ci := f.Change.Files[0]
	if ci.Path != ".github/workflows/ci.yml" || ci.Status != StatusModified {
		t.Errorf("first file = %+v", ci)
	}
	if got := ci.Hunks[0].Added; !slices.Equal(got, []string{"      continue-on-error: true"}) {
		t.Errorf("added lines = %q", got)
	}
	renamed := f.Change.Files[1]
	if renamed.OldPath != "scripts/test.sh" || renamed.Status != StatusRenamed || renamed.NewMode != "100755" {
		t.Errorf("renamed file = %+v", renamed)
	}
	if f.PR.Author.Type != "bot" || !slices.Equal(f.PR.Labels, []string{"ci"}) {
		t.Errorf("PR = %+v", f.PR)
	}
	want := Expect{
		Risk:           "high",
		HumanReview:    "required",
		Triggers:       []string{"G1.ci-workflows"},
		TriggersAbsent: []string{"G7.security-paths"},
		Reviewers:      []string{"platform"},
	}
	if !expectEqual(f.Expect, want) {
		t.Errorf("Expect = %+v, want %+v", f.Expect, want)
	}
}

func TestLoadRejectsInvalidFixtures(t *testing.T) {
	cases := map[string]struct {
		yaml    string
		wantErr string
	}{
		"unknown field": {
			yaml:    "name: x\nexpect: {risk: low}\nsurprise: true\n",
			wantErr: "surprise",
		},
		"missing name": {
			yaml:    "expect: {risk: low}\n",
			wantErr: "name is required",
		},
		"file without path": {
			yaml:    "name: x\nchange: {files: [{path: a}, {status: added}]}\nexpect: {risk: low}\n",
			wantErr: "change.files[1]: path is required",
		},
		"invalid status": {
			yaml:    "name: x\nchange: {files: [{path: a, status: moved}]}\nexpect: {risk: low}\n",
			wantErr: `invalid status "moved"`,
		},
		"rename without old path": {
			yaml:    "name: x\nchange: {files: [{path: a, status: renamed}]}\nexpect: {risk: low}\n",
			wantErr: "old_path is required",
		},
		"invalid risk": {
			yaml:    "name: x\nexpect: {risk: severe}\n",
			wantErr: `invalid expect.risk "severe"`,
		},
		"invalid human review": {
			yaml:    "name: x\nexpect: {human_review: maybe}\n",
			wantErr: `invalid expect.human_review "maybe"`,
		},
		"empty expectation": {
			yaml:    "name: x\n",
			wantErr: "expect must set at least one field",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "case.yaml"), []byte(tc.yaml), 0o600); err != nil {
				t.Fatal(err)
			}

			_, err := Load(dir)

			if err == nil {
				t.Fatal("Load succeeded, want error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %q, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestLoadIgnoresNonYAMLFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("not a fixture"), 0o600); err != nil {
		t.Fatal(err)
	}

	fixtures, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(fixtures) != 0 {
		t.Fatalf("got %d fixtures, want 0", len(fixtures))
	}
}

func TestCompare(t *testing.T) {
	exp := Expect{
		Risk:           "high",
		HumanReview:    "required",
		Triggers:       []string{"G1.ci-workflows"},
		TriggersAbsent: []string{"G7.security-paths"},
		Reviewers:      []string{"platform"},
	}

	t.Run("match", func(t *testing.T) {
		got := Result{Risk: "high", HumanReview: "required", Triggers: []string{"G1.ci-workflows", "G3.lint-config"}, Reviewers: []string{"platform", "security"}}
		if diffs := Compare(exp, got); len(diffs) != 0 {
			t.Fatalf("diffs = %q, want none", diffs)
		}
	})

	t.Run("mismatch", func(t *testing.T) {
		got := Result{Risk: "low", HumanReview: "optional", Triggers: []string{"G7.security-paths"}, Reviewers: nil}
		want := []string{
			`risk: got "low", want "high"`,
			`human_review: got "optional", want "required"`,
			`missing trigger "G1.ci-workflows"`,
			`unexpected trigger "G7.security-paths"`,
			`missing reviewer "platform"`,
		}
		if diffs := Compare(exp, got); !slices.Equal(diffs, want) {
			t.Fatalf("diffs = %q\nwant  %q", diffs, want)
		}
	})

	t.Run("unset expectations are not checked", func(t *testing.T) {
		if diffs := Compare(Expect{Triggers: []string{"G1.ci-workflows"}}, Result{Risk: "low", Triggers: []string{"G1.ci-workflows"}}); len(diffs) != 0 {
			t.Fatalf("diffs = %q, want none", diffs)
		}
	})
}

func TestRunEvaluatesEveryFixture(t *testing.T) {
	var seen []string

	Run(t, "testdata/valid", func(f Fixture) (Result, error) {
		seen = append(seen, f.Name)
		return Result{Risk: "high", HumanReview: "required", Triggers: []string{"G1.ci-workflows"}, Reviewers: []string{"platform"}}, nil
	})

	if !slices.Equal(seen, []string{"workflow change requires human review"}) {
		t.Fatalf("evaluated %q", seen)
	}
}

func expectEqual(a, b Expect) bool {
	return a.Risk == b.Risk && a.HumanReview == b.HumanReview &&
		slices.Equal(a.Triggers, b.Triggers) && slices.Equal(a.TriggersAbsent, b.TriggersAbsent) &&
		slices.Equal(a.Reviewers, b.Reviewers)
}

// recorder is a testing.TB that records failures. Fatalf stops the calling
// function the way testing.T does, through a panic that try recovers.
type recorder struct {
	testing.TB
	errors []string
	fatal  string
}

type stopped struct{}

func (r *recorder) Helper() {}

func (r *recorder) Errorf(format string, args ...any) {
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

func (r *recorder) Fatalf(format string, args ...any) {
	r.fatal = fmt.Sprintf(format, args...)
	panic(stopped{})
}

func (r *recorder) try(fn func()) {
	defer func() {
		if v := recover(); v != nil {
			if _, ok := v.(stopped); !ok {
				panic(v)
			}
		}
	}()
	fn()
}

func TestMustLoadFailsOnAMissingDir(t *testing.T) {
	r := &recorder{TB: t}
	r.try(func() { mustLoad(r, filepath.Join(t.TempDir(), "missing")) })
	if !strings.HasPrefix(r.fatal, "load fixtures: read fixture dir:") {
		t.Errorf("fatal = %q", r.fatal)
	}
}

func TestCheckReportsEvaluateErrors(t *testing.T) {
	r := &recorder{TB: t}
	f := Fixture{Path: "x.yaml", Expect: Expect{Risk: "low"}}
	r.try(func() { check(r, f, func(Fixture) (Result, error) { return Result{}, errors.New("boom") }) })
	if r.fatal != "x.yaml: evaluate: boom" || len(r.errors) != 0 {
		t.Errorf("fatal = %q, errors = %q", r.fatal, r.errors)
	}
}

func TestCheckReportsEveryDifference(t *testing.T) {
	r := &recorder{TB: t}
	f := Fixture{Path: "x.yaml", Expect: Expect{Risk: "high", HumanReview: "required"}}
	r.try(func() {
		check(r, f, func(Fixture) (Result, error) { return Result{Risk: "low", HumanReview: "optional"}, nil })
	})
	want := []string{`x.yaml: risk: got "low", want "high"`, `x.yaml: human_review: got "optional", want "required"`}
	if r.fatal != "" || !slices.Equal(r.errors, want) {
		t.Errorf("fatal = %q, errors = %q, want %q", r.fatal, r.errors, want)
	}
}

func TestLoadReportsUnreadableFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink(filepath.Join(dir, "nowhere"), filepath.Join(dir, "dangling.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "dangling.yaml") {
		t.Errorf("err = %v, want an error naming dangling.yaml", err)
	}
}

func TestValidateAcceptsAnySingleExpectation(t *testing.T) {
	tests := map[string]Expect{
		"risk":            {Risk: "low"},
		"human_review":    {HumanReview: "required"},
		"triggers":        {Triggers: []string{"G1.ci-workflows"}},
		"triggers_absent": {TriggersAbsent: []string{"G1.ci-workflows"}},
		"reviewers":       {Reviewers: []string{"@org/team"}},
	}
	for name, e := range tests {
		if err := validate(Fixture{Name: name, Expect: e}); err != nil {
			t.Errorf("validate with only %s: %v", name, err)
		}
	}
}
