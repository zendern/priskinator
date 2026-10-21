package scoring

import (
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/zendern/priskinator/internal/changeset"
	"github.com/zendern/priskinator/internal/fixture"
	"github.com/zendern/priskinator/internal/fixture/changes"
	"github.com/zendern/priskinator/internal/policy"
)

// TestFixtures checks each factor's bands and the tier and floor outcomes
// (ADR 0007, Confirmation). Risk here is the scoring risk alone, before
// triggers.
func TestFixtures(t *testing.T) {
	fixture.Run(t, "testdata", func(f fixture.Fixture) (fixture.Result, error) {
		res, err := policy.Resolve(nil, f.PolicyYAML())
		if err != nil {
			return fixture.Result{}, err
		}
		if len(res.Triggers) > 0 {
			return fixture.Result{}, fmt.Errorf("fixture policy fired %v: %w %v", res.Triggers, res.ConfigErr, res.Weakenings)
		}
		r := Score(res.Policy, changes.Build(f.Change), Generated{})
		got := fixture.Result{Risk: string(r.Risk), Points: map[string]int{}}
		for _, fa := range r.Factors {
			got.Points[fa.Name] = fa.Points
		}
		return got, nil
	})
}

func ptr[T any](v T) *T { return &v }

func lines(n int) []changeset.Line {
	out := make([]changeset.Line, n)
	for i := range out {
		out[i] = changeset.Line{Line: i + 1, Text: "x"}
	}
	return out
}

func file(path string, added, removed int) changeset.File {
	return changeset.File{Path: path, Status: changeset.StatusModified, Hunks: []changeset.Hunk{{Added: lines(added), Removed: lines(removed)}}}
}

func bands(points ...int) policy.Bands {
	// bands(0, 5, 10) is up_to 10: 0, up_to 20: 5, then 10.
	out := make(policy.Bands, len(points))
	for i, p := range points {
		out[i].Points = p
		if i < len(points)-1 {
			out[i].UpTo = ptr((i + 1) * 10)
		}
	}
	return out
}

func basePolicy() *policy.Policy {
	return &policy.Policy{Scoring: policy.Scoring{
		Thresholds: &policy.Thresholds{Medium: 10, High: 30},
		Factors: policy.Factors{
			Size:       bands(0, 5, 10),
			Files:      bands(0, 5, 10),
			Diffusion:  bands(0, 5, 10),
			TestSignal: bands(0, 5, 10),
		},
	}}
}

func factor(t *testing.T, r Result, name string) Factor {
	t.Helper()
	i := slices.IndexFunc(r.Factors, func(f Factor) bool { return f.Name == name })
	if i < 0 {
		t.Fatalf("no factor %q in %+v", name, r.Factors)
	}
	return r.Factors[i]
}

func TestScoreReportsEveryFactor(t *testing.T) {
	cs := changeset.ChangeSet{Files: []changeset.File{
		file("a/x.go", 12, 3),
		file("b/y.go", 4, 1),
		file("go.sum", 50, 0),
	}}
	p := basePolicy()
	p.Scoring.ExcludePaths = []string{"go.sum"}

	r := Score(p, cs, Generated{})

	want := []Factor{
		{Name: FactorSize, Measured: true, Value: 20, Points: 5, Band: "11-20", Detail: "16 added and 4 removed lines in 2 files; 1 excluded"},
		{Name: FactorFiles, Measured: true, Value: 2, Points: 0, Band: "0-10", Detail: "2 files changed; 1 excluded"},
		{Name: FactorDiffusion, Measured: true, Value: 2, Points: 0, Band: "0-10", Detail: "2 directories; change entropy 0.81"},
		{Name: FactorComplexityDelta, Detail: "not measured: complexity analysis is not available yet"},
		{Name: FactorTestSignal, Measured: true, Value: 20, Points: 5, Band: "11-20", Detail: "20 source lines changed with no test change"},
	}
	if !reflect.DeepEqual(r.Factors, want) {
		t.Errorf("Factors =\n%+v\nwant\n%+v", r.Factors, want)
	}
	if r.Points != 10 || r.Tier != policy.RiskMedium || r.Risk != policy.RiskMedium {
		t.Errorf("Points %d, Tier %s, Risk %s; want 10, medium, medium", r.Points, r.Tier, r.Risk)
	}
	if !slices.Equal(r.Excluded, []string{"go.sum"}) {
		t.Errorf("Excluded = %v", r.Excluded)
	}
}

