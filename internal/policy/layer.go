package policy

import (
	"cmp"
	"fmt"
	"slices"
)

// Weakening is one field a repo policy loosens. A repo may loosen what only
// the built-in defaults set, and then Applied is true: the repo's value takes
// effect and the weakening is reported so verdicts and evidence can name it. A
// field the org baseline sets is a floor: Applied is false, the baseline value
// stays, and the evaluation fires TriggerPolicyWeakeningAttempt.
type Weakening struct {
	Rule    string // trigger ID, empty for fields outside a rule
	Path    string // JSON pointer to the field in the repo policy
	Message string
	Applied bool
}

// Result is the effective policy for one change and what layering found.
type Result struct {
	Policy     *Policy
	Weakenings []Weakening
	// Triggers the policy layer fired, from TriggerConfigInvalid and
	// TriggerPolicyWeakeningAttempt.
	Triggers []string
	// ConfigErr says why the repo policy was ignored, when TriggerConfigInvalid fired.
	ConfigErr error
}

// Resolve builds the effective policy: built-in defaults, then the org
// baseline, then the repo policy (ADR 0010). A nil org or repo is a layer that
// does not exist. An invalid org baseline is an error, because there is no
// safe fallback for it. An invalid repo policy is ignored and fires
// config-invalid. A repo policy may loosen what only the defaults set, and
// each loosening is reported as an applied Weakening. Loosening a field the org
// baseline sets is left out and fires policy-weakening-attempt.
func Resolve(org, repo []byte) (*Result, error) {
	base := Defaults()
	locked := func(string) bool { return false }
	if org != nil {
		o, err := Parse(org)
		if err != nil {
			return nil, fmt.Errorf("org baseline: %w", err)
		}
		base = Overlay(base, o)
		if problems := check(base); len(problems) > 0 {
			return nil, fmt.Errorf("org baseline: %w", &Error{Problems: problems})
		}
		set := setBy(o)
		locked = func(path string) bool { return set[path] }
	}
	res := &Result{Policy: base}
	if repo == nil {
		return res, nil
	}
	r, err := Parse(repo)
	if err != nil {
		res.Triggers = []string{TriggerConfigInvalid}
		res.ConfigErr = err
		return res, nil
	}
	res.Policy, res.Weakenings = merge(base, r, locked)
	if slices.ContainsFunc(res.Weakenings, func(w Weakening) bool { return !w.Applied }) {
		res.Triggers = []string{TriggerPolicyWeakeningAttempt}
	}
	return res, nil
}

// setBy returns the JSON pointer of every field p sets, in the form Merge
// reports weakenings. A rule's paths count as set when p sets paths or
// add_paths.
func setBy(p *Policy) map[string]bool {
	set := map[string]bool{}
	mark := func(path string, ok bool) {
		if ok {
			set[path] = true
		}
	}
	mark("/match/case_sensitive", p.Match.CaseSensitive != nil)
	mark("/limits/max_file_bytes", p.Limits.MaxFileBytes != nil)
	mark("/limits/max_changed_files", p.Limits.MaxChangedFiles != nil)
	mark("/limits/max_changed_lines", p.Limits.MaxChangedLines != nil)
	for id, t := range p.Triggers {
		at := "/triggers/" + id
		// A rule the org configures can't be switched off by a repo.
		mark(at+"/enabled", t.Enabled != nil || t.Paths != nil || len(t.AddPaths) > 0 || t.Reviewers != nil || t.Risk != "")
		mark(at+"/paths", t.Paths != nil || len(t.AddPaths) > 0)
		mark(at+"/reviewers", t.Reviewers != nil)
		mark(at+"/risk", t.Risk != "")
	}
	mark("/scoring/thresholds/medium", p.Scoring.Thresholds != nil)
	mark("/scoring/thresholds/high", p.Scoring.Thresholds != nil)
	names := []string{"size", "files", "diffusion", "complexity_delta", "test_signal"}
	for i, f := range factorsOf(&Factors{}, &p.Scoring.Factors) {
		mark("/scoring/factors/"+names[i], len(f.over) > 0)
	}
	mark("/scoring/exclude_paths", p.Scoring.ExcludePaths != nil)
	mark("/ai/enabled", p.AI.Enabled != nil)
	mark("/ai/required", p.AI.Required != nil)
	mark("/ai/provider", p.AI.Provider != "")
	mark("/ai/model", p.AI.Model != "")
	mark("/ai/endpoint", p.AI.Endpoint != "")
	mark("/ai/max_input_tokens", p.AI.MaxInputTokens != 0)
	mark("/ai/timeout_seconds", p.AI.TimeoutSeconds != 0)
	return set
}

