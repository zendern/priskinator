# Architecture Decision Records

Decisions for pr-riskinator, recorded per [ADR 0001](0001-record-architecture-decisions.md). New records start from [template.md](template.md).

## Index

| ADR | Title | Status |
|---|---|---|
| [0001](0001-record-architecture-decisions.md) | Record architecture decisions as MADR files | proposed |
| [0002](0002-scope-and-threat-model.md) | Scope and threat model | proposed |
| [0003](0003-verdict-model.md) | Verdict model: deterministic floor, AI may only escalate, fail closed | proposed |
| [0004](0004-what-a-low-risk-verdict-permits.md) | What a low-risk verdict permits: human approval now, standard-change bot approval per class as the goal | accepted |
| [0005](0005-trust-boundary-and-enforcement.md) | Trust boundary and enforcement | proposed |
| [0006](0006-mandatory-human-review-triggers.md) | Mandatory human-review triggers: path plus content rules | proposed |
| [0007](0007-risk-and-complexity-scoring.md) | Risk and complexity scoring: transparent additive points | proposed |
| [0008](0008-ai-assessment-layer.md) | AI assessment layer: optional, provider-agnostic, escalate-only | proposed |
| [0009](0009-compliance-evidence-and-segregation-of-duties.md) | Compliance evidence, segregation of duties, waivers, break-glass | proposed |
| [0010](0010-policy-configuration-and-inheritance.md) | Policy configuration: layered, tighten-only, tested | proposed |
| [0011](0011-platform-and-distribution.md) | Platform and distribution: forge-agnostic core, GitHub first | proposed |
| [0012](0012-implementation-language.md) | Implementation language for the core | proposed, **open** |
| [0013](0013-rollout-and-calibration.md) | Rollout and calibration | proposed |

## Open decisions

- **0012**: implementation language. Recommended: Go if content rules shell out to ast-grep or Semgrep; Rust if they run in-process.
- **0005**: where the GitHub App's evaluation runs (hosted service or a job in a protected repo).
- **0008**: default AI provider and model, and data-handling option.
- **0009**: evidence retention period (with GRC), and initiator-metadata strategy for agent platforms.

## Items to verify against current GitHub behavior

- Whether a GitHub App's approval review counts toward ruleset required approvals and required-reviewer rules (0004). Blocks enabling option B for any change class.
- Whether check runs from different workflows under the GitHub Actions app can satisfy a required check by name (0005).
- Plan availability of the ruleset required-reviewer rule (0011).

## Compliance scope

SOC 2 and ISO 27001. The control mapping in [0009](0009-compliance-evidence-and-segregation-of-duties.md) is a proposal and must be validated with GRC before the tool is relied on in an audit period.
