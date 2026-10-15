package triggers

import (
	"fmt"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/zendern/priskinator/internal/changeset"
	"github.com/zendern/priskinator/internal/policy"
)

// G10 rules fire on the shape of a change rather than its paths (ADR 0006).
// Their risk and reviewers come from policy like any other rule.
const (
	RuleSubmodule       = "G10.submodule"
	RuleSymlink         = "G10.symlink"
	RuleExecutableMode  = "G10.executable-mode"
	RuleBinary          = "G10.binary"
	RuleLFS             = "G10.lfs"
	RuleOversizeFile    = "G10.oversize-file"
	RuleWatchedPathMove = "G10.watched-path-moved"
)

// Hit is one rule that fired, with its routing from policy and the files that
// fired it.
type Hit struct {
	Rule      string
	Risk      policy.Risk
	Reviewers []string
	Files     []Evidence
}

// Evidence is one file that fired a rule, and why.
type Evidence struct {
	Path    string
	OldPath string
	Reason  string
}

// Evaluate runs every enabled path rule and G10 rule in p over cs. Path rules
// match both the old and new path of a rename, so a move out of a watched path
// still fires. Rules without paths other than G10 are content rules and are
// left to their own engines. Hits are sorted by rule ID, and each lists its
// files in change-set order.
func Evaluate(p *policy.Policy, cs changeset.ChangeSet) []Hit {
	e := evaluator{
		policy: p,
		fold:   p.Match.CaseSensitive == nil || !*p.Match.CaseSensitive,
		files:  map[string][]Evidence{},
	}
	// Rules without paths never match one, so they need no filtering here.
	for id, t := range p.Triggers {
		if t.IsEnabled() {
			e.pathRules = append(e.pathRules, id)
		}
	}
	slices.Sort(e.pathRules)

	for _, f := range cs.Files {
		e.matchPaths(f)
		e.matchShape(f)
	}

	ids := make([]string, 0, len(e.files))
	for id := range e.files {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	hits := make([]Hit, 0, len(ids))
	for _, id := range ids {
		t := p.Triggers[id]
		hits = append(hits, Hit{Rule: id, Risk: t.Risk, Reviewers: slices.Clone(t.Reviewers), Files: e.files[id]})
	}
	return hits
}

type evaluator struct {
	policy    *policy.Policy
	fold      bool
	pathRules []string
	files     map[string][]Evidence
}

func (e *evaluator) fire(id string, f changeset.File, format string, args ...any) {
	e.files[id] = append(e.files[id], Evidence{Path: f.Path, OldPath: f.OldPath, Reason: fmt.Sprintf(format, args...)})
}

// matchPaths fires each path rule at most once per file, naming the first glob
// that matched and whether it matched the old path.
func (e *evaluator) matchPaths(f changeset.File) {
	for _, id := range e.pathRules {
		for i, name := range f.Paths() {
			glob, why, ok := e.match(id, name)
			if !ok {
				continue
			}
			side := ""
			if i > 0 {
				side = "old path "
			}
			e.fire(id, f, "%s%s %q%s", side, why, glob, suffix(why))
			break
		}
	}
}

// match reports the first glob of rule id that name matches. A glob that fails
// to parse counts as a match, so a bad policy sends changes to a human.
func (e *evaluator) match(id, name string) (glob, why string, ok bool) {
	if e.fold {
		name = strings.ToLower(name)
	}
	for _, g := range e.policy.Triggers[id].Paths {
		pattern := g
		if e.fold {
			pattern = strings.ToLower(g)
		}
		matched, err := doublestar.Match(pattern, name)
		if err != nil {
			return g, "invalid glob", true
		}
		if matched {
			return g, "matches", true
		}
	}
	return "", "", false
}

func suffix(why string) string {
	if why == "invalid glob" {
		return " counts as a match"
	}
	return ""
}

func (e *evaluator) enabled(id string) bool {
	t := e.policy.Triggers[id]
	return t != nil && t.IsEnabled()
}

// matchShape fires the G10 hidden-change rules for one file.
func (e *evaluator) matchShape(f changeset.File) {
	if e.enabled(RuleSubmodule) && (f.OldMode == changeset.ModeSubmodule || f.NewMode == changeset.ModeSubmodule) {
		e.fire(RuleSubmodule, f, "submodule pointer")
	}
	if e.enabled(RuleSymlink) && f.NewMode == changeset.ModeSymlink {
		e.fire(RuleSymlink, f, "symlink to %q", f.SymlinkTarget)
	}
	if e.enabled(RuleExecutableMode) && f.ModeChanged() && f.NewMode == changeset.ModeExecutable {
		e.fire(RuleExecutableMode, f, "mode %s to %s", f.OldMode, f.NewMode)
	}
	if e.enabled(RuleBinary) && f.Binary {
		e.fire(RuleBinary, f, "binary content")
	}
	if e.enabled(RuleLFS) && f.LFS {
		e.fire(RuleLFS, f, "Git LFS pointer")
	}
	if limit := e.policy.Limits.MaxFileBytes; e.enabled(RuleOversizeFile) && limit != nil && f.NewSize > *limit {
		e.fire(RuleOversizeFile, f, "%d bytes, over the %d-byte limit", f.NewSize, *limit)
	}
	if e.enabled(RuleWatchedPathMove) {
		e.matchMove(f)
	}
}

// matchMove fires when a deletion removes a watched file, or a rename takes a
// file out of a rule that watched its old path. A rename within the same rule
// keeps the file watched, and a copy leaves the original in place.
func (e *evaluator) matchMove(f changeset.File) {
	if f.Status != changeset.StatusDeleted && f.Status != changeset.StatusRenamed {
		return
	}
	old := f.Path
	if f.Status == changeset.StatusRenamed {
		old = f.OldPath
	}
	var left []string
	for _, id := range e.pathRules {
		if _, _, ok := e.match(id, old); !ok {
			continue
		}
		if f.Status == changeset.StatusRenamed {
			if _, _, still := e.match(id, f.Path); still {
				continue
			}
		}
		left = append(left, id)
	}
	if len(left) == 0 {
		return
	}
	verb := "moved out of"
	if f.Status == changeset.StatusDeleted {
		verb = "deleted from"
	}
	e.fire(RuleWatchedPathMove, f, "%s %s", verb, strings.Join(left, ", "))
}