func TestTierThresholds(t *testing.T) {
	tests := []struct {
		lines int
		want  policy.Risk
	}{
		// basePolicy: size and test signal both score these lines, files and
		// diffusion stay at 0.
		{10, policy.RiskLow},     // 0 points
		{11, policy.RiskMedium},  // 5 + 5 = 10, exactly medium
		{21, policy.RiskMedium},  // 10 + 10 = 20
		{300, policy.RiskMedium}, // still 20; the last band caps it
	}
	for _, tt := range tests {
		r := Score(basePolicy(), changeset.ChangeSet{Files: []changeset.File{file("a.go", tt.lines, 0)}}, Generated{})
		if r.Tier != tt.want {
			t.Errorf("%d lines: %d points, tier %s, want %s", tt.lines, r.Points, r.Tier, tt.want)
		}
	}

	p := basePolicy()
	p.Scoring.Thresholds = &policy.Thresholds{Medium: 5, High: 20}
	if r := Score(p, changeset.ChangeSet{Files: []changeset.File{file("a.go", 21, 0)}}, Generated{}); r.Tier != policy.RiskHigh {
		t.Errorf("20 points with high at 20: tier %s, want high", r.Tier)
	}
	if r := Score(p, changeset.ChangeSet{Files: []changeset.File{file("a.go", 11, 0)}}, Generated{}); r.Tier != policy.RiskMedium {
		t.Errorf("10 points with medium at 5: tier %s, want medium", r.Tier)
	}
}

func TestMissingThresholdsFailClosed(t *testing.T) {
	p := basePolicy()
	p.Scoring.Thresholds = nil
	r := Score(p, changeset.ChangeSet{}, Generated{})
	if r.Tier != policy.RiskHigh || r.Risk != policy.RiskHigh {
		t.Errorf("Tier %s, Risk %s; want high", r.Tier, r.Risk)
	}
	if !slices.Equal(r.Notes, []string{"policy sets no scoring thresholds, so the tier is high"}) {
		t.Errorf("Notes = %q", r.Notes)
	}
}

func TestBandLabels(t *testing.T) {
	b := policy.Bands{{UpTo: ptr(50), Points: 0}, {UpTo: ptr(200), Points: 5}, {Points: 40}}
	tests := []struct {
		value  int
		points int
		label  string
	}{
		{0, 0, "0-50"},
		{50, 0, "0-50"},
		{51, 5, "51-200"},
		{200, 5, "51-200"},
		{201, 40, ">200"},
	}
	for _, tt := range tests {
		if points, label := band(b, tt.value); points != tt.points || label != tt.label {
			t.Errorf("band(%d) = %d, %q; want %d, %q", tt.value, points, label, tt.points, tt.label)
		}
	}
	if points, label := band(policy.Bands{{Points: 3}}, 7); points != 3 || label != "any" {
		t.Errorf("single open band = %d, %q", points, label)
	}
	if points, label := band(nil, 7); points != 0 || label != "no bands" {
		t.Errorf("no bands = %d, %q", points, label)
	}
	if points, label := band(policy.Bands{{UpTo: ptr(5), Points: 1}}, 9); points != 0 || label != "above 5, no band" {
		t.Errorf("past the last closed band = %d, %q", points, label)
	}
}

func TestFloors(t *testing.T) {
	p := basePolicy()
	p.Scoring.Floors = []policy.Floor{
		{Description: "Migrations", Paths: []string{"db/migrations/**"}, Risk: policy.RiskHigh},
		{Description: "Infra", Paths: []string{"infra/**", "deploy/**"}, Risk: policy.RiskMedium},
		{Description: "Unused", Paths: []string{"nothing/**"}, Risk: policy.RiskHigh},
	}
	cs := changeset.ChangeSet{Files: []changeset.File{
		file("DB/Migrations/1.sql", 1, 0),
		file("deploy/app.yaml", 1, 0),
		{Path: "docs/old.tf", OldPath: "infra/old.tf", Status: changeset.StatusRenamed},
	}}

	r := Score(p, cs, Generated{})

	want := []FloorHit{
		{Description: "Migrations", Risk: policy.RiskHigh, Paths: []string{"DB/Migrations/1.sql"}},
		{Description: "Infra", Risk: policy.RiskMedium, Paths: []string{"deploy/app.yaml", "docs/old.tf"}},
	}
	if !reflect.DeepEqual(r.Floors, want) {
		t.Errorf("Floors =\n%+v\nwant\n%+v", r.Floors, want)
	}
	if r.Tier != policy.RiskLow || r.Risk != policy.RiskHigh {
		t.Errorf("Tier %s, Risk %s; want low, high", r.Tier, r.Risk)
	}

	p.Match.CaseSensitive = ptr(true)
	r = Score(p, cs, Generated{})
	if len(r.Floors) != 1 || r.Floors[0].Description != "Infra" || r.Risk != policy.RiskMedium {
		t.Errorf("case-sensitive: Floors %+v, Risk %s", r.Floors, r.Risk)
	}
}

