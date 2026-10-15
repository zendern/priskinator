// Package fixture loads golden fixtures and compares evaluation results against them.
// Unit tests and `riskinator test` share this harness (ADR 0010).
package fixture

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// File change statuses, matching git's diff statuses.
const (
	StatusAdded       = "added"
	StatusModified    = "modified"
	StatusDeleted     = "deleted"
	StatusRenamed     = "renamed"
	StatusCopied      = "copied"
	StatusTypeChanged = "type_changed"
)

var (
	validStatuses     = []string{StatusAdded, StatusModified, StatusDeleted, StatusRenamed, StatusCopied, StatusTypeChanged}
	validRisks        = []string{"low", "medium", "high"}
	validHumanReviews = []string{"required", "optional"}
)

// Fixture is one synthetic pull request and the verdict expected for it.
type Fixture struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Change      Change         `yaml:"change"`
	PR          PR             `yaml:"pr"`
	Policy      map[string]any `yaml:"policy"`
	Expect      Expect         `yaml:"expect"`

	// Path is the file the fixture was loaded from.
	Path string `yaml:"-"`
}

type Change struct {
	Files []File `yaml:"files"`
}

type File struct {
	Path    string `yaml:"path"`
	OldPath string `yaml:"old_path"`
	Status  string `yaml:"status"`
	OldMode string `yaml:"old_mode"`
	NewMode string `yaml:"new_mode"`
	Binary  bool   `yaml:"binary"`
	Size    int64  `yaml:"size"`
	Hunks   []Hunk `yaml:"hunks"`
}

type Hunk struct {
	Added   []string `yaml:"added"`
	Removed []string `yaml:"removed"`
}

type PR struct {
	Title  string   `yaml:"title"`
	Body   string   `yaml:"body"`
	Author Author   `yaml:"author"`
	Labels []string `yaml:"labels"`
}

type Author struct {
	Login string `yaml:"login"`
	Type  string `yaml:"type"`
}

// Expect lists what must hold for the evaluation result. Unset fields are not
// checked, and Triggers and Reviewers must be present but need not be exhaustive,
// so fixtures stay stable as rules are added.
type Expect struct {
	Risk           string   `yaml:"risk"`
	HumanReview    string   `yaml:"human_review"`
	Triggers       []string `yaml:"triggers"`
	TriggersAbsent []string `yaml:"triggers_absent"`
	Reviewers      []string `yaml:"reviewers"`
}

// Result is the part of an evaluation that fixtures assert on.
type Result struct {
	Risk        string
	HumanReview string
	Triggers    []string
	Reviewers   []string
}

// Load reads every *.yaml and *.yml file in dir, in name order, rejecting unknown
// fields and invalid values.
func Load(dir string) ([]Fixture, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read fixture dir: %w", err)
	}
	var names []string
	for _, e := range entries {
		ext := filepath.Ext(e.Name())
		if !e.IsDir() && (ext == ".yaml" || ext == ".yml") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	fixtures := make([]Fixture, 0, len(names))
	for _, name := range names {
		path := filepath.Join(dir, name)
		f, err := loadFile(path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		fixtures = append(fixtures, f)
	}
	return fixtures, nil
}

func loadFile(path string) (Fixture, error) {
	data, err := os.ReadFile(path) //nolint:gosec // fixture paths come from a directory listing chosen by the caller
	if err != nil {
		return Fixture{}, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var f Fixture
	if err := dec.Decode(&f); err != nil && !errors.Is(err, io.EOF) {
		return Fixture{}, err
	}
	f.Path = path
	return f, validate(f)
}

func validate(f Fixture) error {
	if f.Name == "" {
		return errors.New("name is required")
	}
	for i, file := range f.Change.Files {
		if file.Path == "" {
			return fmt.Errorf("change.files[%d]: path is required", i)
		}
		if file.Status != "" && !slices.Contains(validStatuses, file.Status) {
			return fmt.Errorf("change.files[%d]: invalid status %q (want one of %s)", i, file.Status, strings.Join(validStatuses, ", "))
		}
		if (file.Status == StatusRenamed || file.Status == StatusCopied) && file.OldPath == "" {
			return fmt.Errorf("change.files[%d]: old_path is required for status %q", i, file.Status)
		}
	}
	e := f.Expect
	if e.Risk != "" && !slices.Contains(validRisks, e.Risk) {
		return fmt.Errorf("invalid expect.risk %q (want one of %s)", e.Risk, strings.Join(validRisks, ", "))
	}
	if e.HumanReview != "" && !slices.Contains(validHumanReviews, e.HumanReview) {
		return fmt.Errorf("invalid expect.human_review %q (want one of %s)", e.HumanReview, strings.Join(validHumanReviews, ", "))
	}
	if e.Risk == "" && e.HumanReview == "" && len(e.Triggers) == 0 && len(e.TriggersAbsent) == 0 && len(e.Reviewers) == 0 {
		return errors.New("expect must set at least one field")
	}
	return nil
}

// Compare returns one message per expectation the result does not meet.
func Compare(exp Expect, got Result) []string {
	var diffs []string
	if exp.Risk != "" && got.Risk != exp.Risk {
		diffs = append(diffs, fmt.Sprintf("risk: got %q, want %q", got.Risk, exp.Risk))
	}
	if exp.HumanReview != "" && got.HumanReview != exp.HumanReview {
		diffs = append(diffs, fmt.Sprintf("human_review: got %q, want %q", got.HumanReview, exp.HumanReview))
	}
	for _, t := range exp.Triggers {
		if !slices.Contains(got.Triggers, t) {
			diffs = append(diffs, fmt.Sprintf("missing trigger %q", t))
		}
	}
	for _, t := range exp.TriggersAbsent {
		if slices.Contains(got.Triggers, t) {
			diffs = append(diffs, fmt.Sprintf("unexpected trigger %q", t))
		}
	}
	for _, r := range exp.Reviewers {
		if !slices.Contains(got.Reviewers, r) {
			diffs = append(diffs, fmt.Sprintf("missing reviewer %q", r))
		}
	}
	return diffs
}

// Run loads the fixtures in dir and checks eval against each one as a subtest.
func Run(t *testing.T, dir string, eval func(Fixture) (Result, error)) {
	t.Helper()
	fixtures, err := Load(dir)
	if err != nil {
		t.Fatalf("load fixtures: %v", err)
	}
	for _, f := range fixtures {
		t.Run(f.Name, func(t *testing.T) {
			got, err := eval(f)
			if err != nil {
				t.Fatalf("%s: evaluate: %v", f.Path, err)
			}
			for _, d := range Compare(f.Expect, got) {
				t.Errorf("%s: %s", f.Path, d)
			}
		})
	}
}
