# Architecture Decision Records

Decisions for pr-riskinator, recorded per [ADR 0001](0001-record-architecture-decisions.md). New records start from [template.md](template.md).

## Index

| ADR | Title | Status |
|---|---|---|
| [0001](0001-record-architecture-decisions.md) | Record architecture decisions as MADR files | accepted |
| [0002](0002-scope-and-threat-model.md) | Scope and threat model | accepted |
| [0003](0003-verdict-model.md) | Verdict model: deterministic floor, AI may only escalate, fail closed | accepted |
| [0004](0004-what-a-low-risk-verdict-permits.md) | What a low-risk verdict permits: human approval now, standard-change bot approval per class as the goal | accepted |
| [0005](0005-trust-boundary-and-enforcement.md) | Trust boundary and enforcement | accepted |
| [0006](0006-mandatory-human-review-triggers.md) | Mandatory human-review triggers: path plus content rules | accepted |
| [0007](0007-risk-and-complexity-scoring.md) | Risk and complexity scoring: transparent additive points | accepted |
| [0008](0008-ai-assessment-layer.md) | AI assessment layer: optional, provider-agnostic, escalate-only, fully configurable | accepted |
| [0009](0009-compliance-evidence-and-segregation-of-duties.md) | Compliance evidence, segregation of duties, waivers, break-glass | accepted |
| [0010](0010-policy-configuration-and-inheritance.md) | Policy configuration: layered, tighten-only, tested | accepted |
| [0011](0011-platform-and-distribution.md) | Platform and distribution: forge-agnostic core, GitHub first | accepted |
| [0012](0012-implementation-language.md) | Implementation language: Go, content rules through ast-grep | accepted |
| [0013](0013-rollout-and-calibration.md) | Rollout and calibration | accepted |
| [0014](0014-github-app-hosting-and-tenancy.md) | GitHub App hosting and tenancy: self-hosted per org, long-running container | accepted |
| [0015](0015-responsible-user-for-automated-prs.md) | Responsible user for automated PRs: forge rules in v1, close the gap in v2 | accepted (v1), v2 deferred |
| [0016](0016-license.md) | License: Apache-2.0 | accepted |

## Open decisions

None.

## Deferred to v2

- **0015**: a verified responsible user for PRs opened by bot accounts. v1 relies on forge rules and flags bot-authored PRs in the evidence.

## Items to verify against current GitHub behavior

- Whether a GitHub App's approval review counts toward ruleset required approvals and required-reviewer rules (0004). Blocks enabling option B for any change class.
- Whether check runs from different workflows under the GitHub Actions app can satisfy a required check by name (0005).
- Plan availability of the ruleset required-reviewer rule (0011).

## Compliance scope

SOC 2 and ISO 27001. No GRC function or auditor is engaged yet. The control mapping in [0009](0009-compliance-evidence-and-segregation-of-duties.md) is a best-effort assumption and must be validated before the tool is relied on in an audit period. GRC sign-off is a hard gate for enabling option B of [0004](0004-what-a-low-risk-verdict-permits.md).

## Known data gaps

No incident data linked to PRs and no numeric targets yet. [0013](0013-rollout-and-calibration.md) uses proxy labels for the backtest and sets targets from shadow-phase data.
