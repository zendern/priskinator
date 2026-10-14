# Spike S1: does a GitHub App's approval count toward required approvals?

- Date: 2026-10-13
- Test org and repo: `toro-fast-CHI/github-ruleset-testing` (public, GitHub Free organization)
- Test App: `toro-fast-chi-s1-approver-write` (App ID 5308751, installation 171180609), permissions `pull_requests: write`, `contents: write`, `metadata: read`
- Referenced by: ADR 0004

## Answer

**Yes.** An approving review submitted by a GitHub App satisfies the required-approvals rule, both in a repository ruleset and in classic branch protection with admin enforcement on. A new push dismisses the App's approval like any other approval.

This behavior is not documented by GitHub. See "Risk" below.

## Setup

- Ruleset `s1-main-protection` on the default branch, enforcement `active`, **empty bypass list**:
  - `pull_request`: 1 required approval, dismiss stale reviews on push, require approval of the most recent reviewable push
  - `deletion`, `non_fast_forward`
- All PRs were opened and merged by `zendern`, a repository admin and the PR author. With an empty bypass list, the admin role grants no bypass, which T0 confirms.
- The App was created through the manifest flow. Its private key was held only in the browser tab's session storage, and was discarded when the tab closed.

## Results

| Test | PR | Setup | Before approval | After App approval | Merge as admin author |
|---|---|---|---|---|---|
| T0 control | [#1](https://github.com/toro-fast-CHI/github-ruleset-testing/pull/1) | Ruleset, no approvals | `REVIEW_REQUIRED`, `BLOCKED` | n/a | Rejected, 405 "New changes require approval from someone other than the last pusher" |
| Direct push | n/a | Ruleset | n/a | n/a | Rejected, 409 "Changes must be made through a pull request" |
| T1 App approval | [#2](https://github.com/toro-fast-CHI/github-ruleset-testing/pull/2) | Ruleset | n/a | `APPROVED`, `CLEAN` | **Merged** |
| T2 stale approval | [#3](https://github.com/toro-fast-CHI/github-ruleset-testing/pull/3) | Ruleset, author pushes after App approval | `APPROVED`, `CLEAN` | Review `DISMISSED`, `REVIEW_REQUIRED` | Rejected, 405 |
| T3 classic protection | [#4](https://github.com/toro-fast-CHI/github-ruleset-testing/pull/4) | Ruleset disabled, classic protection with `enforce_admins: true`, same review settings | `REVIEW_REQUIRED`, `BLOCKED`; merge rejected 405 | `APPROVED`, `CLEAN` | **Merged** |
| T4 read-scoped token | [#5](https://github.com/toro-fast-CHI/github-ruleset-testing/pull/5) | Ruleset; approval made with an installation token scoped down to `contents: read`, `pull_requests: write` | n/a | `APPROVED`, `CLEAN` | **Merged** |

States are from the GraphQL fields `reviewDecision` and `mergeStateStatus`.

## Not tested

- **Installation-level `contents: read`.** Changing the App's permissions in the settings UI did not save during the spike. T4 scoped only the token, while the installation kept `contents: write`. GitHub may evaluate the installation's access rather than the token's, so T4 does not prove that a read-only installation's approval counts. Re-test before shipping an App manifest without `contents: write`.
- **`github-actions[bot]` approvals.** Not relevant to the design, which never uses Actions approvals.
- **Ruleset required-reviewer rule naming a team.** GitHub docs state that only team members count, and the test org has no teams.

## Risk

The result rests on undocumented behavior that GitHub can change without notice. ADR 0004 option B must therefore:

- run a scheduled canary that repeats T0, T1 and T2 against a test repository and alerts on any change
- fall back to option A automatically when the canary fails
