---
status: proposed
date: 2026-10-13
decision-makers: Nathan Zender
---

# Verdict model: deterministic floor, AI may only escalate, fail closed

## Context and Problem Statement

pr-riskinator combines deterministic rules with an optional AI assessment. The two sources can disagree, and the AI is neither reproducible nor robust to prompt injection. Auditors need the control to behave the same way every time it sees the same input. How do we combine the two signals into one verdict, and what happens when part of the evaluation fails?

## Decision Drivers

- Reproducibility: same diff, same policy, same tool version must give the same deterministic result (threat T10, audit sampling).
- Prompt-injection resistance: text in the PR must not be able to lower the verdict (T8).
- Fail-safe: an error must never produce a pass.
- Configurability: teams must be able to turn the AI on or off without changing the meaning of the deterministic result.

## Considered Options

1. Deterministic rules only
2. AI only
3. Weighted blend of deterministic score and AI score
4. Deterministic floor; final verdict is the maximum of the deterministic and AI results

## Decision Outcome

Chosen option: "Deterministic floor; final verdict is the maximum", because it keeps the auditable control fully deterministic while still letting the AI catch things the rules miss. An attacker who manipulates the AI can at worst cause extra human review.

### Verdict shape

```yaml
risk: low | medium | high            # ordinal
human_review: required | optional    # "optional" only exists if ADR 0004 option B is adopted
triggers: [rule IDs]                 # mandatory-review rules that fired (ADR 0006)
required_reviewers: [team slugs]     # routing derived from triggers and risk
reasons:                             # every factor that contributed, with evidence
  - rule: G3.lint-config
    file: .eslintrc.json
    detail: "file modified"
evaluation: ok | degraded | error
```

### Combination rules

1. The deterministic engine computes `risk_det` from scoring (ADR 0007) and applies risk floors from any triggers that fired (ADR 0006).
2. If the AI is enabled and returns a valid result, `risk_final = max(risk_det, risk_ai)`. Otherwise `risk_final = risk_det`.
3. Any mandatory trigger sets `human_review: required`, regardless of risk.
4. The AI can add triggers of its own (for example "PR description does not match the diff"), which also require human review. It can never remove a trigger or lower a tier.

### Failure handling

| Condition | Result |
|---|---|
| Deterministic engine error, invalid diff, missing base policy | `evaluation: error`, `risk: high`, `human_review: required` |
| Repo policy invalid | Evaluate with org baseline and built-in defaults, add a `config-invalid` trigger, `human_review: required` (ADR 0010) |
| Diff above size cap | `oversize` trigger, `human_review: required`, AI skipped |
| AI timeout, error, or invalid output | `evaluation: degraded`, deterministic verdict stands, recorded in evidence. If ADR 0004 option B is adopted, a degraded evaluation is never eligible for bot approval. |
| Forge API error while posting the result | The check stays pending or fails; it never reports success. |

### Pipeline

```mermaid
flowchart LR
  A[PR event] --> B[Load policy from base SHA]
  B --> C[Read diff and metadata as data]
  C --> D[Mandatory triggers]
  C --> E[Risk and complexity scoring]
  C --> F[AI assessment, optional]
  D --> G[Deterministic verdict]
  E --> G
  G --> H[Final verdict = max of deterministic and AI]
  F --> H
  H --> I[Check run, reviewer requests, evidence record]
```

### Consequences

- Good, because the deterministic result alone is the control an auditor tests, and it is reproducible.
- Good, because prompt injection that tries to lower risk has no effect.
- Good, because the AI can be disabled per org or repo without changing what the deterministic result means.
- Bad, because a noisy AI can only add work. AI escalation precision must be measured (ADR 0013) and the AI turned down if it floods reviewers.
- Bad, because "max" discards cases where the AI is right that something is safer than the rules think. We accept that cost.

### Confirmation

- Property test: for any AI output, `risk_final >= risk_det` and the trigger set is a superset of the deterministic trigger set.
- Golden-fixture tests: fixed diffs produce fixed deterministic verdicts across releases.
- Fault-injection tests for every row in the failure table.

## Pros and Cons of the Options

### Deterministic rules only

- Good, because it is fully reproducible and simple to audit.
- Bad, because it misses semantic risks that do not match a path or pattern, such as auth logic moved into an unwatched file.

### AI only

- Good, because it understands intent and semantics.
- Bad, because it is not reproducible, can be prompt-injected into a low rating, and is hard to defend as a control.

### Weighted blend

- Good, because it uses both signals.
- Bad, because the AI can pull the verdict down, which reopens T8, and the result is not reproducible.

### Maximum of deterministic and AI

- Good, because it keeps determinism as the floor and makes manipulation useless for downgrading.
- Bad, because AI false positives add reviewer load.

## More Information

- Rewind's Diff Vader separates AI judgment from a deterministic verdict engine, but lets AI findings set the risk label. This ADR deliberately inverts that so the AI can only escalate.