// Overlay applies a trusted layer (the org baseline) over base. The org may
// replace anything, including the placeholder reviewers in the defaults. Only
// fields the org sets change. Neither input is modified.
func Overlay(base, org *Policy) *Policy {
	out := clone(base)
	if org.Match.CaseSensitive != nil {
		out.Match.CaseSensitive = ptr(*org.Match.CaseSensitive)
	}
	setPtr(&out.Limits.MaxFileBytes, org.Limits.MaxFileBytes)
	setPtr(&out.Limits.MaxChangedFiles, org.Limits.MaxChangedFiles)
	setPtr(&out.Limits.MaxChangedLines, org.Limits.MaxChangedLines)

	for id, ot := range org.Triggers {
		bt := out.Triggers[id]
		if bt == nil {
			bt = &Trigger{}
			out.Triggers[id] = bt
		}
		if ot.Description != "" {
			bt.Description = ot.Description
		}
		setPtr(&bt.Enabled, ot.Enabled)
		if ot.Paths != nil {
			bt.Paths = slices.Clone(ot.Paths)
		}
		bt.Paths = union(bt.Paths, ot.AddPaths)
		if ot.Reviewers != nil {
			bt.Reviewers = slices.Clone(ot.Reviewers)
		}
		if ot.Risk != "" {
			bt.Risk = ot.Risk
		}
	}

	if th := org.Scoring.Thresholds; th != nil {
		out.Scoring.Thresholds = &Thresholds{Medium: th.Medium, High: th.High}
	}
	for _, f := range factorsOf(&out.Scoring.Factors, &org.Scoring.Factors) {
		if len(f.over) > 0 {
			*f.dst = cloneBands(f.over)
		}
	}
	out.Scoring.Floors = appendFloors(out.Scoring.Floors, org.Scoring.Floors)
	if org.Scoring.ExcludePaths != nil {
		out.Scoring.ExcludePaths = slices.Clone(org.Scoring.ExcludePaths)
	}

	setPtr(&out.AI.Enabled, org.AI.Enabled)
	setPtr(&out.AI.Required, org.AI.Required)
	setStr(&out.AI.Provider, org.AI.Provider)
	setStr(&out.AI.Model, org.AI.Model)
	setStr(&out.AI.Endpoint, org.AI.Endpoint)
	setInt(&out.AI.MaxInputTokens, org.AI.MaxInputTokens)
	setInt(&out.AI.TimeoutSeconds, org.AI.TimeoutSeconds)
	return out
}

// Merge applies a repo policy over base so the repo can only tighten (ADR
// 0010). Tightenings are applied. Anything that would loosen the baseline is
// left out and reported, field by field. Neither input is modified.
//
// What counts as tighter, per field:
//   - limits, thresholds, ai.max_input_tokens, ai.timeout_seconds: lower. A
//     smaller cap or threshold sends more changes to a human.
//   - risk floors: higher.
//   - case sensitivity: insensitive, which matches more paths.
//   - rule paths and reviewers: a superset. Paths are compared as written, so
//     a narrower glob is a removal even when it would match the same files.
//   - scoring bands: at least the baseline's points for every value.
//   - scoring floors: any floor can be added; none can be removed.
//   - scoring.exclude_paths: a subset, since excluding a path hides it from scoring.
//   - rules: enabled, never disabled; new rules can be added.
//   - ai: on can be turned on, and off is refused only when the baseline
//     requires it. Provider, model, and endpoint are fixed once the baseline
//     sets them, because they decide where source code is sent.
func Merge(base, repo *Policy) (*Policy, []Weakening) {
	return merge(base, repo, func(string) bool { return true })
}

