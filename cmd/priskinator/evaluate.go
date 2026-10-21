package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/zendern/priskinator/internal/changeset"
	"github.com/zendern/priskinator/internal/policy"
	"github.com/zendern/priskinator/internal/scoring"
	"github.com/zendern/priskinator/internal/verdict"
)

// Paths evaluate reads from the base and head commits.
const (
	policyPath     = ".priskinator/policy.yaml"
	attributesPath = ".gitattributes"
)

// Exit codes. A verdict is a result, whatever its risk, so evaluate exits 0
// for every evaluated change; 1 means the verdict is an evaluation error.
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

const evaluateUsage = `usage: priskinator evaluate --base <rev> --head <rev> [flags]

Evaluates the change from base to head against the base commit's policy and
prints the verdict. The base commit's .priskinator/policy.yaml and
.gitattributes apply; the head's policy is read only to call out loosenings.

Flags:
`

// evaluateOptions are the evaluate flags.
type evaluateOptions struct {
	base, head, repo, orgPolicy, format string
	git                                 string
}

func runEvaluate(args []string, stdout, stderr io.Writer) int {
	opts := evaluateOptions{}
	fs := flag.NewFlagSet("evaluate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, evaluateUsage)
		fs.PrintDefaults()
	}
	fs.StringVar(&opts.base, "base", "", "base revision (required)")
	fs.StringVar(&opts.head, "head", "", "head revision (required)")
	fs.StringVar(&opts.repo, "repo", ".", "path to the git repository")
	fs.StringVar(&opts.orgPolicy, "org-policy", "", "org baseline policy file")
	fs.StringVar(&opts.format, "format", "human", "output format: human, json or markdown")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() > 0 || opts.base == "" || opts.head == "" || !slices.Contains([]string{"human", "json", "markdown"}, opts.format) {
		fs.Usage()
		return exitUsage
	}

	v, err := evaluate(context.Background(), opts)
	if err != nil {
		v = verdict.Failed(err)
	}
	if err := write(stdout, opts.format, v); err != nil {
		_, _ = fmt.Fprintf(stderr, "write verdict: %v\n", err)
		return exitError
	}
	if v.Evaluation == verdict.EvaluationError {
		return exitError
	}
	return exitOK
}

// evaluate reads everything one evaluation needs from the repository and
// returns its verdict. Any error fails the evaluation closed.
func evaluate(ctx context.Context, o evaluateOptions) (verdict.Verdict, error) {
	gopts := changeset.Options{Git: o.git}
	base, err := changeset.ResolveCommit(ctx, o.repo, o.base, gopts)
	if err != nil {
		return verdict.Verdict{}, err
	}
	head, err := changeset.ResolveCommit(ctx, o.repo, o.head, gopts)
	if err != nil {
		return verdict.Verdict{}, err
	}
	org, err := readOrgPolicy(o.orgPolicy)
	if err != nil {
		return verdict.Verdict{}, err
	}
	cs, err := changeset.FromGit(ctx, o.repo, base, head, gopts)
	if err != nil {
		return verdict.Verdict{}, err
	}
	read := func(commit, path string) ([]byte, error) {
		data, _, err := changeset.ReadFile(ctx, o.repo, commit, path, policy.MaxPolicyBytes, gopts)
		return data, err
	}
	basePolicy, err := read(base, policyPath)
	if err != nil {
		return verdict.Verdict{}, err
	}
	res, err := policy.Resolve(org, basePolicy)
	if err != nil {
		return verdict.Verdict{}, err
	}
	attrs, err := read(base, attributesPath)
	if err != nil {
		return verdict.Verdict{}, err
	}
	proposed, err := proposedWeakenings(org, basePolicy, func() ([]byte, error) { return read(head, policyPath) })
	if err != nil {
		return verdict.Verdict{}, err
	}
	return verdict.Evaluate(verdict.Input{
		Policy:    res,
		ChangeSet: *cs,
		Generated: scoring.ParseAttributes(attrs),
		Proposed:  proposed,
	}), nil
}

