---
status: proposed
date: 2026-10-13
decision-makers: Nathan Zender
---

# What a low-risk verdict permits

## Context and Problem Statement

ADR 0003 defines a risk tier and a `human_review` flag. The tier is only useful if it changes what happens next. The most consequential choice in the project is whether a low-risk PR can merge without any human approval. If it can, the tool becomes the gatekeeper, and the goal of any adversarial author becomes "get scored low". If it cannot, the tool routes and documents human attention but never replaces it.

This decision is deliberately left open. This ADR records the options and tradeoffs so the decision can be made with GRC and, ideally, the external auditor.

## Decision Drivers

- SOC 2 CC8.1 requires changes to be authorized, tested, and approved through a documented process. It does not name who must approve. Auditor practice expects a named human approver independent of the author.
- ISO 27001 A.5.3 (segregation of duties) and A.8.32 (change management) point the same way.
- No public source found so far reports an auditor accepting an approval chain in which only an agent approved.
- Reviewer attention is the bottleneck. Rewind reports that about 40% of their PRs are now bot-approved, mostly mechanical dependency bumps.
- A wrong low verdict is far more expensive under option B than under option A.

## Considered Options

### A. Advisory: every PR needs at least one human approval

The tier controls how much review is required, not whether review happens.

| Verdict | Required approvals (example defaults) |
|---|---|
| low | 1 approver with write access, independent of author and initiator |
| medium | 1 code owner for the touched paths |
| high | 2 approvers, at least one code owner |
| any mandatory trigger | Approval from the team mapped to each trigger (ADR 0006), in addition to the above |

### B. Standard-change auto-approval

PRs that meet all eligibility conditions are approved by the tool's own identity, and that approval satisfies the branch's review requirement. Eligibility, all required:

- `risk: low` and no triggers
- `evaluation: ok` (not degraded), and the AI ran and agreed on low
- Within a strict size cap
- Within an allow-listed change class (for example dependency patch bumps from Renovate, docs-only changes)
- The tool's evidence record was written successfully

Preconditions before turning B on:

- A written standard-change procedure approved by GRC that defines the eligible classes.
- An auditor walkthrough of the procedure before the audit period in which it is used.
- Retrospective human sampling of a fixed percentage of bot-approved PRs, recorded as evidence.
- A kill switch that reverts every repo to option A immediately.

### C. Phased: A now, B per change class later

Start with A. Collect evidence in shadow and advisory modes (ADR 0013). Enable B for one narrow class at a time, only after the class shows a measured escape rate below an agreed threshold and GRC signs off.

## Decision Outcome

Not decided. Recommended direction: option C. It delivers the gaming detection, routing, and evidence value of the tool immediately, without making the tool's low verdict an attack target until there is data to justify it.

### Consequences of each option

- A: Good, because it is easy to defend in audit and the tool's errors cost reviewer time only. Bad, because it does not reduce the number of reviews, only how deep they are.
- B: Good, because it removes human review from genuinely mechanical changes. Bad, because the tool becomes the approver of record, and every false low becomes an unreviewed production change. It needs GRC and auditor buy-in first.
- C: Good, because it defers the risky part until there is evidence. Bad, because the time savings arrive later.

### Confirmation

Whichever option is chosen, golden-fixture tests assert the approval requirements for each verdict. If B is adopted, a test asserts that no PR with any trigger, a degraded evaluation, or a missing evidence record is ever bot-approved.

## More Information

- Open question for B: confirm that an approval review submitted by a GitHub App counts toward a ruleset's required approvals, and whether it can satisfy a required-reviewer rule for a team.
- Auditor red flag noted in practitioner write-ups: large AI-generated diffs approved within minutes with no comments are treated as evidence the review control is not operating. ADR 0009 covers detecting this.
- Practitioner discussion of SOC 2 and AI approvers: <https://ysecurity.io/blog/agentic-sdlc-code-review-soc-2/>, <https://heygrc.com/blog/soc-2-never-said-a-human-approves-your-prs>