// merge is Merge where only the fields locked reports are held to tighten-only.
// A weakening of any other field is applied and reported as Applied.
func merge(base, repo *Policy, locked func(path string) bool) (*Policy, []Weakening) {
	out := clone(base)
	var ws []Weakening
	weaken := func(rule, path, format string, args ...any) bool {
		applied := !locked(path)
		ws = append(ws, Weakening{Rule: rule, Path: path, Message: fmt.Sprintf(format, args...), Applied: applied})
		return applied
	}

	// Case-insensitive matching is the stricter mode.
	if c := repo.Match.CaseSensitive; c != nil {
		if !*c || isTrue(base.Match.CaseSensitive) ||
			weaken("", "/match/case_sensitive", "case-sensitive matching misses paths the baseline matches") {
			out.Match.CaseSensitive = ptr(*c)
		}
	}

	lower(&out.Limits.MaxFileBytes, repo.Limits.MaxFileBytes, "/limits/max_file_bytes", weaken)
	lower(&out.Limits.MaxChangedFiles, repo.Limits.MaxChangedFiles, "/limits/max_changed_files", weaken)
	lower(&out.Limits.MaxChangedLines, repo.Limits.MaxChangedLines, "/limits/max_changed_lines", weaken)

	ids := make([]string, 0, len(repo.Triggers))
	for id := range repo.Triggers {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		rt := repo.Triggers[id]
		bt := out.Triggers[id]
		if bt == nil {
			out.Triggers[id] = &Trigger{
				Description: rt.Description,
				Enabled:     cloneBoolPtr(rt.Enabled),
				Paths:       union(nil, rt.Paths, rt.AddPaths),
				Reviewers:   slices.Clone(rt.Reviewers),
				Risk:        rt.Risk,
			}
			continue
		}
		mergeTrigger(id, bt, rt, weaken)
	}

	if rth := repo.Scoring.Thresholds; rth != nil {
		if out.Scoring.Thresholds == nil {
			out.Scoring.Thresholds = &Thresholds{Medium: rth.Medium, High: rth.High}
		} else {
			m, h := ptr(out.Scoring.Thresholds.Medium), ptr(out.Scoring.Thresholds.High)
			lower(&m, ptr(rth.Medium), "/scoring/thresholds/medium", weaken)
			lower(&h, ptr(rth.High), "/scoring/thresholds/high", weaken)
			out.Scoring.Thresholds = &Thresholds{Medium: *m, High: *h}
		}
	}
	names := []string{"size", "files", "diffusion", "complexity_delta", "test_signal"}
	for i, f := range factorsOf(&out.Scoring.Factors, &repo.Scoring.Factors) {
		if len(f.over) == 0 {
			continue
		}
		if len(*f.dst) != 0 && !bandsDominate(f.over, *f.dst) &&
			!weaken("", "/scoring/factors/"+names[i], "gives some values fewer points than the baseline") {
			continue
		}
		*f.dst = cloneBands(f.over)
	}
	out.Scoring.Floors = appendFloors(out.Scoring.Floors, repo.Scoring.Floors)
	if repo.Scoring.ExcludePaths != nil {
		var extra []string
		kept := []string{}
		for _, g := range repo.Scoring.ExcludePaths {
			if slices.Contains(base.Scoring.ExcludePaths, g) {
				kept = append(kept, g)
			} else {
				extra = append(extra, g)
			}
		}
		if len(extra) > 0 && weaken("", "/scoring/exclude_paths", "excludes paths the baseline scores: %v", extra) {
			kept = slices.Clone(repo.Scoring.ExcludePaths)
		}
		out.Scoring.ExcludePaths = kept
	}

	mergeAI(&out.AI, base.AI, repo.AI, weaken)
	return out, ws
}

// weakenFn reports a loosening and returns whether to apply it.
type weakenFn func(rule, path, format string, args ...any) bool

