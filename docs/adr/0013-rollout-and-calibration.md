---
status: accepted
date: 2026-10-13
decision-makers: Nathan Zender
---

# Rollout and calibration: backtest, shadow, advise, enforce

## Context and Problem Statement

Rule lists, weights, and thresholds start as judgment calls. Turned on as a blocking gate on day one, a miscalibrated tool will either flood reviewers (who then rubber-stamp) or miss what it exists to catch. Auditors also expect a control to be designed, tested, and then operated, with evidence of each step. How is the tool introduced and tuned?

## Decision Drivers

- False positives erode trust and push reviewers toward rubber-stamping, which auditors flag.
- Calibration needs data: historical PRs, incidents, reverts.
- Each phase should produce evidence that the control was tested before it was relied on.
- Tuning is a policy change and goes through review. The tool never tunes itself.

## Considered Options

1. Enforce immediately
2. Phased rollout with measured exit criteria per phase

## Decision Outcome

Chosen option: "Phased rollout with measured exit criteria", because it produces calibrated thresholds and design-effectiveness evidence before the tool can block anyone.

### Phases

| Phase | Behavior | Exit criteria (initial proposal) |
|---|---|---|
| 0. Backtest | Run `simulate` over the last 6–12 months of merged PRs. With no incident data, use proxy labels (see "Data we do not have yet"). | Proxy-labeled risky PRs are mostly medium or higher, and each exception has a written explanation. Weekly trigger volume is recorded as the first estimate of reviewer load. |
| 1. Shadow | Runs on every PR. The check reports neutral and nothing is posted to the PR. Evidence records are written. | Four weeks of data. Trigger false-positive rate below an agreed target, measured by sampling. No evaluation errors left unexplained. |
| 2. Advisory | Posts the verdict, labels, and reviewer requests. The check is visible but not required. Reviewers react to findings (useful, not useful, unsure). | Reviewer "not useful" rate below an agreed target. AI escalation precision measured. |
| 3. Enforcing | Required check pinned to the App (ADR 0005). Forge approver-independence settings verified (ADR 0009). Break-glass live. | Ongoing. Reviewed monthly. |
| 4. Standard-change approval | Target end state per ADR 0004. Enabled one change class at a time. Shadow eligibility (`bot_eligible`) has been recorded since phase 1. | Per class: shadow-eligible PRs show an escape rate below an agreed threshold over a defined period, GitHub App approvals confirmed to satisfy required approvals, and GRC sign-off. |

Ruleset "evaluate" mode is used where available to stage the forge-side settings alongside phases 1 and 2.

Phases 0 to 2 need no branch protection or rulesets, so they run on any repository, including private repositories on a free plan. Only phase 3 needs a plan that supports required status checks.

### Data we do not have yet

There is no incident data linked to PRs and no agreed numeric targets. Exit criteria therefore start as placeholders, and the numbers come from the data the earlier phases produce:

- **Proxy risk labels for the backtest:**
  - revert commits
  - SZZ-style identification of the changes that later bug-fix commits had to correct (PyDriller has helpers for this)
  - hotfix or incident labels, where they exist
- **Collected from day one:** a `priskinator/incident` label, or the `/priskinator incident` comment command, links a merged PR to an incident. The tool records it against the PR's evidence, building the escape-rate dataset going forward.
- **Targets:**
  - The false-positive target and the weekly reviewer-capacity target are set at the end of phase 1, from shadow data.
  - The escape-rate threshold for ADR 0004 option B is set per change class, once enough labeled history exists for that class.

### Metrics

- Trigger hit rate and sampled false-positive rate, per rule ID
- Tier distribution, and time-to-merge per tier
- Per change class: share of PRs that are shadow-eligible, and how human reviewers treated them (changes requested, comments, reverts)
- AI escalation rate and precision
- Escape rate: incidents or reverts linked to PRs, by tier at merge time
- Rubber-stamp signals (ADR 0009)
- Break-glass and waiver use
- Cost and latency per PR

### Feedback loop

- A weekly digest of metrics and reviewer reactions goes to the policy owners.
- Accepted changes become ordinary PRs against the central policy, with `simulate` impact output attached (ADR 0010).
- Prompts, weights, and rules are never changed automatically.

### Consequences

- Good, because enforcement starts with calibrated thresholds and documented design testing.
- Good, because shadow and advisory phases produce the evidence auditors expect for a new control.
- Bad, because the time from first code to enforcement is weeks, not days.
- Bad, because until real incident data accumulates, calibration rests on proxy labels (reverts, later fixes), which miss incidents that never produced a revert or fix commit.

### Confirmation

Each phase transition is a recorded decision (PR or ADR update) with its metrics attached.

## More Information

- Rewind's lessons include modeling effectiveness on historical data before building, and keeping humans in charge of the feedback loop: <https://rewind.com/blog/ai-approve-pull-requests-safely/>
- A related view, that blocking policy should come only after a finding proves useful and low-noise in a given repo: <https://dev.to/sjh9714/llm-reviewers-are-useful-but-some-pr-checks-should-stay-deterministic-4k1e>
