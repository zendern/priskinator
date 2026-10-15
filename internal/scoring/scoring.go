package scoring

import (
	"fmt"
	"math"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/zendern/priskinator/internal/changeset"
	"github.com/zendern/priskinator/internal/policy"
)

// Factor names, matching the keys under scoring.factors in policy.
const (
	FactorSize            = "size"
	FactorFiles           = "files"
	FactorDiffusion       = "diffusion"
	FactorComplexityDelta = "complexity_delta"
	FactorTestSignal      = "test_signal"
)

// Result is the score for one change and everything that went into it, so
// the verdict can show the author why a change got its tier (ADR 0007).
type Result struct {
	// Points is the sum of every factor's points.
	Points int
	// Tier is what Points reaches against the policy thresholds.
	Tier policy.Risk
	// Risk is Tier raised to the highest criticality floor that applied.
	Risk     policy.Risk
	Factors  []Factor
	Floors   []FloorHit
	Excluded []string
	Notes    []string
}

// Factor is one scoring factor: its raw value, the band it fell in, and the
// points that band is worth. Unmeasured factors score nothing and say why.
type Factor struct {
	Name     string
	Measured bool
	Value    int
	Points   int
	Band     string
	Detail   string
}

// FloorHit is a criticality floor that applied, and the files that matched it.
type FloorHit struct {
	Description string
	Risk        policy.Risk
	Paths       []string
}

// Score computes the additive risk score for cs under p. Files matching
// scoring.exclude_paths or gen are left out of every factor; floors still see
// them. Exclusions match case-sensitively and need every path of a rename to
// match, because excluding more would hide code from scoring. Floors match like
// triggers, both paths of a rename and case-insensitively unless policy says
// otherwise, because matching more is the stricter side for a floor.
func Score(p *policy.Policy, cs changeset.ChangeSet, gen Generated) Result {
	var r Result
	var scored []changeset.File
	for _, f := range cs.Files {
		if excluded(p.Scoring.ExcludePaths, gen, f) {
			r.Excluded = append(r.Excluded, f.Path)
			continue
		}
		scored = append(scored, f)
	}

	f := p.Scoring.Factors
	r.Factors = []Factor{
		measure(FactorSize, f.Size, sizeOf(scored, len(r.Excluded))),
		measure(FactorFiles, f.Files, filesOf(scored, len(r.Excluded))),
		measure(FactorDiffusion, f.Diffusion, diffusionOf(scored)),
		{Name: FactorComplexityDelta, Detail: "not measured: complexity analysis is not available yet"},
		measure(FactorTestSignal, f.TestSignal, testSignalOf(scored)),
	}
	for _, fa := range r.Factors {
		r.Points += fa.Points
	}

	r.Tier = policy.RiskHigh
	if th := p.Scoring.Thresholds; th == nil {
		r.Notes = append(r.Notes, "policy sets no scoring thresholds, so the tier is high")
	} else {
		r.Tier = tier(r.Points, *th)
	}

	fold := p.Match.CaseSensitive == nil || !*p.Match.CaseSensitive
	r.Risk = r.Tier
	for _, fl := range p.Scoring.Floors {
		var paths []string
		for _, file := range cs.Files {
			if floorMatches(fl.Paths, file, fold) {
				paths = append(paths, file.Path)
			}
		}
		if len(paths) == 0 {
			continue
		}
		r.Floors = append(r.Floors, FloorHit{Description: fl.Description, Risk: fl.Risk, Paths: paths})
		r.Risk = policy.MaxRisk(r.Risk, fl.Risk)
	}
	return r
}

func tier(points int, th policy.Thresholds) policy.Risk {
	if points >= th.High {
		return policy.RiskHigh
	}
	if points >= th.Medium {
		return policy.RiskMedium
	}
	return policy.RiskLow
}

// measurement is a factor's raw value and a sentence about it.
type measurement struct {
	value  int
	detail string
}

func measure(name string, b policy.Bands, m measurement) Factor {
	points, label := band(b, m.value)
	return Factor{Name: name, Measured: true, Value: m.value, Points: points, Band: label, Detail: m.detail}
}

