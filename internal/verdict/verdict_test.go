package verdict

import (
	"errors"
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"

	"github.com/zendern/priskinator/internal/changeset"
	"github.com/zendern/priskinator/internal/fixture"
	"github.com/zendern/priskinator/internal/fixture/changes"
	"github.com/zendern/priskinator/internal/policy"
)

func TestFixtures(t *testing.T) {
	fixture.Run(t, "testdata", func(f fixture.Fixture) (fixture.Result, error) {
		res, err := policy.Resolve(nil, f.PolicyYAML())
		if err != nil {
			return fixture.Result{}, err
		}
		v := Evaluate(Input{Policy: res, ChangeSet: changes.Build(f.Change)})
		return fixture.Result{Risk: string(v.Risk), HumanReview: v.HumanReview, Triggers: v.Triggers, Reviewers: v.RequiredReviewers}, nil
	})
}

func resolved(t *testing.T, repo string) *policy.Result {
	t.Helper()
	var data []byte
	if repo != "" {
		data = []byte(repo)
	}
	res, err := policy.Resolve(nil, data)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func lines(n int) []changeset.Line {
	out := make([]changeset.Line, n)
	for i := range out {
		out[i] = changeset.Line{Line: i + 1, Text: "x"}
	}
	return out
}

func file(path string, added int) changeset.File {
	return changeset.File{Path: path, Status: changeset.StatusModified, Hunks: []changeset.Hunk{{Added: lines(added)}}}
}

func docs() changeset.ChangeSet {
	return changeset.ChangeSet{Files: []changeset.File{file("docs/a.md", 3)}}
}

func hasReason(v Verdict, want Reason) bool { return slices.Contains(v.Reasons, want) }

func TestFailed(t *testing.T) {
	v := Failed(errors.New("git diff: exit 128"))
	want := Verdict{
		Risk:        policy.RiskHigh,
		HumanReview: HumanReviewRequired,
		Triggers:    []string{TriggerEvaluationError},
		Reasons:     []Reason{{Rule: TriggerEvaluationError, Detail: "git diff: exit 128"}},
		Evaluation:  EvaluationError,
		Error:       "git diff: exit 128",
	}
	if !reflect.DeepEqual(v, want) {
		t.Errorf("Failed =\n%+v\nwant\n%+v", v, want)
	}
}

func TestFailedNilError(t *testing.T) {
	v := Failed(nil)
	if v.Risk != policy.RiskHigh || v.Evaluation != EvaluationError || v.Error != "evaluation failed" ||
		v.Reasons[0].Detail != "evaluation failed" {
		t.Errorf("Failed(nil) = %+v", v)
	}
}

func TestEvaluateWithoutPolicyFails(t *testing.T) {
	for name, res := range map[string]*policy.Result{"nil result": nil, "nil policy": {}} {
		v := Evaluate(Input{Policy: res, ChangeSet: docs()})
		if v.Evaluation != EvaluationError || v.Risk != policy.RiskHigh || v.Error != "no base policy to evaluate against" {
			t.Errorf("%s: %+v", name, v)
		}
	}
}

func TestEvaluateLowChange(t *testing.T) {
	v := Evaluate(Input{Policy: resolved(t, ""), ChangeSet: docs()})
	if v.Risk != policy.RiskLow || len(v.Triggers) != 0 || v.Evaluation != EvaluationOK || v.HumanReview != HumanReviewRequired {
		t.Errorf("verdict = %+v", v)
	}
	if v.Score == nil || v.Score.Points != 0 || v.Reasons != nil || v.RequiredReviewers != nil {
		t.Errorf("score %+v, reasons %+v, reviewers %v", v.Score, v.Reasons, v.RequiredReviewers)
	}
	if v.BotEligible || v.ChangeClass != "" {
		t.Error("eligible for bot approval with no change class")
	}
}

func TestEvaluateCollectsTriggerReasonsAndReviewers(t *testing.T) {
	cs := changeset.ChangeSet{Files: []changeset.File{
		file(".github/workflows/ci.yml", 1),
		file(".github/workflows/release.yml", 1),
		file("internal/auth/token.go", 1),
	}}
	v := Evaluate(Input{Policy: resolved(t, ""), ChangeSet: cs})
	if !slices.Equal(v.Triggers, []string{"G1.ci-workflows", "G7.security-paths"}) {
		t.Errorf("triggers = %v", v.Triggers)
	}
	if !slices.Equal(v.RequiredReviewers, []string{"platform", "security"}) {
		t.Errorf("reviewers = %v", v.RequiredReviewers)
	}
	for _, want := range []Reason{
		{Rule: "G1.ci-workflows", File: ".github/workflows/ci.yml", Detail: `matches ".github/workflows/**"`},
		{Rule: "G1.ci-workflows", File: ".github/workflows/release.yml", Detail: `matches ".github/workflows/**"`},
		{Rule: "G7.security-paths", File: "internal/auth/token.go", Detail: `matches "**/auth/**"`},
	} {
		if !hasReason(v, want) {
			t.Errorf("missing reason %+v in %+v", want, v.Reasons)
		}
	}
}

func TestEvaluateReportsScoreFactorsAndFloors(t *testing.T) {
	res := resolved(t, "version: 1\nscoring:\n  floors:\n    - {description: Migrations, paths: [\"db/migrations/**\"], risk: high}\n")
	cs := changeset.ChangeSet{Files: []changeset.File{file("db/migrations/1.sql", 1), file("app/main.go", 60)}}
	v := Evaluate(Input{Policy: res, ChangeSet: cs})
	if v.Risk != policy.RiskHigh {
		t.Errorf("risk = %s", v.Risk)
	}
	for _, want := range []Reason{
		{Rule: "floor", File: "db/migrations/1.sql", Detail: "high floor: Migrations"},
		{Rule: "score.size", Detail: "61 added and 0 removed lines in 2 files"},
		{Rule: "score.test_signal", Detail: "60 source lines changed with no test change"},
	} {
		if !hasReason(v, want) {
			t.Errorf("missing reason %+v in %+v", want, v.Reasons)
		}
	}
	for _, r := range v.Reasons {
		if r.Rule == "score.files" || r.Rule == "score.complexity_delta" {
			t.Errorf("factor with no points listed: %+v", r)
		}
	}
}

func TestEvaluatePolicyLayerTriggers(t *testing.T) {
	v := Evaluate(Input{Policy: resolved(t, "version: 1\nbogus: 1\n"), ChangeSet: docs()})
	if !slices.Equal(v.Triggers, []string{policy.TriggerConfigInvalid}) || v.Risk != policy.RiskHigh {
		t.Errorf("config-invalid: %+v", v)
	}
	if len(v.Reasons) != 1 || v.Reasons[0].Rule != policy.TriggerConfigInvalid || v.Reasons[0].Detail[:21] != "repo policy ignored: " {
		t.Errorf("reasons = %+v", v.Reasons)
	}

	res := &policy.Result{
		Policy:     policy.Defaults(),
		Triggers:   []string{policy.TriggerPolicyWeakeningAttempt},
		Weakenings: []policy.Weakening{{Rule: "G1.ci-workflows", Path: "/triggers/G1.ci-workflows/enabled", Message: "disables a baseline rule"}},
	}
	v = Evaluate(Input{Policy: res, ChangeSet: docs()})
	if !slices.Equal(v.Triggers, []string{policy.TriggerPolicyWeakeningAttempt}) || v.Risk != policy.RiskHigh {
		t.Errorf("weakening: %+v", v)
	}
	want := Reason{Rule: policy.TriggerPolicyWeakeningAttempt, File: "/triggers/G1.ci-workflows/enabled", Detail: "disables a baseline rule"}
	if !hasReason(v, want) {
		t.Errorf("missing %+v in %+v", want, v.Reasons)
	}
}

func TestEvaluateProposedLoosening(t *testing.T) {
	inForce := policy.Weakening{Path: "/limits/max_file_bytes", Message: "raises 1 to 2"}
	res := &policy.Result{Policy: policy.Defaults(), Weakenings: []policy.Weakening{inForce}}

	v := Evaluate(Input{Policy: res, ChangeSet: docs(), Proposed: []policy.Weakening{inForce}})
	if slices.Contains(v.Triggers, TriggerPolicyLoosening) {
		t.Error("a loosening already in force fired policy-loosening")
	}

	added := []policy.Weakening{
		inForce,
		{Path: "/limits/max_file_bytes", Message: "raises 1 to 3"},
		{Path: "/triggers/G7.security-paths/paths", Message: "removes baseline paths: [**/session*/**]"},
	}
	v = Evaluate(Input{Policy: res, ChangeSet: docs(), Proposed: added})
	if !slices.Equal(v.Triggers, []string{TriggerPolicyLoosening}) || v.Risk != policy.RiskHigh {
		t.Errorf("verdict = %+v", v)
	}
	var got []Reason
	for _, r := range v.Reasons {
		if r.Rule == TriggerPolicyLoosening {
			got = append(got, r)
		}
	}
	want := []Reason{
		{Rule: TriggerPolicyLoosening, File: "/limits/max_file_bytes", Detail: "raises 1 to 3"},
		{Rule: TriggerPolicyLoosening, File: "/triggers/G7.security-paths/paths", Detail: "removes baseline paths: [**/session*/**]"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reasons = %+v, want %+v", got, want)
	}
}

func TestEvaluateOversize(t *testing.T) {
	res := resolved(t, "version: 1\nlimits: {max_changed_files: 2, max_changed_lines: 10}\n")
	ai := AI{Enabled: true}

	v := Evaluate(Input{Policy: res, ChangeSet: changeset.ChangeSet{Files: []changeset.File{file("a.md", 10), file("b.md", 0)}}, AI: ai})
	if slices.Contains(v.Triggers, TriggerOversize) {
		t.Errorf("at the limits: %+v", v.Triggers)
	}
	if v.Evaluation != EvaluationDegraded {
		t.Error("the AI was skipped for a change within the limits")
	}

	cs := changeset.ChangeSet{Files: []changeset.File{file("a.md", 9), file("b.md", 1), file("c.md", 1)}}
	v = Evaluate(Input{Policy: res, ChangeSet: cs, AI: ai})
	if !slices.Equal(v.Triggers, []string{TriggerOversize}) || v.Risk != policy.RiskHigh {
		t.Errorf("verdict = %+v", v)
	}
	want := []Reason{
		{Rule: TriggerOversize, Detail: "3 changed files, over the limit of 2"},
		{Rule: TriggerOversize, Detail: "11 changed lines, over the limit of 10"},
	}
	for _, w := range want {
		if !hasReason(v, w) {
			t.Errorf("missing %+v in %+v", w, v.Reasons)
		}
	}
	if v.Evaluation != EvaluationOK {
		t.Errorf("oversize should skip the AI, not degrade: %s", v.Evaluation)
	}

	p := policy.Defaults()
	p.Limits = policy.Limits{}
	v = Evaluate(Input{Policy: &policy.Result{Policy: p}, ChangeSet: cs})
	if slices.Contains(v.Triggers, TriggerOversize) {
		t.Error("oversize fired with no limits")
	}
}

func TestEvaluateAI(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		v := Evaluate(Input{Policy: resolved(t, ""), ChangeSet: docs(), AI: AI{Runs: []AIRun{{Risk: policy.RiskHigh}}}})
		if v.Risk != policy.RiskLow {
			t.Errorf("a disabled AI changed the risk to %s", v.Risk)
		}
	})
	t.Run("unavailable", func(t *testing.T) {
		v := Evaluate(Input{Policy: resolved(t, ""), ChangeSet: docs(), AI: AI{Enabled: true, Err: errors.New("timeout")}})
		if v.Evaluation != EvaluationDegraded || !slices.Equal(v.Triggers, []string{TriggerAIUnavailable}) || v.BotEligible {
			t.Errorf("verdict = %+v", v)
		}
		if v.Risk != policy.RiskLow {
			t.Errorf("ai-unavailable changed the risk to %s", v.Risk)
		}
		want := "AI assessment did not return a result: timeout"
		if v.Error != want || !hasReason(v, Reason{Rule: TriggerAIUnavailable, Detail: want}) {
			t.Errorf("error %q, reasons %+v", v.Error, v.Reasons)
		}
	})
	t.Run("unavailable without an error", func(t *testing.T) {
		v := Evaluate(Input{Policy: resolved(t, ""), ChangeSet: docs(), AI: AI{Enabled: true}})
		if v.Error != "AI assessment did not return a result" {
			t.Errorf("error = %q", v.Error)
		}
	})
	t.Run("escalations are sticky across runs", func(t *testing.T) {
		ai := AI{Enabled: true, Err: errors.New("later run failed"), Runs: []AIRun{
			{Risk: policy.RiskMedium, Findings: []AIFinding{{Rule: "ai.description-mismatch", Detail: "body says docs only"}}},
			{Risk: policy.RiskLow},
		}}
		v := Evaluate(Input{Policy: resolved(t, ""), ChangeSet: docs(), AI: ai})
		if v.Risk != policy.RiskMedium || !slices.Equal(v.Triggers, []string{"ai.description-mismatch"}) || v.Evaluation != EvaluationOK {
			t.Errorf("verdict = %+v", v)
		}
		if !hasReason(v, Reason{Rule: "ai.description-mismatch", Detail: "body says docs only"}) {
			t.Errorf("reasons = %+v", v.Reasons)
		}
	})
}

