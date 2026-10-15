---
status: accepted
date: 2026-10-13
decision-makers: Nathan Zender
---

# Responsible user for automated PRs: rely on forge rules in v1, close the gap in v2

## Context and Problem Statement

Separation of duties requires that every change has an accountable human, and that this person is not the change's only approver (SOC 2 CC8.1 practice, ISO 27001 A.5.3).

When a person opens a PR, or a coding agent works on their behalf under their own account, the forge knows who the author is and blocks that person from approving. That covers the common case.

Automated PRs opened under a bot account break this:

- **A bot acting for a person.** Examples: the Claude GitHub App started by an `@claude` comment, Codex cloud, Devin. The person who started the agent is effectively the author, but the forge only sees the bot, so that person can approve their own agent's work. GitHub Copilot is the exception: GitHub records who started it and does not count that person's approval.
- **A bot acting on its own schedule.** Examples: Renovate, Dependabot, scheduled agents. No person started the change, and no human is named as accountable for it.

What is the need, what does v1 do about it, and what must v2 provide?

## Decision Drivers

- Threat T9 (process bypass) from ADR 0002.
- Keep v1 small. Forge-native rules already cover PRs tied to a user.
- The gap must be visible, not silent, so nobody mistakes v1 for full coverage.
- Anything an agent writes into the PR (commit trailers, PR body, assignees it sets itself) is untrusted.

## The need (requirements for v2)

1. Every PR has exactly one responsible user, and that user is a human.
2. The responsible user is established from a verified source, something the agent cannot forge.
3. The responsible user cannot be the only approver, and their own approval does not count toward required approvals.
4. The responsible user and how they were established are recorded in the evidence (ADR 0009).
5. If no responsible user can be established, a compensating control applies.

## Considered Options (for establishing the responsible user in v2)

1. **Forge-native agent metadata.** For example the user who assigned Copilot. Verified, but only some platforms provide it.
2. **Actor of the triggering event.** When an agent runs from a workflow or App event started by a comment, label, or assignment, the actor of that event is the responsible user. Verified, but needs per-platform integration.
3. **Policy mapping from a bot account to its single operator.** Works for a machine user that one person owns. Fails for shared bots.
4. **Assignee set by a human.** A human with write access assigns a responsible user to the PR. The tool reads the PR timeline to verify who made the assignment, and ignores assignments made by bots. Until a human-made assignment exists, the check stays pending. Works on any platform and covers scheduled bots such as Renovate.
5. **Commit trailer** such as `Agent-Initiator: <login>`. Written by the agent, so it is untrusted. It may only exclude the named person from approving and never resolves the responsible user. Otherwise an agent could name someone else and let its real initiator approve.
6. **Fallback: two independent human approvals** when no verified responsible user exists.

## Decision Outcome

### v1 (accepted)

- Assume every PR is tied to a human user. Rely on GitHub and GitLab rules for approver independence: the author cannot approve, and the most recent pusher cannot approve (ADR 0009). The settings-drift job verifies those settings (ADR 0005).
- Detect bot-authored PRs and flag them in the evidence: `author_type: bot`, `responsible_user: unverified`.
- Add no extra approval requirement for bot-authored PRs.
- The auditor report (ADR 0009) lists every bot-authored PR merged in the period, so the gap is visible.

### v2 (direction, not yet decided)

Combine options 1–3 as automatic verified sources, with option 4 as the universal manual path, option 5 limited to exclusion only, and option 6 as the fallback. Decide in a new ADR that supersedes this one.

### Consequences

- Good, because v1 stays small and relies on controls auditors already understand.
- Good, because the gap is recorded on every affected PR and in every report, not hidden.
- Bad, because in v1 the person who started a bot-account agent can approve its PR.
- Bad, because v1 names no accountable human for scheduled-bot PRs (Renovate, Dependabot) other than their approver.

Sequencing: phases 0–2 of the rollout (ADR 0013) enforce nothing, so the gap only matters from phase 3. An organization that uses bot-account agents should not rely on the tool as audit evidence for separation of duties until v2 ships.

### Confirmation

- v1 tests: bot-authored PRs are detected and flagged `responsible_user: unverified`; the auditor report lists them.
- v2 acceptance: each requirement in "The need" maps to a test, including a bot that sets its own assignee or writes a false trailer.

## More Information

- GitHub's handling for Copilot, where the initiator's approval does not count toward required approvals: <https://docs.github.com/en/copilot/how-tos/use-copilot-agents/coding-agent/review-copilot-prs>
