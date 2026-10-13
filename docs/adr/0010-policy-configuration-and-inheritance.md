---
status: proposed
date: 2026-10-13
decision-makers: Nathan Zender
---

# Policy configuration: layered YAML, repos can only tighten, policies are tested

## Context and Problem Statement

Rules, weights, thresholds, reviewer routing, waivers, and AI settings must be configurable. Configuration is also an attack surface (threat T1): a policy a PR can relax is worthless. Organizations need one baseline across many repos, with room for repo-specific additions. How is policy expressed, where does it live, how do layers combine, and how do we know a policy change does what it claims?

## Decision Drivers

- Policy is read only from trusted locations (ADR 0005).
- An org baseline that individual repos cannot weaken.
- Strict validation: a typo must not silently disable a rule.
- Policy changes are themselves changes that need impact analysis and review.

## Considered Options

1. A single repo-level file, like most PR bots
2. Layered: built-in defaults, then org baseline, then repo policy, with tighten-only merge semantics
3. Policy in a general policy engine (OPA/Rego)

## Decision Outcome

Chosen option: "Layered, tighten-only", because it gives a central baseline and local flexibility without letting a repo opt out of the baseline. A general policy engine adds a language most contributors do not know, for little gain at this scope. It stays an option for complex org-specific rules later.

### Layers

1. **Built-in defaults**, shipped and versioned with the tool. Contains the G1–G10 baseline from ADR 0006 and conservative scoring thresholds.
2. **Org baseline**, in a protected central repository (for example `<org>/riskinator-policy`), read at a pinned ref.
3. **Repo policy**, at `.riskinator/policy.yaml`, read from the PR's base SHA.

### Merge semantics

- A repo can add rules, add paths to existing rules, raise risk floors, lower score thresholds, add required reviewers, and enable stricter options.
- A repo cannot remove or disable a baseline rule, remove paths, lower floors, raise thresholds, or disable the AI if the baseline requires it.
- A repo policy that tries to weaken a rule is rejected for that rule with a `policy-weakening-attempt` trigger, and the baseline value applies.
- Exceptions to the baseline go through waivers only (ADR 0009).

### Format and validation

- YAML, validated against a published JSON Schema. Unknown keys are errors.
- A top-level `version` key. The tool supports the current and previous schema versions.
- An invalid repo policy is evaluated as baseline plus defaults, with a `config-invalid` trigger (ADR 0003).
- Rule IDs are stable and namespaced (for example `G3.lint-config`, `repo.payments-paths`), because evidence and waivers reference them.

Illustrative repo policy:

```yaml
version: 1
extends: org            # implicit; shown for clarity
triggers:
  G7.security-paths:
    add_paths:
      - "services/billing/**"
    reviewers: [security, payments]
  repo.feature-flags:
    paths: ["config/flags/**"]
    reviewers: [platform]
scoring:
  floors:
    - paths: ["db/migrations/**"]
      risk: high
ai:
  enabled: true
  provider: anthropic
```

### Testing and impact analysis

- `riskinator test` runs policy fixtures in `.riskinator/tests/`. Each fixture describes a synthetic change (file list, diff hunks, PR metadata) and the expected verdict. The central policy repo's CI runs all fixtures.
- `riskinator explain --pr <n>` prints the effective merged policy and every rule that fired, with evidence.
- `riskinator simulate --policy <file> --since 90d` replays recent PRs against a proposed policy and reports which verdicts would change. Its output is attached to the policy PR as impact analysis.

### Consequences

- Good, because the baseline is enforced everywhere and is auditable in one place.
- Good, because policy changes come with tests and a measured impact.
- Bad, because tighten-only semantics need careful definitions per field, and those definitions need their own tests.
- Bad, because a central policy repo is another protected asset in audit scope.

### Confirmation

- Merge-semantics tests: for every field, a weakening attempt is rejected and a tightening is applied.
- Schema tests: unknown keys, wrong types, and an unsupported version all produce `config-invalid`.

## More Information

- ReviewGate and policy-bot both read config from the target branch: <https://github.com/leo-aa88/reviewgate>, <https://github.com/palantir/policy-bot>
