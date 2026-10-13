---
status: proposed
date: 2026-10-13
decision-makers: Nathan Zender
---

# Compliance evidence, segregation of duties, waivers, and break-glass

## Context and Problem Statement

SOC 2 and ISO 27001 auditors test change management by sampling changes from the audit period. For each sample they want proof that the change was authorized, risk-assessed, reviewed by someone independent of the author, and tested before it reached production. A SOC 2 Type II audit covers a period, commonly 6 to 12 months. GitHub Actions logs and artifacts are deleted after 90 days by default (configurable up to 400 days on private repositories), and PR comments and labels can be edited. Coding agents add a new separation-of-duties gap: when an agent opens a PR under a bot account, the human who prompted it can approve it. What evidence does pr-riskinator produce, where is it kept, and which separation-of-duties rules does it enforce?

## Decision Drivers

- Threats T9 (process bypass) and T10 (evidence loss or tampering) from ADR 0002.
- The tool itself is an in-scope control. Its own configuration, releases, and decisions need the same rigor as the changes it gates.
- Evidence must outlive the audit window and be tamper-evident.
- Exceptions (waivers, emergencies) must exist, or people will route around the tool. Every exception must be visible.

## Control mapping (proposed; validate with GRC and the auditor)

| Framework reference | What pr-riskinator contributes |
|---|---|
| SOC 2 CC8.1 change management | Risk assessment per change, required approvals by tier, evidence record per change |
| SOC 2 CC7.1 detection of configuration changes | Mandatory triggers for CI, lint, hook, and test configuration (ADR 0006) |
| SOC 2 CC4.1 monitoring of controls | Settings-drift job (ADR 0005), rubber-stamp detection, periodic reports |
| SOC 2 CC9.2 vendor management | AI provider treatment (ADR 0008) |
| ISO 27001 A.8.32 change management | Same as CC8.1 |
| ISO 27001 A.8.25 / A.8.28 secure development and coding | Security-sensitive triggers, suppression detection |
| ISO 27001 A.5.3 segregation of duties | Approver independence rules below |
| ISO 27001 A.8.4 access to source code | Verification of protected-branch settings |
| ISO 27001 A.8.15 logging / A.5.33 protection of records | Immutable evidence store with retention |
| ISO 27001 A.5.19–A.5.23 supplier and cloud services | AI provider treatment (ADR 0008) |

## Decisions

### 1. One evidence record per evaluation, plus one per merge

Evaluation record (written on every evaluation):

```json
{
  "schema": "riskinator.evidence.evaluation/v1",
  "repo": "org/repo",
  "pr": 123,
  "base_sha": "…",
  "head_sha": "…",
  "policy": {"source": "org/riskinator-policy@<sha>", "sha256": "…"},
  "tool": {"version": "x.y.z", "image_digest": "sha256:…"},
  "inputs": {"diff_sha256": "…"},
  "deterministic": {"risk": "medium", "points": 37, "factors": [], "triggers": []},
  "ai": {"enabled": true, "status": "ok", "model": "…", "prompt_sha256": "…", "output_sha256": "…", "risk": "high"},
  "verdict": {"risk": "high", "human_review": "required", "required_reviewers": ["security"], "bot_eligible": false, "change_class": null},
  "waivers_applied": [],
  "evaluated_at": "2026-10-13T00:00:00Z"
}
```

Merge record (written when the PR merges):

- The approvals that counted: approver, time, approved head SHA, team membership at the time, and the independence check result
- Merged SHA and merge method
- The final evaluation record ID
- Break-glass or waiver use

The merged SHA gives deployment tooling a key to link the production artifact back to its review evidence.

### 2. Evidence storage

| Option | Tamper-evident | Retention control | Notes |
|---|---|---|---|
| PR comments and check runs only | No | Tied to the forge | Editable. Not sufficient alone. |
| Actions artifacts | No | Up to 400 days, deletable | Not sufficient alone. |
| Object store in write-once (WORM) mode, for example S3 Object Lock in compliance mode | Yes | Set by GRC | Recommended primary store. |
| Signed in-toto attestation (Sigstore or GitHub artifact attestations) | Yes, verifiable | Depends on store | Recommended in addition, for verifiability. |
| Kosli or a similar evidence vault | Yes | Vendor-managed | Supported through a sink adapter. |