// proposedWeakenings returns the weakenings in the head's repo policy when the
// change edits it, so the verdict can call out each loosening before it takes
// effect (ADR 0010).
func proposedWeakenings(org, basePolicy []byte, readHead func() ([]byte, error)) ([]policy.Weakening, error) {
	headPolicy, err := readHead()
	if err != nil {
		return nil, err
	}
	if headPolicy == nil || bytes.Equal(headPolicy, basePolicy) {
		return nil, nil
	}
	res, err := policy.Resolve(org, headPolicy)
	if err != nil {
		return nil, err
	}
	return res.Weakenings, nil
}

// readOrgPolicy reads the org baseline, or returns nil when none is given.
func readOrgPolicy(path string) ([]byte, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("org baseline: %w", err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, policy.MaxPolicyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("org baseline: %w", err)
	}
	if len(data) > policy.MaxPolicyBytes {
		return nil, fmt.Errorf("org baseline: %w", errOversizePolicy)
	}
	return data, nil
}

var errOversizePolicy = errors.New("over the policy size limit")

func write(w io.Writer, format string, v verdict.Verdict) error {
	v = withEmptyLists(v)
	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	out := human(v)
	if format == "markdown" {
		out = markdown(v)
	}
	_, err := io.WriteString(w, out)
	return err
}

// withEmptyLists replaces nil lists with empty ones, so JSON readers see []
// rather than null.
func withEmptyLists(v verdict.Verdict) verdict.Verdict {
	v.Triggers = orEmpty(v.Triggers)
	v.RequiredReviewers = orEmpty(v.RequiredReviewers)
	v.Reasons = orEmpty(v.Reasons)
	if v.Score != nil {
		s := *v.Score
		s.Factors = orEmpty(s.Factors)
		s.Floors = orEmpty(s.Floors)
		s.Excluded = orEmpty(s.Excluded)
		s.Notes = orEmpty(s.Notes)
		v.Score = &s
	}
	return v
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// human renders a verdict for a person reading a terminal or a PR comment.
func human(v verdict.Verdict) string {
	var b strings.Builder
	fmt.Fprintf(&b, "risk: %s\nhuman review: %s\nevaluation: %s\n", v.Risk, v.HumanReview, v.Evaluation)
	if v.Error != "" {
		fmt.Fprintf(&b, "error: %s\n", v.Error)
	}
	fmt.Fprintf(&b, "triggers: %s\n", list(v.Triggers))
	fmt.Fprintf(&b, "required reviewers: %s\n", list(v.RequiredReviewers))
	if s := v.Score; s != nil {
		fmt.Fprintf(&b, "\nscore: %d points, %s tier, %s risk after floors\n", s.Points, s.Tier, s.Risk)
		for _, f := range s.Factors {
			if !f.Measured {
				fmt.Fprintf(&b, "  %-16s -  %s\n", f.Name, f.Detail)
				continue
			}
			fmt.Fprintf(&b, "  %-16s +%d (%d, band %s)  %s\n", f.Name, f.Points, f.Value, f.Band, f.Detail)
		}
		if len(s.Excluded) > 0 {
			fmt.Fprintf(&b, "  excluded: %s\n", strings.Join(s.Excluded, ", "))
		}
		for _, n := range s.Notes {
			fmt.Fprintf(&b, "  note: %s\n", n)
		}
	}
	if len(v.Reasons) > 0 {
		b.WriteString("\nreasons:\n")
	}
	for _, r := range v.Reasons {
		if r.File == "" {
			fmt.Fprintf(&b, "  %s: %s\n", r.Rule, r.Detail)
			continue
		}
		fmt.Fprintf(&b, "  %s: %s: %s\n", r.Rule, r.File, r.Detail)
	}
	return b.String()
}

func list(s []string) string {
	if len(s) == 0 {
		return "none"
	}
	return strings.Join(s, ", ")
}