func TestFloorWithABadGlobFailsClosed(t *testing.T) {
	p := basePolicy()
	p.Scoring.Floors = []policy.Floor{{Description: "Bad", Paths: []string{"a/["}, Risk: policy.RiskHigh}}
	r := Score(p, changeset.ChangeSet{Files: []changeset.File{file("a/b", 1, 0)}}, Generated{})
	if r.Risk != policy.RiskHigh {
		t.Errorf("Risk %s, want high", r.Risk)
	}
}

func TestExclusions(t *testing.T) {
	p := basePolicy()
	p.Scoring.ExcludePaths = []string{"**/vendor/**", "bad/["}
	gen := ParseAttributes([]byte("*.pb.go linguist-generated\n"))
	cs := changeset.ChangeSet{Files: []changeset.File{
		file("vendor/lib/a.go", 100, 0),
		file("api/v1/api.pb.go", 100, 0),
		file("Vendor/lib/a.go", 1, 0), // exclusions are case-sensitive: matching more would hide code
		file("bad/x.go", 1, 0),        // a bad exclusion glob excludes nothing
		{Path: "vendor/moved.go", OldPath: "src/moved.go", Status: changeset.StatusRenamed, Hunks: []changeset.Hunk{{Added: lines(1)}}},
	}}

	r := Score(p, cs, gen)

	if want := []string{"vendor/lib/a.go", "api/v1/api.pb.go"}; !slices.Equal(r.Excluded, want) {
		t.Errorf("Excluded = %v, want %v", r.Excluded, want)
	}
	if got := factor(t, r, FactorSize).Value; got != 3 {
		t.Errorf("size = %d, want 3 (moving a file into vendor/ does not exclude it)", got)
	}
}

func TestDiffusion(t *testing.T) {
	tests := []struct {
		name   string
		files  []changeset.File
		value  int
		detail string
	}{
		{"none", nil, 0, "0 directories; change entropy 0.00"},
		{"root files", []changeset.File{file("a.md", 1, 0), file("b.md", 1, 0)}, 1, "1 directory; change entropy 1.00"},
		{"even spread", []changeset.File{file("a/x", 2, 0), file("b/x", 2, 0), file("c/x", 2, 0), file("d/x", 2, 0)}, 4, "4 directories; change entropy 1.00"},
		{"one dominant file", []changeset.File{file("a/x", 99, 0), file("a/y", 1, 0)}, 1, "1 directory; change entropy 0.08"},
		{"removed lines count", []changeset.File{file("a/x", 1, 1), file("a/y", 2, 0)}, 1, "1 directory; change entropy 1.00"},
		{"a file with no lines", []changeset.File{file("a/x", 5, 0), {Path: "b/bin", Binary: true}}, 2, "2 directories; change entropy 0.00"},
		{
			"rename across directories",
			[]changeset.File{{Path: "b/x", OldPath: "a/x", Status: changeset.StatusRenamed}},
			2, "2 directories; change entropy 0.00",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := factor(t, Score(basePolicy(), changeset.ChangeSet{Files: tt.files}, Generated{}), FactorDiffusion)
			if f.Value != tt.value || f.Detail != tt.detail {
				t.Errorf("diffusion = %d, %q; want %d, %q", f.Value, f.Detail, tt.value, tt.detail)
			}
		})
	}
}

func TestTestSignal(t *testing.T) {
	tests := []struct {
		name   string
		files  []changeset.File
		value  int
		detail string
	}{
		{"no source", []changeset.File{file("README.md", 50, 0)}, 0, "no source lines changed"},
		{"source only", []changeset.File{file("a.go", 30, 5)}, 35, "35 source lines changed with no test change"},
		{"with a test", []changeset.File{file("a.go", 30, 0), file("a_test.go", 1, 0)}, 0, "tests changed alongside 30 source lines"},
		{"deleted test", []changeset.File{file("a.go", 30, 0), {Path: "old_test.go", Status: changeset.StatusDeleted}}, 0, "tests changed alongside 30 source lines"},
		{"renamed test", []changeset.File{file("a.go", 30, 0), {Path: "b_test.go", OldPath: "a_test.go", Status: changeset.StatusRenamed}}, 30, "30 source lines changed with no test change"},
		{"renamed and edited test", []changeset.File{file("a.go", 30, 0), {Path: "b_test.go", OldPath: "a_test.go", Status: changeset.StatusRenamed, Hunks: []changeset.Hunk{{Added: []changeset.Line{{Line: 1, Text: "x"}}, Removed: []changeset.Line{{Line: 1, Text: "y"}}}}}}, 0, "tests changed alongside 30 source lines"},
		{"test listed first", []changeset.File{file("a_test.go", 1, 0), file("a.go", 30, 0)}, 0, "tests changed alongside 30 source lines"},
		{"test only", []changeset.File{file("a_test.go", 30, 0)}, 0, "no source lines changed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := factor(t, Score(basePolicy(), changeset.ChangeSet{Files: tt.files}, Generated{}), FactorTestSignal)
			if f.Value != tt.value || f.Detail != tt.detail {
				t.Errorf("test signal = %d, %q; want %d, %q", f.Value, f.Detail, tt.value, tt.detail)
			}
		})
	}
}

