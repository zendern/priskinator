---
status: accepted
date: 2026-10-13
amended: 2026-10-15
decision-makers: Nathan Zender
---

# Policy configuration: layered YAML, the org baseline is a floor, policies are tested

## Context and Problem Statement

Rules, weights, thresholds, reviewer routing, waivers, and AI settings must be configurable. Configuration is also an attack surface (threat T1): a policy a PR can relax is worthless. Organizations need one baseline across many repos, with room for repo-specific additions. How is policy expressed, where does it live, how do layers combine, and how do we know a policy change does what it claims?

## Decision Drivers

- Policy is read only from trusted locations (ADR 0005).
- An org baseline that individual repos cannot weaken.
- Repos without an org baseline, or with one that leaves a field open, can tune the shipped defaults to their codebase.
- Strict validation: a typo must not silently disable a rule.
- Policy changes are themselves changes that need impact analysis and review.

## Considered Options

1. A single repo-level file, like most PR bots
2. Layered: built-in defaults, then org baseline, then repo policy, with tighten-only merge semantics
3. Policy in a general policy engine (OPA/Rego)

## Decision Outcome

Chosen option: "Layered, tighten-only", because it gives a central baseline and local flexibility without letting a repo opt out of the baseline. Amended 2026-10-15: tighten-only applies to what the org baseline sets. A repo may loosen what only the built-in defaults set (see Merge semantics). A general policy engine adds a language most contributors do not know, for little gain at this scope. It stays an option for complex org-specific rules later.

### Layers

1. **Built-in defaults**, shipped and versioned with the tool. Contains the G1–G10 baseline from ADR 0006 and conservative scoring thresholds.
2. **Org baseline**, in a protected central repository (for example `<org>/priskinator-policy`), read at a pinned ref.
3. **Repo policy**, at `.priskinator/policy.yaml`, read from the PR's base SHA.

### Merge semantics

- A repo can add rules, add paths to existing rules, raise risk floors, lower score thresholds, add required reviewers, and enable stricter options.
- **Fields the org baseline sets are a floor.** A repo cannot remove or disable them, remove their paths, lower their floors, raise their thresholds, or disable the AI if the baseline requires it. A repo policy that tries is rejected for that field with a `policy-weakening-attempt` trigger, and the baseline value applies. A rule's paths count as set when the org sets `paths` or `add_paths` for it.
- **Fields only the built-in defaults set can be loosened** by the repo: for example narrowing a default glob, raising a threshold, or disabling a default rule. The repo's value takes effect, and each loosened field is reported as an applied weakening. The defaults are a starting point written without knowledge of the repo; the org baseline is the organization's control.
- **A loosening never takes effect without human review.** Policy is read from the base branch only (ADR 0005), so a PR that changes `.priskinator/` is evaluated under the old policy, cannot apply its own change, and fires G2 (ADR 0006), which always requires human review. That PR's verdict names each field its policy loosens, so the reviewer sees what is being given up rather than only a YAML diff. After merge, every evaluation's evidence lists the weakenings in force (ADR 0009).
- Exceptions to the org baseline go through waivers only (ADR 0009).

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

- `priskinator test` runs policy fixtures in `.priskinator/tests/`. Each fixture describes a synthetic change (file list, diff hunks, PR metadata) and the expected verdict. The central policy repo's CI runs all fixtures.
- `priskinator explain --pr <n>` prints the effective merged policy and every rule that fired, with evidence.
- `priskinator simulate --policy <file> --since 90d` replays recent PRs against a proposed policy and reports which verdicts would change. Its output is attached to the policy PR as impact analysis.

### Consequences

- Good, because the baseline is enforced everywhere and is auditable in one place.
- Good, because policy changes come with tests and a measured impact.
- Bad, because tighten-only semantics need careful definitions per field, and those definitions need their own tests.
- Bad, because a repo without an org baseline can loosen any default. The protection is human review of the policy PR and the weakenings recorded in evidence, not the tool refusing.
- Bad, because a central policy repo is another protected asset in audit scope.

### Confirmation

- Merge-semantics tests: for every field, a tightening is applied; a weakening is rejected when the org baseline sets the field, and applied and reported when only the defaults do.
- Schema tests: unknown keys, wrong types, and an unsupported version all produce `config-invalid`.

## More Information

- ReviewGate and policy-bot both read config from the target branch: <https://github.com/leo-aa88/reviewgate>, <https://github.com/palantir/policy-bot>
