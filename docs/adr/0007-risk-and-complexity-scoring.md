---
status: accepted
date: 2026-10-13
decision-makers: Nathan Zender
---

# Risk and complexity scoring: transparent additive points model

## Context and Problem Statement

PRs that hit no mandatory trigger still differ greatly in risk. A one-line copy change and a 900-line refactor of a hot billing path should not get the same review. The tier must be deterministic and explainable to an auditor and to the author. What inputs feed the score, and how are they combined?

## Decision Drivers

- Explainability: every point in the score must trace to a named factor and a threshold.
- Determinism: frozen weights, versioned with the policy.
- Grade by risk, not size. Rewind's experience is that blast radius matters more than line count.
- Grounded in research: just-in-time (JIT) defect prediction, following Kamei et al. (2013), groups change metrics into size, diffusion, history, and experience. Studies report simple churn-based models perform close to complex ones.

## Considered Options

1. Size thresholds only (lines and files changed)
2. Transparent additive points model with documented factors and weights
3. Trained ML model (for example logistic regression on JIT features)
4. AI-assigned risk

## Decision Outcome

Chosen option: "Transparent additive points model", because it is explainable line by line and deterministic, and it can absorb weights learned later from data (option 3) without changing its shape.

### Factors

| Factor | Source | Notes |
|---|---|---|
| Size | Lines added and deleted, files changed | Excludes generated, vendored, and lockfiles, identified from base-branch `.gitattributes` (`linguist-generated`, `linguist-vendored`) and policy globs. Low weight. |
| Diffusion | Number of top-level directories or declared subsystems touched; entropy of the change distribution across files | High diffusion means the change is harder to review as one unit. |
| Complexity delta | Per changed function: cyclomatic complexity before vs after, maximum nesting | Scores the change in complexity, not the absolute value, so legacy code does not dominate. |
| Criticality floors | Policy-defined path classes: DB migrations, IaC, IAM, payments, public API schemas | A floor sets a minimum tier instead of adding points. |
| Test signal | Non-test source lines changed with no test change | Adds points; never subtracts them. |
| History (optional) | File churn over the last N days, count of prior bug-fix commits touching the file (hotspots) | Needs git history access. Off by default until performance is measured. |

Author and initiator experience (the EXP metrics in Kamei et al.) are deliberately excluded. They are undefined for agent authors. The initiator's experience is a weak proxy for the quality of an agent's change, and the factor would penalize new contributors.

### Combination

- Each factor maps its raw value to points through documented threshold bands in the policy, for example complexity delta `0–5: 0`, `6–15: 10`, `>15: 25`.
- Total points map to `low`, `medium`, or `high` through policy thresholds.
- Criticality floors and trigger floors (ADR 0006) are applied after the sum: `risk_det = max(tier(points), floors)`.
- Diffs above a hard size cap skip scoring and fire the `oversize` trigger (ADR 0003).

### Output

The verdict lists every factor with raw value, points, and threshold band, so the author can see exactly why a PR is medium and what would make it low (for example, split the PR).

### Consequences

- Good, because the score is fully explainable and reproducible.
- Good, because weights live in versioned policy and change only through reviewed PRs.
- Bad, because hand-set weights start as guesses. Backtesting on historical PRs and incidents (ADR 0013) calibrates them.
- Bad, because complexity analysis needs per-language support. Languages without a parser fall back to size and diffusion only, and the verdict says so.

### Confirmation

- Golden fixtures for each factor's threshold bands.
- A backtest report (ADR 0013) showing tier distribution and incident-linked PRs by tier before enforcement.

## Pros and Cons of the Options

### Size only

- Good, because it is trivial and transparent.
- Bad, because it grades the wrong thing: small, dangerous changes score low.

### Additive points

- Good, because it is explainable and deterministic, and can take learned weights.
- Bad, because initial weights are judgment calls.

### Trained ML model

- Good, because weights come from data.
- Bad, because a model is harder to explain to auditors, needs labeled defect data per org, and drifts. It is worth revisiting as a weight-fitting step that outputs a frozen points table.

### AI-assigned risk

- Good, because it captures semantics.
- Bad, because it is not deterministic. It belongs in the escalate-only layer (ADR 0003, ADR 0008).

## More Information

- Kamei et al., JIT quality assurance: <https://posl.ait.kyushu-u.ac.jp/~kamei/publications/Kamei_EMSE2015.pdf>
- Building blocks to evaluate: lizard (multi-language cyclomatic complexity), scc, PyDriller (history mining).