func TestIsTest(t *testing.T) {
	for _, p := range []string{
		"pkg/a_test.go", "tests/test_api.py", "api/views_test.py", "app/conftest.py",
		"src/app.test.ts", "src/app.spec.tsx", "web/__tests__/x.js", "test/helper.rb",
		"spec/models/user_spec.rb", "src/test/java/FooTest.java", "Foo.Tests/BarTests.cs",
		"src/HTTPSpec.kt", "e2e/Login2Test.php", "TEST/x.c", "specs/a.js",
	} {
		if !isTest(p) {
			t.Errorf("isTest(%q) = false", p)
		}
	}
	for _, p := range []string{
		"pkg/a.go", "latest/x.go", "contest.py", "src/attest.ts", "app/inspect.py", "lib/respect.go",
		"Specification.java", "internal/fixture/testdata/x.go", "Contest/Protest.java", "",
	} {
		if isTest(p) {
			t.Errorf("isTest(%q) = true", p)
		}
	}
}

func TestWords(t *testing.T) {
	tests := map[string][]string{
		"src/FooTest.java":  {"src", "foo", "test", "java"},
		"HTTPSpec":          {"http", "spec"},
		"a_b-c.d":           {"a", "b", "c", "d"},
		"Login2Test":        {"login2", "test"},
		"ABC":               {"abc"},
		"x//__tests__//y":   {"x", "tests", "y"},
		"getHTTPResponseOK": {"get", "http", "response", "ok"},
		"":                  nil,
	}
	for in, want := range tests {
		if got := words(in); !slices.Equal(got, want) {
			t.Errorf("words(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsSource(t *testing.T) {
	src := func(p string, added ...string) changeset.File {
		var ls []changeset.Line
		for i, a := range added {
			ls = append(ls, changeset.Line{Line: i + 1, Text: a})
		}
		return changeset.File{Path: p, Hunks: []changeset.Hunk{{Added: ls}}}
	}
	for _, f := range []changeset.File{
		src("a.go"), src("dir/b.PY"), src("c.tsx"), src("e.sh"), src("f.java"), src("Makefile"),
		src("lib.rs", "fn main() {", `    println!("hi");`, "}"),
		src("tool.unknownext"),
	} {
		if !isSource(f) {
			t.Errorf("isSource(%q) = false", f.Path)
		}
	}
	for _, f := range []changeset.File{
		src("README.md"), src("go.mod"), src("data.json"), src("x.yaml"), src("notes.txt"),
	} {
		if isSource(f) {
			t.Errorf("isSource(%q) = true", f.Path)
		}
	}
}

func TestParseAttributes(t *testing.T) {
	data := []byte(`# generated code
*.pb.go linguist-generated
/gen/** linguist-generated=true
docs/api/*.md linguist-vendored
third_party/** linguist-vendored=true
third_party/ours/** -linguist-vendored
keep.pb.go linguist-generated=false
legacy/** !linguist-generated
images/** binary
docs/gen/** text linguist-generated
[attr]custom linguist-generated
"quoted name" linguist-generated
dir/ linguist-generated
lonely

`)
	g := ParseAttributes(data)

	for _, p := range []string{"api/x.pb.go", "x.pb.go", "gen/a/b.go", "docs/api/ref.md", "docs/gen/x.md", "third_party/lib/x.c"} {
		if !g.Matches(p) {
			t.Errorf("Matches(%q) = false", p)
		}
	}
	for _, p := range []string{
		"x.go", "src/gen/a.go", "docs/api/sub/ref.md", "third_party/ours/x.c",
		"keep.pb.go", "a/keep.pb.go", "legacy/x.pb.go", "images/a.png", "dir/x", "lonely",
	} {
		if g.Matches(p) {
			t.Errorf("Matches(%q) = true", p)
		}
	}
	if (Generated{}).Matches("anything") {
		t.Error("empty Generated matched")
	}
}