func mergeTrigger(id string, out, repo *Trigger, weaken weakenFn) {
	at := "/triggers/" + id
	if repo.Enabled != nil {
		if *repo.Enabled || !out.IsEnabled() || weaken(id, at+"/enabled", "disables a baseline rule") {
			out.Enabled = ptr(*repo.Enabled)
		}
	}
	if repo.Paths != nil {
		if missing := difference(out.Paths, repo.Paths); len(missing) > 0 && weaken(id, at+"/paths", "removes baseline paths: %v", missing) {
			out.Paths = nil
		}
		out.Paths = union(out.Paths, repo.Paths)
	}
	out.Paths = union(out.Paths, repo.AddPaths)
	if repo.Reviewers != nil {
		if missing := difference(out.Reviewers, repo.Reviewers); len(missing) > 0 && weaken(id, at+"/reviewers", "removes baseline reviewers: %v", missing) {
			out.Reviewers = nil
		}
		out.Reviewers = union(out.Reviewers, repo.Reviewers)
	}
	if repo.Risk != "" {
		if out.Risk == "" || repo.Risk.Rank() >= out.Risk.Rank() ||
			weaken(id, at+"/risk", "lowers the risk floor from %s to %s", out.Risk, repo.Risk) {
			out.Risk = repo.Risk
		}
	}
	if out.Description == "" {
		out.Description = repo.Description
	}
}

func mergeAI(out *AI, base, repo AI, weaken weakenFn) {
	if repo.Required != nil {
		if *repo.Required || !base.IsRequired() || weaken("", "/ai/required", "stops requiring the AI assessment") {
			out.Required = ptr(*repo.Required)
		}
	}
	if repo.Enabled != nil {
		if *repo.Enabled || !out.IsRequired() || !out.IsEnabled() ||
			weaken("", "/ai/enabled", "disables a required AI assessment") {
			out.Enabled = ptr(*repo.Enabled)
		}
	}
	fixed := func(dst *string, base, repo, path string) {
		if repo == "" || repo == base {
			return
		}
		if base != "" && !weaken("", path, "changes %q set by the baseline", base) {
			return
		}
		*dst = repo
	}
	fixed(&out.Provider, base.Provider, repo.Provider, "/ai/provider")
	fixed(&out.Model, base.Model, repo.Model, "/ai/model")
	fixed(&out.Endpoint, base.Endpoint, repo.Endpoint, "/ai/endpoint")

	for _, f := range []struct {
		dst        *int
		base, repo int
		path       string
	}{
		{&out.MaxInputTokens, base.MaxInputTokens, repo.MaxInputTokens, "/ai/max_input_tokens"},
		{&out.TimeoutSeconds, base.TimeoutSeconds, repo.TimeoutSeconds, "/ai/timeout_seconds"},
	} {
		if f.repo == 0 {
			continue
		}
		v := ptr(f.base)
		if f.base == 0 {
			v = nil
		}
		lower(&v, ptr(f.repo), f.path, weaken)
		*f.dst = *v
	}
}

// lower sets *dst to repo when repo is not above it. A higher repo is
// reported, and applied only when weaken allows it. An unset repo value
// inherits; an unset base takes repo's.
func lower[T cmp.Ordered](dst **T, repo *T, path string, weaken weakenFn) {
	if repo == nil {
		return
	}
	if *dst != nil && *repo > **dst && !weaken("", path, "raises %v to %v", **dst, *repo) {
		return
	}
	*dst = ptr(*repo)
}

// bandsDominate reports whether repo gives at least as many points as base for
// every value. Both are step functions, so checking each breakpoint and the
// value after it covers every segment.
func bandsDominate(repo, base Bands) bool {
	values := []int{0}
	for _, bs := range []Bands{repo, base} {
		for _, b := range bs {
			if b.UpTo != nil {
				values = append(values, *b.UpTo, *b.UpTo+1)
			}
		}
	}
	for _, v := range values {
		if repo.Points(v) < base.Points(v) {
			return false
		}
	}
	return true
}