// band returns the points for value and a label for its band, such as
// "51-200" or ">1000".
func band(b policy.Bands, value int) (int, string) {
	if len(b) == 0 {
		return 0, "no bands"
	}
	low := 0
	for _, bd := range b {
		if bd.UpTo == nil {
			if low == 0 {
				return bd.Points, "any"
			}
			return bd.Points, ">" + strconv.Itoa(low-1)
		}
		if value <= *bd.UpTo {
			return bd.Points, strconv.Itoa(low) + "-" + strconv.Itoa(*bd.UpTo)
		}
		low = *bd.UpTo + 1
	}
	return 0, "above " + strconv.Itoa(low-1) + ", no band"
}

func excludedNote(n int) string {
	if n == 0 {
		return ""
	}
	return "; " + strconv.Itoa(n) + " excluded"
}

func sizeOf(files []changeset.File, excluded int) measurement {
	added, removed := 0, 0
	for _, f := range files {
		added += f.AddedLines()
		removed += f.RemovedLines()
	}
	return measurement{
		value:  added + removed,
		detail: fmt.Sprintf("%d added and %d removed lines in %s%s", added, removed, plural(len(files), "file"), excludedNote(excluded)),
	}
}

func filesOf(files []changeset.File, excluded int) measurement {
	return measurement{value: len(files), detail: plural(len(files), "file") + " changed" + excludedNote(excluded)}
}

// diffusionOf counts the directories a change touches, both sides of a move
// included, and reports the normalized entropy of its lines across files
// (Kamei et al.): 0 when one file holds every changed line, 1 when they are
// spread evenly.
func diffusionOf(files []changeset.File) measurement {
	var dirs []string
	var churn []int
	total := 0
	for _, f := range files {
		for _, p := range f.Paths() {
			if d := path.Dir(p); !slices.Contains(dirs, d) {
				dirs = append(dirs, d)
			}
		}
		if n := f.AddedLines() + f.RemovedLines(); n > 0 {
			churn = append(churn, n)
			total += n
		}
	}
	entropy := 0.0
	if len(churn) > 1 {
		for _, n := range churn {
			q := float64(n) / float64(total)
			entropy -= q * math.Log2(q)
		}
		entropy /= math.Log2(float64(len(churn)))
	}
	return measurement{value: len(dirs), detail: fmt.Sprintf("%s; change entropy %.2f", plural(len(dirs), "directory"), entropy)}
}

// testSignalOf scores non-test source lines changed when no test file
// changed. It adds risk for untested change and never subtracts it.
func testSignalOf(files []changeset.File) measurement {
	source, tests := 0, false
	for _, f := range files {
		if isTest(f.Path) {
			tests = true
			continue
		}
		if isSource(f) {
			source += f.AddedLines() + f.RemovedLines()
		}
	}
	if source == 0 {
		return measurement{detail: "no source lines changed"}
	}
	if tests {
		return measurement{detail: fmt.Sprintf("tests changed alongside %d source lines", source)}
	}
	return measurement{value: source, detail: fmt.Sprintf("%d source lines changed with no test change", source)}
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	if strings.HasSuffix(noun, "y") {
		return strconv.Itoa(n) + " " + strings.TrimSuffix(noun, "y") + "ies"
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

func excluded(globs []string, gen Generated, f changeset.File) bool {
	for _, p := range f.Paths() {
		if !gen.Matches(p) && !matchAny(globs, p, false) {
			return false
		}
	}
	return true
}

// floorMatches reports whether any path of f matches a floor glob. A glob that
// fails to parse counts as a match, so a bad policy raises risk.
func floorMatches(globs []string, f changeset.File, fold bool) bool {
	for _, p := range f.Paths() {
		if matchAny(globs, p, fold) {
			return true
		}
	}
	return false
}

// matchAny reports whether name matches any glob, lowercasing both when fold
// is set. A glob that fails to parse matches only when fold is set: floors
// pass fold from policy and treat a bad glob as a match, while exclusions
// never fold and never exclude on a bad glob.
func matchAny(globs []string, name string, fold bool) bool {
	if fold {
		name = strings.ToLower(name)
	}
	for _, g := range globs {
		if fold {
			g = strings.ToLower(g)
		}
		ok, err := doublestar.Match(g, name)
		if ok || (err != nil && fold) {
			return true
		}
	}
	return false
}
