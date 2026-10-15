---
status: accepted
date: 2026-10-13
decision-makers: Nathan Zender
---

# Run the gate in GitHub Actions with a pinned reusable workflow and an OIDC token broker

Supersedes ADR 0014 and decision 4 of ADR 0005.

## Context and Problem Statement

ADR 0005 chose a dedicated GitHub App as the enforcement identity, and ADR 0014 chose to run that App as a self-hosted, long-running server per organization. Two problems surfaced afterwards:

- **Hosting burden.** Every adopting organization must run a webhook service with public ingress, a queue, workers, and a scheduler. That slows adoption, which is now a goal (ADR 0014, ADR 0016).
- **The root-of-trust key sits next to the parser.** The server parses attacker-controlled PR content in the same process that holds the org-wide App private key. A parser exploit leaks a key that can post passing checks on every repository.

GitHub Actions was originally rejected because a PR can rewrite a `pull_request` workflow and because checks posted with the job token can be faked. Can the gate run in Actions without reopening those holes, while working for public, private, and internal repositories, fork PRs, and other organizations?

## Decision Drivers

- Threats T1 (gate tampering) and T9 (process bypass) from ADR 0002.
- One mode that works everywhere on GitHub.com: any plan, any visibility, fork PRs, other organizations.
- No long-lived secret in any repository.
- The smallest possible blast radius if the gate job is compromised.
- Low hosting burden for adopters.

## Considered Options

### Where the gate runs

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| A1. Repo workflow on `pull_request`, check posted with the job token | Nothing to host | The PR runs its own copy of the workflow and can rewrite or skip the gate. The check posts as "GitHub Actions", so it can be faked. Fork PRs get read-only access. | Rejected |
| A2. Repo workflow on `pull_request_target`, check posted with the job token | Uses the default branch's copy of the workflow | The check is still "GitHub Actions" and can be faked by PR code run in other CI jobs. Pwn-request risk. | Rejected |
| A3. Org ruleset "required workflows" | The PR can't modify it. Built into GitHub. | Enterprise Cloud only | Good for Enterprise Cloud orgs. Not the baseline. |
| A4. Self-hosted App server (ADR 0014) | No Actions limits. Low latency. No files needed in each repo. | Hosting with public ingress. The org-wide key sits next to the parser. An outage blocks merges. | Fallback |
| A5. Serverless App | No idle cost | Cloud-specific glue and time limits. Same key-next-to-parser problem as A4. | Variant of A4 |
| A6. Webhook relay into Actions jobs, with the App key in Actions secrets | Little infrastructure | Anyone who can edit the relay repo's workflows can steal the key | Rejected |
| A7. Hosted multi-tenant SaaS | One-click adoption | We hold every customer's key and code. We land in every customer's audit scope. | Not now |
| **A8. Actions + pinned reusable workflow + octo-sts, on `pull_request_target`** | No stored secrets. A small stateless broker. The parser never runs next to the key. Forks work. Signed evidence via `actions/attest`. | Actions minutes and runner start-up delay. Each repo needs a caller workflow and a trust policy. Review events need a relay. Requires discipline around `pull_request_target`. | **Chosen** |
| A9. Same as A8, calling the action directly | The caller is a single step | The trust anchor becomes each repo's caller file. A rubber-stamped edit adding a third-party step can steal tokens. | Supported, weaker |
| A10. `pull_request` triggering `workflow_run` | No `pull_request_target` at all | Two runs. Fork PRs must be looked up by head SHA. | Alternative trigger |
| A11. Actions for same-repo PRs, App server for forks | Each path uses its environment's native features | Two systems to build, run, and audit | Rejected |

