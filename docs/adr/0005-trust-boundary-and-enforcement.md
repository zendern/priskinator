---
status: accepted
date: 2026-10-13
decision-makers: Nathan Zender
---

# Trust boundary and enforcement: nothing in the PR can change how the PR is judged

## Context and Problem Statement

A gate that the gated change can edit is not a gate. With the GitHub `pull_request` event, the workflow file that runs comes from the PR's merge commit, so a PR can rewrite the very workflow that evaluates it. A PR can also edit the policy file, change scripts that a naive tool would execute, or add a second workflow that posts a passing check with the same name. Where does trusted input end, and what identity enforces the verdict?

## Decision Drivers

- Threats T1 (gate tampering) and T9 (process bypass) from ADR 0002.
- Never execute untrusted code with access to secrets or write tokens.
- The verdict must be bound to an exact commit so later pushes cannot inherit it.
- Some GitHub features are plan-dependent, so the design must state which plan each option needs.

## Decisions

### 1. Head content is data, never code

The tool reads the head commit only as git objects and diffs. It never runs `npm install`, `make`, test suites, linters configured by the repo, or any script from the head. External analyzers (for example Semgrep or ast-grep) are pinned binaries shipped with the tool, run with rule sets from the tool or the base-branch policy, never from the head.

### 2. Policy comes from the base, never the head

Policy is loaded from the base branch at the PR's base SHA, or from a protected central policy repository (ADR 0010). A PR that changes policy, prompts, or the tool's workflow triggers mandatory review (ADR 0006, group G2). The new policy takes effect only after merge.

### 3. Verdicts are bound to the head SHA

Every check run and evidence record names the head SHA it evaluated. The tool re-evaluates on:

- new pushes (`synchronize`)
- base branch changes and PR edits (`edited`), including retargeting to another base
- `reopened`
- review submitted or dismissed
- label changes used for waivers or break-glass (ADR 0009)
- a policy change merged to the base branch, which re-evaluates all open PRs against that base

Approvals on an older head SHA do not count toward the tool's requirements, even if the forge setting to dismiss stale approvals is off.

### 4. Enforcement identity: options

| Option | PR can alter it? | Spoofable by a PR workflow? | Plan / ops cost |
|---|---|---|---|
| a. Repo workflow on `pull_request` | Yes | Yes | None. Rejected. |
| b. Repo workflow on `pull_request_target` | No (runs base version) | Yes, in practice: a PR can add a workflow on `pull_request` whose job has the same name, and it reports under the same GitHub Actions app. GitHub calls duplicate job names "ambiguous" and does not document which result wins. | None. Risky: runs with write token and secrets next to untrusted input. |
| c. Org ruleset "require workflows to pass" from a protected central repo | No | The rule requires the workflow itself, not a check name | GitHub Enterprise Cloud only |
| d. Dedicated GitHub App with its own identity; required status check pinned to that App as expected source | No | No: PR workflows cannot post as the App | Requires hosting the App (service or job) and managing its private key |

### 5. Repository settings are verified, not assumed

The tool depends on forge settings it cannot enforce by itself. A scheduled job checks each covered repository and records drift as evidence:

- PR required for the protected branch; no direct pushes
- required check from priskinator pinned to the expected App
- stale approvals dismissed; approval of the most recent push required
- force push and deletion blocked
- bypass list limited to the break-glass group
- merge queue workflows also trigger on `merge_group`

## Decision Outcome

Decisions 1, 2, 3 and 5: accepted as stated.

Decision 4 is **superseded by ADR 0017**, which runs the gate in GitHub Actions through a pinned reusable workflow, with tokens for the organization's own App issued by an OIDC token broker. The required check stays pinned to the App. The original decision is kept below for history.

Original decision 4: chosen option d, a dedicated GitHub App with the required check pinned to the App. GitHub is the first target, and option c needs GitHub Enterprise Cloud, which cannot be assumed. The App evaluates PRs in every rollout phase from shadow onward, so there is one enforcement path to build, not an Action that is later replaced. Options a and b are rejected for PR evaluation. Hosting and tenancy for the App are decided in ADR 0014.

Enforcement still relies on GitHub branch protection or rulesets requiring the App's check. On private repositories those features need a paid GitHub plan.

### Consequences

- Good, because no file in the PR can alter policy, analysis, or the posted result.
- Good, because a verdict cannot be carried over to a later push.
- Bad, because option d means operating a service and protecting an App private key. The private key is the root of trust for enforcement: anyone holding it can post a passing check. The service and the key are in audit scope.
- Bad, because the tool cannot use the repo's own linters or tests as signals. Those remain separate CI checks.

### Confirmation

- Integration test: a PR that edits the tool's workflow, policy, or prompts gets `human_review: required` and the verdict is computed with the base policy.
- Integration test: a PR that adds a workflow posting a fake check with the tool's name does not satisfy the required check.
- Integration test: pushing after approval resets the tool's approval accounting.
- The settings-drift job runs on every covered repo and alerts on any drift.

## More Information

- Where the App runs and who operates it: ADR 0014.
- Verified 2026-10-13 from GitHub docs: "Any person or integration with write permissions to a repository can set the state of any status check", and when a required check has an expected source app, "If the status is set by any other person or integration, merging won't be allowed." Pinning to a dedicated App blocks spoofing. Pinning to the GitHub Actions app does not distinguish between workflows. <https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/about-protected-branches>
- A ruleset's expected source app must be installed with the `statuses: write` permission (ADR 0014).
- GitHub ruleset required workflows: <https://github.blog/changelog/2023-10-11-requiring-workflows-with-repository-rules-is-generally-available/>
- Expected-source pinning for required checks: <https://docs.github.com/en/enterprise-server@3.16/pull-requests/collaborating-with-pull-requests/collaborating-on-repositories-with-code-quality-features/troubleshooting-required-status-checks>
- Copilot coding agent precedent: workflows on its PRs do not run until a human approves them: <https://github.blog/changelog/2026-03-13-optionally-skip-approval-for-copilot-coding-agent-actions-workflows/>
