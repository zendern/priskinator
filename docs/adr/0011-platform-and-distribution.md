---
status: accepted
date: 2026-10-13
decision-makers: Nathan Zender
---

# Platform and distribution: forge-agnostic core, GitHub first

## Context and Problem Statement

The initial target is GitHub, but organizations also run GitLab. The tool must run locally for policy development, in CI, and behind an identity a PR cannot spoof (ADR 0005). As a security control, its own supply chain must also be trustworthy. How is the tool structured and shipped?

## Decision Drivers

- Enforcement identity requirements from ADR 0005.
- Keep the deterministic core testable without network access or a forge.
- Avoid locking the architecture to one forge's API.
- The tool must meet the bar it enforces: signed, reproducible, pinned.

## Considered Options

1. A GitHub Action only
2. A GitHub App only
3. A forge-agnostic core library and CLI, with thin forge adapters and multiple delivery wrappers

## Decision Outcome

Chosen option: "A forge-agnostic core with thin adapters", because the deterministic engine can then be tested in isolation, run locally, and reused across forges, while delivery can follow the enforcement choice in ADR 0005.

### Components

| Component | Responsibility |
|---|---|
| Core | Input: normalized change set (file changes, hunks, modes, renames, submodule changes) plus PR metadata plus merged policy. Output: verdict and evidence record. No network access. |
| Analyzers | Path matcher, content rules (tree-sitter, optionally ast-grep or Semgrep), complexity, secret scan. Pinned versions. |
| Forge adapters | GitHub first. Fetch PR data and approvals, post check runs, request reviewers, read labels and timeline. GitLab later: external status checks, approval rules, MR approval policies. |
| AI providers | Adapter per provider (ADR 0008). |
| Evidence sinks | S3-compatible write-once object store, Kosli (ADR 0009). Attestation signing is deferred to v2. |
| Delivery | CLI: local use, policy development, and backtesting (`explain`, `test`, `simulate`). Action: runs the gate on PRs. Reusable workflow wrapping the action: the recommended entry point and trust anchor for PR evaluation, from shadow through enforcing (ADR 0017). The same action also runs `priskinator test` and `simulate` in the central policy repo's CI. Container image: the action's runtime, pinned by digest. Per-org infrastructure: a self-hosted octo-sts broker with the org's own GitHub App (ADR 0017). |

### Supply chain of the tool itself

- Releases are built reproducibly in CI with SLSA build provenance, signed (cosign or GitHub artifact attestations), and published with an SBOM.
- Consumers pin by image digest or release checksum, never by a mutable tag.
- The tool's own repository is protected by the tool's own policy (dogfooding).
- Third-party actions in the tool's own workflows are pinned by full commit SHA. The March 2025 `tj-actions/changed-files` compromise (CVE-2025-30066) is the reference case.

### Consequences

- Good, because the core is pure and fast to test, and forge support is additive.
- Good, because local `explain`, `test`, and `simulate` commands use the same engine as enforcement.
- Bad, because there are several delivery wrappers to maintain.
- Bad, because GitLab parity is not free. Some enforcement primitives differ and need their own ADR when GitLab work starts.

### Confirmation

- The core package has no network dependencies, enforced in CI by a dependency check.
- Every release has a verified provenance attestation before publication.

## More Information

- Plan dependencies (checked 2026-10-13):
  - Rulesets cover public repositories on all plans, and private repositories on Pro, Team, and Enterprise Cloud.
  - Ruleset required workflows need Enterprise Cloud.
  - The ruleset required-reviewer rule is not available on user-owned repositories, because they have no teams. It needs an organization.
  - None of these block the App. They only limit the native defense-in-depth layer (ADR 0006) and the enforcing phase (ADR 0013).
  - Source: <https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/available-rules-for-rulesets>
