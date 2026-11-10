package verdict

import (
	"errors"
	"fmt"
	"slices"

	"github.com/zendern/priskinator/internal/changeset"
	"github.com/zendern/priskinator/internal/policy"
	"github.com/zendern/priskinator/internal/scoring"
	"github.com/zendern/priskinator/internal/triggers"
)

// Triggers the verdict itself can fire (ADR 0003), alongside the rule IDs
// from triggers and the policy layer's config-invalid and
// policy-weakening-attempt.
const (
	TriggerEvaluationError = "evaluation-error"
	TriggerOversize        = "oversize"
	TriggerAIUnavailable   = "ai-unavailable"
	// TriggerPolicyLoosening fires when the change's own policy file loosens
	// a field the base policy does not, so a human sees the loosening before it
	// takes effect (ADR 0010).
	TriggerPolicyLoosening = "policy-loosening"
)

var errNoPolicy = errors.New("no base policy to evaluate against")

// Evaluation is how completely the verdict was computed.
type Evaluation string

const (
	EvaluationOK       Evaluation = "ok"
	EvaluationDegraded Evaluation = "degraded"
	EvaluationError    Evaluation = "error"
)

// Human review requirements. Optional is reserved for change classes enabled
// under ADR 0004 option B, and none can be enabled yet.
const (
	HumanReviewRequired = "required"
	HumanReviewOptional = "optional"
)

// Verdict is the outcome of one evaluation, in the shape of ADR 0003.
type Verdict struct {
	Risk        policy.Risk `json:"risk"`
	HumanReview string      `json:"human_review"`
	// BotEligible and ChangeClass are computed and recorded but never acted on
	// under ADR 0004 option A.
	BotEligible       bool       `json:"bot_eligible"`
	ChangeClass       string     `json:"change_class,omitempty"`
	Triggers          []string   `json:"triggers"`
	RequiredReviewers []string   `json:"required_reviewers"`
	Reasons           []Reason   `json:"reasons"`
	Evaluation        Evaluation `json:"evaluation"`
	// Score is the scoring breakdown, nil when evaluation failed.
	Score *scoring.Result `json:"score,omitempty"`
	// Error says why evaluation failed or was degraded.
	Error string `json:"error,omitempty"`
}

// Reason is one thing that contributed to the verdict, with its evidence.
type Reason struct {
	Rule   string `json:"rule"`
	File   string `json:"file,omitempty"`
	Detail string `json:"detail"`
}

// AI is the optional AI assessment as data (ADR 0008). Runs holds every
// successful run for the current head SHA, so escalations are sticky: the
// verdict takes the highest risk and every finding across them, and a later,
// calmer run cannot remove an earlier escalation (ADR 0003).
type AI struct {
	Enabled bool
	Runs    []AIRun
	// Err is the latest failure when no run succeeded.
	Err error
}

// AIRun is one successful AI assessment.
type AIRun struct {
	Risk     policy.Risk
	Findings []AIFinding
}

// AIFinding is a trigger the AI raised. It requires human review like any
// other trigger.
type AIFinding struct {
	Rule   string
	Detail string
}

// Input is everything one evaluation reads, all of it data.
type Input struct {
	// Policy is the effective base-branch policy (policy.Resolve).
	Policy    *policy.Result
	ChangeSet changeset.ChangeSet
	// Generated is the base branch's linguist-generated and vendored paths.
	Generated scoring.Generated
	// Proposed holds the weakenings in the head's repo policy, when the change
	// edits it. Any not already in Policy.Weakenings fire policy-loosening.
	Proposed []policy.Weakening
	AI       AI
}

// Failed is the verdict when evaluation could not run: an engine error, an
// unreadable diff, or a missing base policy. It is high risk and needs a
// human (ADR 0003). A nil err still fails closed rather than panicking.
func Failed(err error) Verdict {
	msg := "evaluation failed"
	if err != nil {
		msg = err.Error()
	}
	return Verdict{
		Risk:        policy.RiskHigh,
		HumanReview: HumanReviewRequired,
		Triggers:    []string{TriggerEvaluationError},
		Reasons:     []Reason{{Rule: TriggerEvaluationError, Detail: msg}},
		Evaluation:  EvaluationError,
		Error:       msg,
	}
}

