---
status: accepted
date: 2026-11-07
decision-makers: Nathan Zender
---

# Reject policy weakenings per field, not per rule

Amends ADR 0010 (Merge semantics, third bullet).

## Context and Problem Statement

ADR 0010 says a repo policy that tries to weaken a rule "is rejected for that rule with a `policy-weakening-attempt` trigger, and the baseline value applies." Read literally, one weakening field throws away every change the repo made to that rule, including its tightenings. The implementation in `internal/policy/layer.go` (`Merge`, `mergeTrigger`, `mergeAI`) rejects only the field that would weaken and still applies the repo's other changes, in the same rule and elsewhere. Which behavior is the decision?

## Decision Drivers

- Tighten-only must hold: no merged field may be looser than the baseline (ADR 0010).
- A rejected weakening must not make the effective policy looser than the repo's own tightenings asked for.
- Evidence should name exactly what was refused (ADR 0009).
- The behavior should be predictable from the policy file alone.

## Considered Options

1. Per rule: one weakening field drops all of the repo's changes to that rule (ADR 0010 as written)
2. Per field: drop only the weakening field and apply everything else
3. Whole policy: any weakening invalidates the repo policy, as `config-invalid` does

## Decision Outcome

Chosen option: "Per field", because it is the only option where a rejected weakening never discards a tightening. Dropping a whole rule, or the whole policy, would throw away added paths, reviewers, and raised floors that the repo legitimately asked for, so a policy author's mistake in one field would loosen the effective policy compared with what the rest of their file says.

Replacement for ADR 0010, Merge semantics, third bullet:

- A repo policy field that would weaken the baseline is rejected and the baseline value for that field applies. The repo's other fields, in the same rule and elsewhere, are still merged. Each rejected field is reported as a weakening with its JSON pointer (for example `/triggers/G3.lint-config/risk`) and, for rule fields, its rule ID. The `policy-weakening-attempt` trigger fires once per evaluation when any field was rejected.

What counts as one field:

- **Scalars** (limits, thresholds `medium` and `high` separately, risk floors, `enabled`, case sensitivity, AI settings): the value is accepted or rejected as a whole.
- **Rule `paths` and `reviewers`**: if the repo's list leaves out baseline entries, the field is reported as a weakening, and the merged list is the union of the baseline and the repo's list. The repo's additions are kept and the baseline entries are not removed.
- **Scoring factor bands**: one factor's band list is one field. If it gives any value fewer points than the baseline, that factor's bands are rejected and the other factors are still merged.
- **`scoring.exclude_paths`**: entries the baseline does not exclude are rejected and reported together; entries the baseline already excludes are kept.

### Consequences

- Good, because a rejected weakening can never make the effective policy looser than the rest of the repo's file asks for.
- Good, because the evidence lists every refused field precisely, which helps both auditors and the policy author.
- Bad, because the effective rule can be a mix the author did not write as a whole, for example their added reviewers with the baseline risk. The `policy-weakening-attempt` trigger sends the change to a human, and `priskinator explain` shows the merged result and each rejection (ADR 0010), so the mix is never silent.

### Confirmation

Tests in `internal/policy` pin this behavior:

- `TestMergeMixed`: within one rule, a rejected risk downgrade leaves the added reviewers applied, and each weakening carries its rule ID.
- `TestMergeKeepsGoingAfterARuleOrFactor`: a rejected factor does not stop other rules and factors from merging.
- `TestMergeEveryField` and `TestEveryFieldHasBothCases`: every field has a tightening case that is applied and a weakening case that is rejected (ADR 0010, Confirmation).

## Pros and Cons of the Options

### Per rule

- Good, because the effective rule is always either the baseline or exactly what the repo wrote.
- Bad, because a weakening in one field discards the repo's tightenings in the others.
- Bad, because "rule" has no meaning for fields outside triggers (limits, scoring, AI), so the rule needs a second definition there anyway.

### Per field

- Good, because tightenings always survive.
- Good, because it applies the same way to trigger and non-trigger fields.
- Bad, because the effective rule can be a mix (see Consequences).

### Whole policy

- Good, because it is simple and loud.
- Bad, because one mistake drops every repo-specific rule, which loosens the effective policy the most of the three options.

## More Information

- ADR 0010: layered, tighten-only policy configuration.
- ADR 0018: how an accepted ADR is amended.
