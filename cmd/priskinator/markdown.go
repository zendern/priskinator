package main

import (
	"fmt"
	"strings"

	"github.com/zendern/priskinator/internal/policy"
	"github.com/zendern/priskinator/internal/scoring"
	"github.com/zendern/priskinator/internal/verdict"
)

// riskMarks give each risk level a colour that reads at a glance.
var riskMarks = map[policy.Risk]string{
	policy.RiskLow:    "🟢",
	policy.RiskMedium: "🟡",
	policy.RiskHigh:   "🔴",
}

// docsURL explains how to read a verdict.
const docsURL = "https://github.com/zendern/priskinator/blob/main/docs/verdict.md"

// markdown renders a verdict as GitHub-flavoured Markdown for a PR comment.
// Paths and details come from the change under review, so every one of them is
// escaped: a crafted file name can't add formatting, links or @mentions.
func markdown(v verdict.Verdict) string {
	var b strings.Builder
	mark := riskMarks[v.Risk]
	if v.Evaluation == verdict.EvaluationError {
		mark = "⚠️"
	}
	fmt.Fprintf(&b, "### %s %s risk, human review %s\n\n", mark, v.Risk, v.HumanReview)
	if v.Evaluation != verdict.EvaluationOK {
		fmt.Fprintf(&b, "> [!WARNING]\n> Evaluation %s: %s\n\n", v.Evaluation, text(v.Error))
	}
	fmt.Fprintf(&b, "**Triggers:** %s · **Required reviewers:** %s\n", rules(v), codes(v.RequiredReviewers))
	if s := v.Score; s != nil {
		writeScore(&b, v.Risk, s)
	}
	// The score table already shows each factor's detail.
	var reasons []verdict.Reason
	for _, r := range v.Reasons {
		if !strings.HasPrefix(r.Rule, "score.") {
			reasons = append(reasons, r)
		}
	}
	if len(reasons) > 0 {
		fmt.Fprintf(&b, "\n<details>\n<summary>Reasons (%d)</summary>\n\n| Rule | File | Detail |\n|---|---|---|\n", len(reasons))
		for _, r := range reasons {
			file := ""
			if r.File != "" {
				file = code(r.File)
			}
			fmt.Fprintf(&b, "| %s | %s | %s |\n", code(r.Rule), file, text(r.Detail))
		}
		b.WriteString("\n</details>\n")
	}
	return b.String()
}

// rules lists the fired rules with what each is for, or "none".
func rules(v verdict.Verdict) string {
	if len(v.Triggers) == 0 {
		return "none"
	}
	out := make([]string, len(v.Triggers))
	for i, id := range v.Triggers {
		out[i] = code(id)
		if d := v.Descriptions[id]; d != "" {
			out[i] += " (" + text(d) + ")"
		}
	}
	return strings.Join(out, ", ")
}

// writeScore says whether the score or something above it set the risk, then
// shows only the factors that added points: the bands behind them are policy
// detail that the JSON keeps.
func writeScore(b *strings.Builder, risk policy.Risk, s *scoring.Result) {
	decided := "the score."
	if risk != s.Risk {
		decided = fmt.Sprintf("the triggers above. On its own the score is %s risk.", s.Risk)
	}
	fmt.Fprintf(b, "\n**Decided by:** %s\n", decided)
	fmt.Fprintf(b, "\n**Score:** %d points", s.Points)
	if th := s.Thresholds; th != nil {
		fmt.Fprintf(b, "; medium starts at %d and high at %d", th.Medium, th.High)
	}
	b.WriteString("\n")
	table := "\n| Factor | Points | Detail |\n|---|--:|---|\n"
	for _, f := range s.Factors {
		if f.Points > 0 {
			fmt.Fprintf(b, "%s| %s | +%d | %s |\n", table, f.Name, f.Points, text(f.Detail))
			table = ""
		}
	}
	fmt.Fprintf(b, "\n<sub>Factors that added no points are hidden. [How priskinator scores a PR](%s)</sub>\n", docsURL)
	for _, fl := range s.Floors {
		fmt.Fprintf(b, "\nFloor: %s raises the risk to %s for %s\n", text(fl.Description), fl.Risk, codes(fl.Paths))
	}
	if len(s.Excluded) > 0 {
		fmt.Fprintf(b, "\nExcluded from scoring: %s\n", codes(s.Excluded))
	}
	for _, n := range s.Notes {
		fmt.Fprintf(b, "\nNote: %s\n", text(n))
	}
}

// codes renders a list as inline code spans, or "none".
func codes(s []string) string {
	if len(s) == 0 {
		return "none"
	}
	out := make([]string, len(s))
	for i, x := range s {
		out[i] = code(x)
	}
	return strings.Join(out, ", ")
}

// code renders s as an inline code span, fenced with one more backtick than
// the longest run inside it so s can't close the span early. Pipes are escaped
// because GitHub splits table cells on them even inside code.
func code(s string) string {
	s = oneLine(s)
	run, longest := 0, 0
	for _, r := range s {
		run++
		if r != '`' {
			run = 0
		}
		longest = max(longest, run)
	}
	fence := strings.Repeat("`", longest+1)
	return fence + " " + strings.ReplaceAll(s, "|", `\|`) + " " + fence
}

// markdownEscaper backslash-escapes Markdown punctuation, turns HTML
// delimiters into entities, and writes @ as an entity so no mention pings.
var markdownEscaper = strings.NewReplacer(
	`\`, `\\`, "`", "\\`", `*`, `\*`, `_`, `\_`, `[`, `\[`, `]`, `\]`,
	`#`, `\#`, `|`, `\|`, `~`, `\~`, `!`, `\!`,
	`<`, "&lt;", `>`, "&gt;", `&`, "&amp;", `@`, "&#64;",
)

// text renders s as plain text on one line.
func text(s string) string {
	return markdownEscaper.Replace(oneLine(s))
}

// lineBreaks would end a table row or a code span early.
var lineBreaks = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ")

func oneLine(s string) string {
	return lineBreaks.Replace(s)
}
