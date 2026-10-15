---
status: accepted
date: 2026-10-13
decision-makers: Nathan Zender
---

# What a low-risk verdict permits

## Context and Problem Statement

ADR 0003 defines a risk tier and a `human_review` flag. The tier is only useful if it changes what happens next. The most consequential choice in the project is whether a low-risk PR can merge without any human approval. If it can, the tool becomes the gatekeeper, and the goal of any adversarial author becomes "get scored low". If it cannot, the tool routes and documents human attention but never replaces it.

This ADR records the options, the tradeoffs, and the chosen phased path.

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
| low | 1 approver with write access, other than the author (ADR 0009; responsible user for automated PRs in ADR 0015) |
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

Chosen option: "C. Phased", with option A in force now and option B as the target end state. Option A delivers the gaming detection, routing, and evidence value of the tool immediately, and no low verdict replaces a human until there is data to justify it.

B is enabled one change class at a time, never as a blanket setting. Each class must meet the preconditions listed under option B and the exit criteria in ADR 0013, phase 4.

### How the approval requirements are enforced

Native forge settings cannot vary the number of approvals with a PR's risk. The App's required check therefore carries the approval requirements, the same model policy-bot uses:

- The check stays pending until the approvals the verdict requires are present on the current head SHA, from eligible approvers (ADR 0009). It then reports success.
- The native "required approving reviews" setting stays at 1 or more while option A is in force, as a forge-enforced floor beneath the tool's tiered requirements.

### Design implications while option A is in force

Because B is the goal, the design must assume from the start that adversaries will push PRs toward a low verdict:

- **Shadow eligibility.** Every evaluation computes `bot_eligible` and the matching change class using the full option B rules, and records both in the evidence (ADR 0009). Nothing acts on it under option A.
- **Measure before enabling.** For each class, compare shadow-eligible PRs with what human reviewers actually did: requested changes, review comments, reverts, and incidents linked to the PR. That comparison is the escape-rate evidence a class needs before B is enabled for it.
- **Eligibility logic is tested now.** The option B eligibility rules and their tests ship with option A, so enabling a class is a configuration change backed by already-tested code, not new code.
- **GRC is a hard gate for B.** No GRC function is engaged yet (as of 2026-10-13), so the compliance design in ADR 0009 is a best-effort assumption. Option A does not depend on GRC sign-off. No change class can be enabled under B until GRC has approved the standard-change procedure.
- **Mechanism for B.** GitHub does not document whether a GitHub App's approving review counts toward required approvals. Spike S1 (2026-10-13, `docs/spikes/s1-app-approval-counts.md`) showed that it does, in rulesets and in classic branch protection with admin enforcement. A new push dismisses the App's approval like any other. So under B the App submits an approving review on eligible PRs, and the native floor stays at 1.
- **Canary for B.** Because that behavior is undocumented, B also requires a scheduled canary that repeats the S1 tests against a test repository. If the canary fails, every class falls back to option A automatically. If GitHub ever stops counting App approvals, the only alternative is a native floor of 0 with the App's check as the sole approval gate, which would need its own ADR and GRC review.
- **Open:** whether an App installation with only `contents: read` still counts. S1 tested only a read-scoped token. Re-test before the App manifest drops `contents: write`.

### Consequences of each option

- A: Good, because it is easy to defend in audit and the tool's errors cost reviewer time only. Bad, because it does not reduce the number of reviews, only how deep they are.
- B: Good, because it removes human review from genuinely mechanical changes. Bad, because the tool becomes the approver of record, and every false low becomes an unreviewed production change. It needs GRC and auditor buy-in first.
- C: Good, because it defers the risky part until there is evidence. Bad, because the time savings arrive later.

### Confirmation

- Golden-fixture tests assert the option A approval requirements for each verdict.
- Tests assert that `bot_eligible` is false for any PR with a trigger, a degraded evaluation, a missing evidence record, or no matching change class.
- Under option A, a test asserts that the tool never submits an approving review, whatever `bot_eligible` says.
- Enabling B for a class is recorded as a PR against the central policy, with the class's shadow-eligibility metrics and GRC sign-off attached.

## More Information

- GitHub docs describe required approvals only as coming from "people with write permissions" and team members: <https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/about-protected-branches>
- Community report on bot approvals not counting: <https://github.com/orgs/community/discussions/181487>
- An App approval cannot satisfy a ruleset required-reviewer rule, which counts only members of the named teams.
- Auditor red flag noted in practitioner write-ups: large AI-generated diffs approved within minutes with no comments are treated as evidence the review control is not operating. ADR 0009 covers detecting this.
- Practitioner discussion of SOC 2 and AI approvers: <https://ysecurity.io/blog/agentic-sdlc-code-review-soc-2/>, <https://heygrc.com/blog/soc-2-never-said-a-human-approves-your-prs>
