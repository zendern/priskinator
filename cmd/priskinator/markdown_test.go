package main

import (
	"errors"
	"testing"

	"github.com/zendern/priskinator/internal/policy"
	"github.com/zendern/priskinator/internal/scoring"
	"github.com/zendern/priskinator/internal/verdict"
)

func TestMarkdownCoversEverySection(t *testing.T) {
	v := verdict.Verdict{
		Risk:              policy.RiskMedium,
		HumanReview:       "required",
		Triggers:          []string{"G7.security-paths"},
		RequiredReviewers: []string{"security"},
		Reasons: []verdict.Reason{
			{Rule: "G7.security-paths", File: "auth/login.go", Detail: `matches "auth/**"`},
			{Rule: "score.size", Detail: "60 added and 0 removed lines in 1 file"},
		},
		Evaluation: verdict.EvaluationDegraded,
		Error:      "AI assessment unavailable",
		Score: &scoring.Result{
			Points: 5, Tier: policy.RiskLow, Risk: policy.RiskMedium,
			Factors: []scoring.Factor{
				{Name: "size", Measured: true, Value: 60, Points: 5, Band: "51-200", Detail: "60 lines"},
				{Name: "complexity_delta", Detail: "not measured"},
			},
			Floors:   []scoring.FloorHit{{Description: "auth code", Risk: policy.RiskMedium, Paths: []string{"auth/login.go"}}},
			Excluded: []string{"go.sum"},
			Notes:    []string{"a note"},
		},
	}
	golden(t, "markdown-all.md", markdown(v))
}

func TestMarkdownEvaluationError(t *testing.T) {
	golden(t, "markdown-error.md", markdown(withEmptyLists(verdict.Failed(errors.New("git failed")))))
}

func TestMarkdownRiskMarks(t *testing.T) {
	for risk, want := range map[policy.Risk]string{
		policy.RiskLow:    "### 🟢 low risk, human review not required\n",
		policy.RiskMedium: "### 🟡 medium risk, human review not required\n",
		policy.RiskHigh:   "### 🔴 high risk, human review not required\n",
	} {
		got := markdown(verdict.Verdict{Risk: risk, HumanReview: "not required", Evaluation: verdict.EvaluationOK})
		if first := got[:len(want)]; first != want {
			t.Errorf("risk %s heading = %q, want %q", risk, first, want)
		}
	}
}

func TestCode(t *testing.T) {
	for in, want := range map[string]string{
		"a.go":          "` a.go `",
		"a`b":           "`` a`b ``",
		"a``b`c":        "``` a``b`c ```",
		"x|y":           "` x\\|y `",
		"line\nbreak":   "` line break `",
		"cr\r\nlf":      "` cr lf `",
		"`":             "`` ` ``",
		"no ` run ``  ": "``` no ` run ``   ```",
	} {
		if got := code(in); got != want {
			t.Errorf("code(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTextEscapesMarkdown(t *testing.T) {
	for in, want := range map[string]string{
		"plain words":                "plain words",
		`matches "src/**"`:           `matches "src/\*\*"`,
		"hi @octocat":                "hi &#64;octocat",
		"[link](http://x)":           `\[link\](http://x)`,
		"<img src=x>":                "&lt;img src=x&gt;",
		"a & b":                      "a &amp; b",
		"_e_ `c` #h ~s~ !b | \\":     `\_e\_ \` + "`c\\`" + ` \#h \~s\~ \!b \| \\`,
		"two\nlines\rand\r\nwindows": "two lines and windows",
	} {
		if got := text(in); got != want {
			t.Errorf("text(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCodesNone(t *testing.T) {
	if got := codes(nil); got != "none" {
		t.Errorf("codes(nil) = %q, want none", got)
	}
	if got := codes([]string{"a", "b"}); got != "` a `, ` b `" {
		t.Errorf("codes = %q", got)
	}
}