Proposed: write each record to a WORM object store and sign it as an in-toto attestation. Evidence sinks are a pluggable interface. Retention is configurable and set with GRC so it covers the full audit period plus the reporting lag. If an evidence write fails, the check does not pass (ADR 0003).

### 3. Separation-of-duties rules

- An approval counts only if the approver is not the PR author, not the author of any commit in the PR, and not the initiator of an agent-authored PR.
- Initiator resolution, in order: forge-native agent metadata (for example Copilot assignment), then a commit trailer `Agent-Initiator: <login>` on agent commits, then a policy mapping of bot accounts to owning teams.
- An agent-authored PR whose initiator cannot be resolved requires two independent human approvals.
- Approvals count only for the current head SHA (ADR 0005).
- The approver's membership in a required team is checked and recorded at approval time.
- Bot accounts never count as approvers. The only planned exception is the tool's own App identity, for change classes enabled under ADR 0004 option B.

### 4. Rubber-stamp detection

Signals recorded on the merge record, alerting only at first:

- Time from the last push to approval, compared with diff size
- High-risk or triggered PRs approved with zero review comments
- An approver approving many PRs within a short window

These feed a periodic report. Blocking on them is a later decision, once false-positive rates are known.

### 5. Break-glass

- A designated label (for example `riskinator/break-glass`) applied by a member of the break-glass group, verified from the PR timeline, not just from the label's presence.
- Effect: mandatory-trigger approvals reduce to one independent human approval. The tool never allows zero approvals.
- The tool automatically opens a follow-up issue requiring a retrospective review within a configured number of business days, and reports open retrospectives.
- Every use is flagged in the merge record and in the periodic report.

### 6. Waivers

- Waivers live only in base-branch or central policy, never in the PR, and never as code comments.
- Each waiver has an ID, the rule ID, a path scope, a justification, an owner, and an expiry date no more than 90 days out.
- Expired waivers are ignored, and expiring waivers are reported ahead of time.
- Adding or changing a waiver is a G2 change and requires human review.

### 7. Reporting for auditors

A report command produces, for a date range:

- Every merge to every protected branch, with verdict, approvals, waivers, and break-glass use
- A completeness check comparing merges in the forge's audit log or push events against evaluation records, so direct pushes or bypasses show up as gaps instead of disappearing

## Decision Outcome

Proposed as stated above. The control mapping must be reviewed by GRC before the tool is relied on in an audit period.

### Consequences

- Good, because each sampled change can be answered with one verifiable record.
- Good, because the agent-initiator approval gap is closed by the tool, not left to the forge.
- Good, because exceptions exist, are bounded, and are visible.
- Bad, because a WORM store and signing add infrastructure, and that infrastructure is in audit scope.
- Bad, because initiator resolution depends on agent platforms exposing metadata or adding trailers. The fallback (two approvals) costs reviewer time.

### Confirmation

- Tests that every SoD rule rejects the forbidden approver combinations.
- A test that a failed evidence write blocks the check.
- A quarterly dry-run audit: sample 25 merged PRs and answer the standard CC8.1 questions using only the evidence store.

## More Information

- Open: retention period, set with GRC.
- Open: which agent platforms expose initiator metadata, and whether to require the `Agent-Initiator` trailer through a commit-msg hook.
- GitHub Actions retention limits: <https://docs.github.com/en/organizations/managing-organization-settings/configuring-the-retention-period-for-github-actions-artifacts-and-logs-in-your-organization>
- Kosli: <https://docs.kosli.com/understand_kosli/what_is_kosli>
- What PR evidence does and does not prove: <https://oneuptime.com/blog/post/2026-08-04-github-pull-requests-change-management-evidence/view>