// Property test (ADR 0003, Confirmation): no AI input lowers the risk or
// removes a trigger.
func TestAINeverLowersTheVerdict(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	risks := []policy.Risk{policy.RiskLow, policy.RiskMedium, policy.RiskHigh}
	paths := []string{"docs/a.md", ".github/workflows/ci.yml", "internal/auth/x.go", ".golangci.yml", "app/main.go", "go.sum"}
	res := resolved(t, "")
	for i := range 500 {
		var cs changeset.ChangeSet
		for _, p := range paths {
			if rng.IntN(3) == 0 {
				cs.Files = append(cs.Files, file(p, rng.IntN(400)))
			}
		}
		det := Evaluate(Input{Policy: res, ChangeSet: cs})

		ai := AI{Enabled: true}
		if rng.IntN(4) == 0 {
			ai.Err = errors.New("failed")
		} else {
			for range rng.IntN(3) + 1 {
				run := AIRun{Risk: risks[rng.IntN(3)]}
				if rng.IntN(2) == 0 {
					run.Findings = []AIFinding{{Rule: "ai.finding", Detail: "x"}}
				}
				ai.Runs = append(ai.Runs, run)
			}
		}
		got := Evaluate(Input{Policy: res, ChangeSet: cs, AI: ai})

		if got.Risk.Rank() < det.Risk.Rank() {
			t.Fatalf("case %d: AI lowered risk from %s to %s", i, det.Risk, got.Risk)
		}
		for _, tr := range det.Triggers {
			if !slices.Contains(got.Triggers, tr) {
				t.Fatalf("case %d: AI removed trigger %s", i, tr)
			}
		}
	}
}

func TestEligible(t *testing.T) {
	ok := Verdict{Risk: policy.RiskLow, Evaluation: EvaluationOK, ChangeClass: "docs-only"}
	if !eligible(ok) {
		t.Error("a low, trigger-free, ok verdict in a change class is not eligible")
	}
	for name, v := range map[string]Verdict{
		"medium risk":     {Risk: policy.RiskMedium, Evaluation: EvaluationOK, ChangeClass: "docs-only"},
		"a trigger":       {Risk: policy.RiskLow, Evaluation: EvaluationOK, ChangeClass: "docs-only", Triggers: []string{"G10.binary"}},
		"degraded":        {Risk: policy.RiskLow, Evaluation: EvaluationDegraded, ChangeClass: "docs-only"},
		"no change class": {Risk: policy.RiskLow, Evaluation: EvaluationOK},
	} {
		if eligible(v) {
			t.Errorf("%s: eligible", name)
		}
	}
}
