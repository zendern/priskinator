---
status: proposed
date: 2026-10-13
decision-makers: Nathan Zender
---

# AI assessment layer: optional, provider-agnostic, escalate-only

## Context and Problem Statement

Deterministic rules miss semantic risk: auth logic moved into an unwatched file, a refactor that quietly changes behavior, or a PR whose description says "fix typo" while the diff touches permission checks. An LLM can catch some of these. It is also non-deterministic, can be prompt-injected, costs money per PR, and sends source code to a third party. How is the AI layer designed so it adds signal without weakening the control or the compliance posture?

## Decision Drivers

- ADR 0003: the AI may only raise risk or add triggers, never lower either.
- Threat T8 (prompt injection) from ADR 0002.
- SOC 2 CC9.2 and ISO 27001 A.5.19–A.5.23: an LLM provider that receives source code is a supplier or subprocessor and needs vendor-risk treatment.
- Reproducibility for audit: record exactly what the model saw and said.
- The tool must work fully with the AI disabled.

## Considered Options

1. No AI layer
2. Single-pass structured assessment through a provider-agnostic adapter
3. Multi-agent group of specialist reviewers (Diff Vader style)
4. Delegate to a commercial AI reviewer (CodeRabbit, Qodo, or similar) and parse its output

## Decision Outcome

Chosen option: "Single-pass structured assessment through a provider-agnostic adapter", because it is the smallest design that meets the escalate-only contract and is fully recordable. Specialist passes (option 3) can be added later behind the same interface.

### Responsibilities

The AI is asked to look for what rules cannot see:

- Security-relevant behavior changes outside configured paths
- Mismatch between the PR's stated intent (title, body, linked issue) and the diff
- Changes to tests or expectations that make tests pass without fixing the cause
- Risky patterns in languages without content rules yet

### Contract

Input, assembled by the tool:

- The redacted diff and file list
- The deterministic verdict and its reasons, so the model does not repeat them
- Repo context from base-branch policy (what is security-sensitive in this repo)
- PR title, body, and linked issue text, wrapped and labeled as untrusted content

Output, validated against a JSON schema:

```json
{
  "risk": "low | medium | high",
  "escalate": true,
  "triggers": [{"id": "ai.intent-mismatch", "file": "src/x.py", "line": 42, "rationale": "..."}],
  "findings": [{"category": "...", "severity": "...", "confidence": 0.0, "file": "...", "line": 0, "rationale": "..."}],
  "summary": "..."
}
```

Any output that fails schema validation is an AI error (`evaluation: degraded`, ADR 0003).

### Safety rules

- The model has no tools, no repository write access, and no secrets in its environment other than its own API credential.
- Before sending, a secret scanner (for example gitleaks) runs on the diff. Matches are redacted and also fire a mandatory trigger.
- Model output posted to the PR is sanitized: no @-mentions, no raw HTML, and links are not rendered as clickable.
- Prompt templates and model configuration are policy files. Changing them triggers G2 (ADR 0006) and requires human review.

### Reproducibility and evidence

- Recorded per run: provider, model ID, prompt template hash, input hash, temperature and other parameters, raw output hash, and latency. The raw output is stored with the evidence (ADR 0009).
- Results are cached by input hash (diff, model ID, prompt hash, policy SHA). A re-run with identical input returns the cached result instead of calling the model again.

### Data handling options (configurable per org)

| Option | Notes |
|---|---|
| Commercial API with a zero-data-retention agreement | Simplest. Needs vendor review and a DPA. |
| Model hosted inside an existing cloud boundary (for example Amazon Bedrock or Google Vertex AI) | Reuses an already-assessed cloud vendor. |
| Self-hosted open-weight model | No code leaves the boundary. Lower quality, and we operate it. |
| Disabled | Deterministic verdict only. |

### Limits

- Token cap per PR. Above the cap, the AI is skipped and the skip is recorded.
- Per-PR cost cap and timeout.
- On any failure, the deterministic verdict stands (ADR 0003).

### Consequences

- Good, because it adds semantic coverage without giving the AI any power to approve or downgrade.
- Good, because every AI decision is recorded and can be replayed from cache.
- Bad, because AI false positives add reviewer load. Escalation precision is a tracked metric (ADR 0013).
- Bad, because enabling it brings a vendor into audit scope.

### Confirmation

- A labeled evaluation set of PRs, including injected-prompt cases. Prompt or model changes must not reduce recall on it.
- Tests that invalid, truncated, or hostile model output yields `degraded`, never a lower verdict.
- A test that a prompt-injection string in the PR body cannot change `risk_final` below `risk_det`.

## Pros and Cons of the Options

### No AI

- Good, because it is simplest and adds no vendor.
- Bad, because semantic risks go undetected.

### Single-pass structured assessment

- Good, because it is small, recordable, and meets the escalate-only contract.
- Bad, because one generalist pass misses things specialist passes catch.

### Specialist group

- Good, because it gives deeper coverage per domain (migrations, IaC, Actions, supply chain).
- Bad, because cost and latency multiply. Better as a later extension for high-risk PRs only.

### Commercial AI reviewer

- Good, because it requires nothing to build.
- Bad, because output formats are not designed as machine-readable verdicts, its configuration may live in the repo where the PR can edit it, and it adds another vendor.

## More Information

- Open: default provider and model, decided at implementation time and pinned in config.
- Candidate integration for GitHub-hosted runs: `anthropics/claude-code-action`, or a direct API call with structured output.
