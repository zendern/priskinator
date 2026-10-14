---
status: accepted
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

## Control mapping (best-effort assumption; not validated)

No GRC function or auditor has reviewed this yet (as of 2026-10-13). The mapping and evidence design below are our best reading of SOC 2 and ISO 27001 practice. Every decision is built so a field can be added or retention extended later without re-architecting.

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

- PR author, every commit author, author type (`user` or `bot`), and responsible-user status (ADR 0015)
- The approvals that counted: approver, time, approved head SHA, and team membership at the time
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
| Signed in-toto attestation (Sigstore or GitHub artifact attestations) | Yes, verifiable offline | Depends on store | Deferred to v2 (see below). |
| Kosli or a similar evidence vault | Yes | Vendor-managed | Supported through a sink adapter. |

Chosen for v1: write each record to an S3-compatible object store in write-once mode (object lock in compliance mode). Provenance comes from access control and the store's audit log, not from a signature:

- The gate reaches the store through OIDC federation to the cloud provider. No stored cloud credentials exist.
- The store's role trusts only the gate's identity (repository owner, default branch, pinned workflow), and it may only put objects under that repository's prefix. It has no delete or overwrite permission.
- The store's access log records which identity wrote each object.
- MinIO, which supports object locking, is used for local development and end-to-end tests, so v1 is cloud-agnostic.

Signed attestations are deferred to v2. When added, they use GitHub artifact attestations (`actions/attest`) where available, meaning public repositories on any plan and private or internal repositories on Enterprise Cloud. Elsewhere they use cosign with a cloud key-service key. Signing with Sigstore's public instance is not used for private repositories, because the public transparency log would expose the repository's name and workflow identity.

Evidence sinks are a pluggable interface. Retention is configurable per evidence sink. The default is 13 months, covering a 12-month SOC 2 Type II window plus reporting lag. The default is a placeholder until GRC sets a value. If an evidence write fails, the check does not pass (ADR 0003).

### 3. Separation-of-duties rules

v1 assumes every PR is tied to a human user and relies on the forge's own rules for approver independence:

- GitHub and GitLab prevent the PR author from approving their own PR. When an agent works on a person's behalf under that person's account, that person is the author, so this covers the common case.
- The forge setting "require approval of the most recent reviewable push" prevents the last pusher from approving. The settings-drift job (ADR 0005) verifies it is on.
- The tool records the PR author, every commit author, and every approver in the evidence, so independence can be checked after the fact even where the tool does not enforce it.
- PRs authored by a bot account are flagged in the evidence (`author_type: bot`, `responsible_user: unverified`). v1 adds no extra approval requirement for them. This is a known, documented gap, see ADR 0015.

Rules the tool enforces in v1:

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
- A dead-man's-switch check: any covered repository without a settings-drift record in the last N days is flagged, so a disabled or dropped scheduled run becomes visible

### 8. Settings-drift checking

- One central scheduled workflow in the organization's policy repository checks every covered repository. It uses a broker token with `administration: read`, granted by a trust policy in that repository. Repository admins cannot disable it for their own repository.
- It runs at an off-peak minute (for example `:17`). GitHub delays scheduled runs under load and may drop some, especially at the top of the hour.
- GitHub disables scheduled workflows in public repositories after 60 days without activity. The dead-man's-switch check in the auditor report catches this, along with dropped or disabled runs.

## Decision Outcome

Accepted as stated above, as a best-effort design without GRC input. Before the tool is relied on in an audit period, GRC or the auditor must review the control mapping, the evidence fields, and retention. Until then, evidence records are versioned (`schema` field) so the format can change without losing earlier records.

### Consequences

- Good, because each sampled change can be answered with one verifiable record.
- Good, because exceptions exist, are bounded, and are visible.
- Good, because v1 relies on forge-native approver independence, which auditors already understand, and records enough to check it afterwards.
- Bad, because a WORM store and signing add infrastructure, and that infrastructure is in audit scope.
- Bad, because in v1 the person who started a bot-authored PR can approve it. The gap is flagged on every such PR and closed in v2 (ADR 0015).

### Confirmation

- Tests that approvals on a superseded head SHA, approvals from bot accounts, and approvals from users outside a required team do not count.
- Tests that bot-authored PRs are flagged `responsible_user: unverified` in the evidence.
- A test that a failed evidence write blocks the check.
- A quarterly dry-run audit: sample 25 merged PRs and answer the standard CC8.1 questions using only the evidence store.

## More Information

- Retention defaults to 13 months and is configurable. Revisit the default with GRC.
- Deferred to v2: a verified responsible user for automated PRs (ADR 0015).
- GitHub Actions retention limits: <https://docs.github.com/en/organizations/managing-organization-settings/configuring-the-retention-period-for-github-actions-artifacts-and-logs-in-your-organization>
- Kosli: <https://docs.kosli.com/understand_kosli/what_is_kosli>
- What PR evidence does and does not prove: <https://oneuptime.com/blog/post/2026-08-04-github-pull-requests-change-management-evidence/view>