### How credentials reach the gate

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| B1. Job token only | Nothing to manage | The check can't be pinned to a unique source, so it can be faked | Rejected |
| B2. App key in a repo or org secret | Trivial | Org secrets reach same-repo `pull_request` runs. Org-wide exposure. | Rejected |
| B3. App key in an environment secret, default branch only | GitHub checks the run's ref before releasing the secret | A key copy sits in every repo, and every repo admin can extract it | Extra layer only |
| **B4. OIDC to a self-hosted octo-sts** | No stored secret in repos. Scoped, short-lived tokens per repo. Policy based on claims. | We run and secure the broker. A trust-policy file per repo. | **Chosen** |
| B5. OIDC to Chainguard's public octo-sts | Nothing to host | A third party becomes the root of trust, under a shared App identity | Supported as the quick-start provider (see Token providers) |
| B6. OIDC to Vault's GitHub secrets engine | Same model as B4. Reuses Vault. | Needs Vault | Alternative for Vault shops |
| B7. OIDC to a cloud key service that signs the App JWT | The key can't be exported | The JWT carries App-wide power for about 10 minutes | Only behind a broker |
| B8. OIDC to a cloud secret manager that returns the key | Simple | The key lands in runner memory | Rejected |
| B9. Self-hosted runner group restricted to one workflow and ref | GitHub enforces the workflow and ref when scheduling | Runners to operate. Needs Team or Enterprise. | Option for orgs with their own runners |
| B10. Custom deployment protection rule | An external veto per run | Another App to host | Optional layer |

### Packaging

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| C1. Reusable workflow only | `job_workflow_ref` proves which code runs. Callers can't add steps. | Job-level call | Part of C3 |
| C2. Action only | One step | No OIDC claim names the action | Part of C3 |
| **C3. Action plus a thin reusable workflow wrapping it** | Serves both kinds of adopter | One extra short YAML file | **Chosen** |

### Fork PRs

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| **D1. `pull_request_target` for every PR** | One path. The trust policy can require `ref` = default branch, so no `pull_request` run ever gets a token. | Requires discipline. Review events need a relay. | **Chosen** |
| D2. `pull_request` for same-repo PRs, `pull_request_target` for forks | Same-repo PRs avoid `pull_request_target` | Two paths. More trust-policy rules. | Rejected |
| D3. Only require approval before fork workflows run | None | Fork runs still get no OIDC token, so the check never posts | Rejected |
| D4. Maintainer comment `/riskinator evaluate` | Manual override in the default branch's context | Manual | Added as an override |

## Decision Outcome

Chosen: **A8 + B4 + C3 + D1.** The gate runs in GitHub Actions on `pull_request_target` for every PR. It calls our action through a reusable workflow pinned by SHA. It gets short-lived, repository-scoped tokens from a configurable OIDC token broker (see Token providers) and posts the required check as the broker's App.

### Components

- **Action** (`zendern/priskinator@<sha>`): runs the Go core against the PR, read as data. It is the product.
- **Reusable workflow** (`zendern/priskinator/.github/workflows/gate.yml@<sha>`): one job, one step that uses the action. This is the recommended entry point and the trust anchor.
- **Caller workflow** in each repository: a few lines that invoke the reusable workflow on the triggers below.
- **Token provider**: an octo-sts broker, either Chainguard's public instance or one self-hosted per organization with the organization's own GitHub App (the tenancy model of ADR 0014 is kept). The App's private key lives only in the broker's key store.
- **Trust policy** in each repository at `.github/chainguard/riskinator.sts.yaml`.

### Triggers