// Evaluate combines the mandatory triggers, the risk score, and the optional AI
// result into a verdict. The deterministic result is the floor; the AI can only
// raise risk and add triggers.
func Evaluate(in Input) Verdict {
	if in.Policy == nil || in.Policy.Policy == nil {
		return Failed(errNoPolicy)
	}
	p := in.Policy.Policy
	v := Verdict{Risk: policy.RiskLow, HumanReview: HumanReviewRequired, Evaluation: EvaluationOK}
	b := builder{v: &v}

	for _, h := range triggers.Evaluate(p, in.ChangeSet) {
		b.fire(h.Rule, h.Risk, h.Reviewers)
		for _, e := range h.Files {
			b.reason(h.Rule, e.Path, e.Reason)
		}
	}

	for _, id := range in.Policy.Triggers {
		b.fire(id, policy.RiskHigh, nil)
	}
	if in.Policy.ConfigErr != nil {
		b.reason(policy.TriggerConfigInvalid, "", "repo policy ignored: "+in.Policy.ConfigErr.Error())
	}
	for _, w := range in.Policy.Weakenings {
		b.reason(policy.TriggerPolicyWeakeningAttempt, w.Path, w.Message)
	}
	for _, w := range in.Proposed {
		if !slices.ContainsFunc(in.Policy.Weakenings, func(have policy.Weakening) bool {
			return have.Path == w.Path && have.Message == w.Message
		}) {
			b.fire(TriggerPolicyLoosening, policy.RiskHigh, nil)
			b.reason(TriggerPolicyLoosening, w.Path, w.Message)
		}
	}

	score := scoring.Score(p, in.ChangeSet, in.Generated)
	v.Score = &score
	b.raise(score.Risk)
	for _, fl := range score.Floors {
		for _, path := range fl.Paths {
			b.reason("floor", path, string(fl.Risk)+" floor: "+fl.Description)
		}
	}
	for _, f := range score.Factors {
		if f.Points > 0 {
			b.reason("score."+f.Name, "", f.Detail)
		}
	}

	// The caps count what scoring counts, so excluded lockfiles and generated
	// code don't make a dependency bump oversize.
	oversize := over(p.Limits.MaxChangedFiles, factorValue(score, scoring.FactorFiles), "changed files")
	oversize = append(oversize, over(p.Limits.MaxChangedLines, factorValue(score, scoring.FactorSize), "changed lines")...)
	for _, o := range oversize {
		b.fire(TriggerOversize, policy.RiskHigh, nil)
		b.reason(TriggerOversize, "", o)
	}

	// An oversize change skips the AI rather than degrading the evaluation.
	if in.AI.Enabled && len(oversize) == 0 {
		b.ai(in.AI)
	}

	slices.Sort(v.Triggers)
	v.BotEligible = eligible(v)
	return v
}

// eligible applies the ADR 0004 option B conditions. No change class can be
// matched yet, so it is always false; the other conditions are checked first
// so they stay tested when classes arrive.
func eligible(v Verdict) bool {
	if v.Risk != policy.RiskLow || len(v.Triggers) > 0 || v.Evaluation != EvaluationOK {
		return false
	}
	return v.ChangeClass != ""
}

type builder struct {
	v *Verdict
}

func (b builder) raise(r policy.Risk) {
	b.v.Risk = policy.MaxRisk(b.v.Risk, r)
}

func (b builder) fire(id string, r policy.Risk, reviewers []string) {
	if !slices.Contains(b.v.Triggers, id) {
		b.v.Triggers = append(b.v.Triggers, id)
	}
	b.raise(r)
	for _, rv := range reviewers {
		if !slices.Contains(b.v.RequiredReviewers, rv) {
			b.v.RequiredReviewers = append(b.v.RequiredReviewers, rv)
		}
	}
}

func (b builder) reason(rule, file, detail string) {
	b.v.Reasons = append(b.v.Reasons, Reason{Rule: rule, File: file, Detail: detail})
}

func (b builder) ai(ai AI) {
	if len(ai.Runs) == 0 {
		b.v.Evaluation = EvaluationDegraded
		detail := "AI assessment did not return a result"
		if ai.Err != nil {
			detail += ": " + ai.Err.Error()
		}
		b.v.Error = detail
		b.fire(TriggerAIUnavailable, policy.RiskLow, nil)
		b.reason(TriggerAIUnavailable, "", detail)
		return
	}
	for _, run := range ai.Runs {
		b.raise(run.Risk)
		for _, f := range run.Findings {
			b.fire(f.Rule, policy.RiskLow, nil)
			b.reason(f.Rule, "", f.Detail)
		}
	}
}

func factorValue(r scoring.Result, name string) int {
	value := 0
	for _, f := range r.Factors {
		if f.Name == name {
			value = f.Value
		}
	}
	return value
}

// over describes value exceeding limit, or returns nothing when it does not.
func over(limit *int, value int, what string) []string {
	if limit == nil || value <= *limit {
		return nil
	}
	return []string{fmt.Sprintf("%d %s, over the limit of %d", value, what, *limit)}
}