type factorPair struct {
	dst  *Bands
	over Bands
}

// factorsOf pairs each scoring factor in dst with its counterpart in over, in
// schema order.
func factorsOf(dst, over *Factors) []factorPair {
	return []factorPair{
		{&dst.Size, over.Size},
		{&dst.Files, over.Files},
		{&dst.Diffusion, over.Diffusion},
		{&dst.ComplexityDelta, over.ComplexityDelta},
		{&dst.TestSignal, over.TestSignal},
	}
}

func appendFloors(dst, add []Floor) []Floor {
	for _, f := range add {
		if !slices.ContainsFunc(dst, func(e Floor) bool {
			return e.Risk == f.Risk && slices.Equal(e.Paths, f.Paths)
		}) {
			dst = append(dst, Floor{Description: f.Description, Paths: slices.Clone(f.Paths), Risk: f.Risk})
		}
	}
	return dst
}

// union appends each list's new items to a copy of base, keeping order.
func union(base []string, lists ...[]string) []string {
	out := slices.Clone(base)
	for _, l := range lists {
		for _, s := range l {
			if !slices.Contains(out, s) {
				out = append(out, s)
			}
		}
	}
	return out
}

// difference returns the items of want that have is not.
func difference(want, have []string) []string {
	var out []string
	for _, s := range want {
		if !slices.Contains(have, s) {
			out = append(out, s)
		}
	}
	return out
}

func ptr[T any](v T) *T { return &v }

func isTrue(b *bool) bool { return b != nil && *b }

func cloneBoolPtr(b *bool) *bool {
	if b == nil {
		return nil
	}
	return ptr(*b)
}

func setPtr[T any](dst **T, src *T) {
	if src != nil {
		*dst = ptr(*src)
	}
}

func setStr(dst *string, src string) {
	if src != "" {
		*dst = src
	}
}

func setInt(dst *int, src int) {
	if src != 0 {
		*dst = src
	}
}

func cloneBands(b Bands) Bands {
	out := make(Bands, len(b))
	for i, band := range b {
		out[i] = Band{Points: band.Points}
		if band.UpTo != nil {
			out[i].UpTo = ptr(*band.UpTo)
		}
	}
	return out
}

// clone deep-copies a policy so layering never shares memory with its inputs.
func clone(p *Policy) *Policy {
	out := *p
	out.Match.CaseSensitive = cloneBoolPtr(p.Match.CaseSensitive)
	if p.Limits.MaxFileBytes != nil {
		out.Limits.MaxFileBytes = ptr(*p.Limits.MaxFileBytes)
	}
	if p.Limits.MaxChangedFiles != nil {
		out.Limits.MaxChangedFiles = ptr(*p.Limits.MaxChangedFiles)
	}
	if p.Limits.MaxChangedLines != nil {
		out.Limits.MaxChangedLines = ptr(*p.Limits.MaxChangedLines)
	}
	out.Triggers = make(map[string]*Trigger, len(p.Triggers))
	for id, t := range p.Triggers {
		c := *t
		c.Enabled = cloneBoolPtr(t.Enabled)
		c.Paths = slices.Clone(t.Paths)
		c.AddPaths = slices.Clone(t.AddPaths)
		c.Reviewers = slices.Clone(t.Reviewers)
		out.Triggers[id] = &c
	}
	if p.Scoring.Thresholds != nil {
		out.Scoring.Thresholds = ptr(*p.Scoring.Thresholds)
	}
	f := &out.Scoring.Factors
	for _, pair := range factorsOf(f, &p.Scoring.Factors) {
		*pair.dst = cloneBands(pair.over)
		if pair.over == nil {
			*pair.dst = nil
		}
	}
	out.Scoring.Floors = appendFloors([]Floor{}, p.Scoring.Floors)
	out.Scoring.ExcludePaths = slices.Clone(p.Scoring.ExcludePaths)
	out.AI.Enabled = cloneBoolPtr(p.AI.Enabled)
	out.AI.Required = cloneBoolPtr(p.AI.Required)
	return &out
}