| Need | Trigger | Ref the run uses |
|---|---|---|
| PR opened, pushed, retargeted, labeled | `pull_request_target` | Default branch (always, since GitHub's 2025-12-08 change) |
| Review submitted or dismissed | `pull_request_review` (no privileges) triggers `workflow_run`. The privileged run looks the PR up by head SHA, because `workflow_run.pull_requests` is empty for fork PRs. | Default branch |
| Re-run or manual evaluation | Actions "Re-run jobs", or `/riskinator rerun` and `/riskinator evaluate` through `issue_comment`, after checking that the commenter has write access | Default branch |
| Policy merged | `push` to the default branch, re-evaluating open PRs | Default branch |
| Settings drift, waiver expiry, reports | `schedule` in the organization's central policy repository, covering every repository (ADR 0009, decision 8) | Default branch |
| Org-baseline policy changed | Picked up on each PR's next event. Immediate re-evaluation across repositories is deferred. | n/a |

### Token providers

The credential source is configuration, set in the caller workflow on the default branch or in base-branch policy. It is never controlled by the PR.

| Provider | Status | Notes |
|---|---|---|
| Chainguard's public octo-sts (`octo-sts.dev`) | Supported. Default for quick start and the S4 spike. | Nothing to host and no key to handle. Chainguard's App holds installation permissions on the covered repositories, and checks post under the shared `octo-sts` App identity. Not recommended where SOC 2 or ISO 27001 evidence depends on the gate, because it adds a third party with write access (ADR 0008 and ADR 0009 vendor treatment). |
| Self-hosted octo-sts with the org's own App | Supported. Recommended for compliance-bound organizations. | A small stateless service, for example on serverless. The key stays in the org's key store. |
| Vault GitHub secrets engine | Deferred | For organizations already running Vault |
| Cloud key service signing the App JWT, behind a broker | Deferred | The key never leaves the key service |
| App key in an environment secret, App installed on exactly one repository | Deferred | Only safe because the key's reach is one repository |
| Job token only; App key in repo or org secrets; one org-wide App key in each repo's environment | **Not offered** | The check could be faked, or the org-wide key would be exposed |

When the check posts under a shared App identity (the public instance), any trust policy in the same repository that grants `checks: write` could post a fake gate check. Covered repositories must therefore have no other trust policy granting `checks` or `statuses` write. The G2 trigger (ADR 0006) flags new trust-policy files, and the drift job (ADR 0009) checks for such policies.

### Trust policy (required claims)

```yaml
issuer: https://token.actions.githubusercontent.com
subject_pattern: 'repo:<org>/<repo>:.*'
claim_pattern:
  repository_owner: <org>
  ref: refs/heads/<default-branch>
  event_name: (pull_request_target|workflow_run|issue_comment|push|schedule)
  job_workflow_ref: zendern/priskinator/\.github/workflows/gate\.yml@<pinned-sha>
permissions:
  checks: write
  statuses: write
  pull_requests: write
  contents: read
```

- `job_workflow_ref` must name the **central** workflow at a pinned SHA. Never a workflow in the calling repository, and never a ref matching `refs/pull/*`. Prior art in the wild (below) shows that pattern lets a PR edit the workflow and still get a token.
- Callers in another organization must set `id-token: write` explicitly, which GitHub requires for reusable workflows from outside the organization.
- The action-only form (A9) uses a trust policy pinned to the caller file on the default branch (`workflow_ref`) instead, and is documented as the weaker option.

### Rules for the gate job

- `permissions: {}` at workflow level. The gate job gets `id-token: write` and `contents: read` only.
- PR content is read as data and never executed (ADR 0005): no package installs, no build tools, no repository scripts, no local actions, and `persist-credentials: false`.
- Event fields are never interpolated into `run:`. The PR number goes through `env:`, and everything else is fetched from the API.
- No `actions/cache` in the gate workflow.
- `workflow_run` and `issue_comment` jobs trust only the PR number or head SHA from the event, never artifacts or outputs of other runs.
- The central workflow uses only our own pinned action, with no third-party actions, so organizations with action allow lists can permit it easily.
- Our own workflows are linted with zizmor in CI.

### Layers that depend on the plan

| Layer | Availability |
|---|---|
| Environment `riskinator` (default branch only, administrator bypass off), plus an `environment` claim required in the trust policy | Public repositories on any plan. Private repositories need Pro, Team, or Enterprise. |
| Required check pinned to the App, which makes enforcement possible (ADR 0013, phase 3) | Public repositories on any plan. Private repositories need Pro, Team, or Enterprise. |
| CODEOWNERS on `.github/**` with code-owner review required | Everywhere |
| Ruleset required-reviewer rule | Organization-owned repositories only |

On a free private repository, the core still produces a check that can't be faked, in shadow and advisory mode. It just can't be made required.

### Distribution

- The reusable workflow must live in a **public** repository to be callable from other organizations. A workflow in a private repository can only be called from private repositories with the same owner (verified in GitHub's documentation). priskinator will be public. An organization that wants to control its own supply chain can mirror the workflow into a repository it owns and pin its trust policy to the mirror.
- Because adopters pin our SHA, priskinator is a trust anchor and needs a hardened release process (ADR 0011).

### What this does not cover

- GitHub Enterprise Server (unverified).
- GitLab, which needs its own adapter.

### Consequences

- Good, because no long-lived secret exists in any repository, and the org-wide key never sits next to untrusted parsing.
- Good, because a compromised gate job can fake at most one repository's check, for about an hour.
- Good, because adopters host nothing with the public broker, or only a small stateless broker when self-hosting. There is no webhook ingress, queue, or scheduler.
- Good, because evidence reaches a write-once store through OIDC federation, with no stored cloud credentials (ADR 0009). Signed attestations are deferred to v2, and `actions/attest` only covers public repositories and Enterprise Cloud.
- Bad, because it costs Actions minutes and adds runner start-up delay.
- Bad, because every repository needs a caller workflow and a trust-policy file. An installer will open a PR to add them, and both files join the G1 and G2 triggers (ADR 0006).
- Bad, because review events need a `workflow_run` relay with a head-SHA lookup for forks.
- Bad, because upgrades require bumping the pinned SHA in both the caller and the trust policy.
- Bad, because this exact combination has no public precedent, although each piece does.

### Confirmation

Spike S4 in the test organization `toro-fast-CHI`, before M3 implementation starts:

1. Chainguard's public octo-sts installed on the test organization, with the trust policy above. Self-hosting is verified separately when that provider is built.
2. A job added by an attacker in a same-repository PR requests a token and is denied.
3. A plain `pull_request` run requests a token and is denied.
4. A fork PR (from a second account) through the `pull_request_target` caller gets a token, and the check posts as the App.
5. A review on the fork PR flows through the `workflow_run` relay and re-evaluates.
6. The `environment` claim appears, and a `pull_request` run is refused the environment.
7. The action-only form with a caller-pinned policy works as documented.

If S4 fails on items 2, 3, or 4, this ADR is superseded and the App server (A4) returns as the primary design.

## More Information

Prior art (checked 2026-10-13):

- **octo-sts in production:** DataDog has trust policies in `.github/chainguard/` across `datadog-agent`, `dd-trace-*`, and `integrations-core`, including PR-triggered workflows, through its own action fork. Also used at Chainguard and Wolfi, the UK Ministry of Justice, Kartverket, and Liatrio. Trust policies support `claim_pattern` on `job_workflow_ref`, `event_name`, `ref`, and `repository`.
- **Warning from the same prior art:** DataDog's PR policies accept `job_workflow_ref` at `@refs/pull/N/merge` for workflows in the same repository. A PR can edit those workflows and still get tokens. DataDog accepts this because the permissions are low (labels). This design forbids it.
- **Reusable workflow as an identity anchor:** the SLSA GitHub generator, GitHub's documentation on OIDC with reusable workflows, Liatrio autogov-workflows (which deliberately excludes `pull_request` events), and `gh attestation verify --signer-workflow`.
- **Privilege separation for agents:** GitHub Agentic Workflows "safe outputs".
- GitHub `pull_request_target` and environment changes: <https://github.blog/changelog/2025-11-07-actions-pull_request_target-and-environment-branch-protections-changes/>
- octo-sts: <https://github.com/octo-sts/app>
- Example trust policy: <https://github.com/DataDog/datadog-agent/blob/main/.github/chainguard/self.add-labels-pr.label-pr.sts.yaml>
- OIDC with reusable workflows: <https://docs.github.com/en/actions/how-tos/secure-your-work/security-harden-deployments/oidc-with-reusable-workflows>
- Sharing from private repositories: <https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/enabling-features-for-your-repository/managing-github-actions-settings-for-a-repository>
