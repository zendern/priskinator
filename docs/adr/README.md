# Architecture Decision Records

Decisions for pr-riskinator, recorded per [ADR 0001](0001-record-architecture-decisions.md). New records start from [template.md](template.md).

## Index

| ADR | Title | Status |
|---|---|---|
| [0001](0001-record-architecture-decisions.md) | Record architecture decisions as MADR files | accepted |
| [0002](0002-scope-and-threat-model.md) | Scope and threat model | accepted |
| [0003](0003-verdict-model.md) | Verdict model: deterministic floor, AI may only escalate, fail closed | accepted |
| [0004](0004-what-a-low-risk-verdict-permits.md) | What a low-risk verdict permits: human approval now, standard-change bot approval per class as the goal | accepted |
| [0005](0005-trust-boundary-and-enforcement.md) | Trust boundary and enforcement | accepted; decision 4 superseded by 0017 |
| [0006](0006-mandatory-human-review-triggers.md) | Mandatory human-review triggers: path plus content rules | accepted |
| [0007](0007-risk-and-complexity-scoring.md) | Risk and complexity scoring: transparent additive points | accepted |
| [0008](0008-ai-assessment-layer.md) | AI assessment layer: optional, provider-agnostic, escalate-only, fully configurable | accepted |
| [0009](0009-compliance-evidence-and-segregation-of-duties.md) | Compliance evidence, segregation of duties, waivers, break-glass | accepted |
| [0010](0010-policy-configuration-and-inheritance.md) | Policy configuration: layered, tighten-only, tested | accepted |
| [0011](0011-platform-and-distribution.md) | Platform and distribution: forge-agnostic core, GitHub first | accepted |
| [0012](0012-implementation-language.md) | Implementation language: Go, content rules through ast-grep | accepted |
| [0013](0013-rollout-and-calibration.md) | Rollout and calibration | accepted |
| [0014](0014-github-app-hosting-and-tenancy.md) | GitHub App hosting and tenancy: self-hosted per org, long-running container | superseded by 0017 |
| [0015](0015-responsible-user-for-automated-prs.md) | Responsible user for automated PRs: forge rules in v1, close the gap in v2 | accepted (v1), v2 deferred |
| [0016](0016-license.md) | License: Apache-2.0 | accepted |
| [0017](0017-actions-gate-with-oidc-token-broker.md) | Run the gate in GitHub Actions with a pinned reusable workflow and an OIDC token broker | accepted; confirmation by spike S4 pending |

## Open decisions

None.

## Deferred to v2

- **0015**: a verified responsible user for PRs opened by bot accounts. v1 relies on forge rules and flags bot-authored PRs in the evidence.

## Spikes

| Spike | Question | Status |
|---|---|---|
| S1 | Does a GitHub App's approval count toward required approvals? | Done: yes ([report](../spikes/s1-app-approval-counts.md)) |
| S2 | How to measure complexity change: ast-grep decision points, or lizard? | Open |
| S3 | How to sign attestations | Resolved by [0017](0017-actions-gate-with-oidc-token-broker.md): `actions/attest` in the gate job |
| S4 | Actions gate with octo-sts: attacker jobs and `pull_request` runs denied, fork PRs and the review relay work | Next. Confirms [0017](0017-actions-gate-with-oidc-token-broker.md) |

## GitHub behavior checks (2026-10-13)

- **Resolved:**
  - Required checks pinned to a dedicated App cannot be satisfied by any other source. Checks pinned to the GitHub Actions app cannot tell workflows apart. The required check is therefore pinned to the organization's own App, whose tokens come from the broker ([0017](0017-actions-gate-with-oidc-token-broker.md)). The App needs `statuses: write` to be an expected source.
  - The ruleset required-reviewer rule needs an organization, because user-owned repositories have no teams. Rulesets on private repositories need Pro, Team, or Enterprise Cloud ([0011](0011-platform-and-distribution.md)).
- **Resolved by spike S1:** a GitHub App's approving review does count toward required approvals, in rulesets and in classic protection, and a new push dismisses it ([report](../spikes/s1-app-approval-counts.md)). The behavior is undocumented, so option B in [0004](0004-what-a-low-risk-verdict-permits.md) also requires a scheduled canary with automatic fallback.
- **Still open:** whether an App installation with only `contents: read` still counts.

## Compliance scope

SOC 2 and ISO 27001. No GRC function or auditor is engaged yet. The control mapping in [0009](0009-compliance-evidence-and-segregation-of-duties.md) is a best-effort assumption and must be validated before the tool is relied on in an audit period. GRC sign-off is a hard gate for enabling option B of [0004](0004-what-a-low-risk-verdict-permits.md).

## Known data gaps

No incident data linked to PRs and no numeric targets yet. [0013](0013-rollout-and-calibration.md) uses proxy labels for the backtest and sets targets from shadow-phase data.
