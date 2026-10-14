---
status: accepted
date: 2026-10-13
decision-makers: Nathan Zender
---

# GitHub App hosting and tenancy

## Context and Problem Statement

ADR 0005 chose a dedicated GitHub App as the enforcement identity. The App must receive webhooks, fetch PR data and git objects, run the core binary, call the AI provider, write evidence, and post check runs. It also runs scheduled work: settings-drift checks, re-evaluating open PRs after a policy change, waiver-expiry and break-glass reports. The App's private key is the root of trust: anyone holding it can post a passing check. Who operates the App, and where does it run?

## Decision Drivers

- Protection of the App private key (threat T1).
- Compliance: customer source code, evidence, and AI calls should stay inside the customer's own audit boundary where possible (ADR 0008, ADR 0009).
- Correctness under GitHub webhook behavior: redeliveries, out-of-order events, rapid successive pushes.
- Low operating burden for a first deployment.
- Big PRs: the REST API caps PR file listings and diff sizes, so the App needs git access for large changes.

## Considered Options

### Tenancy

1. **Self-hosted per organization.** Each org registers its own GitHub App from a published manifest and runs the container in its own cloud account. This is the model policy-bot uses.
2. **Multi-tenant hosted service.** We run one public App for all orgs.

### Runtime (for self-hosted)

- **a. Long-running container service.** HTTP webhook receiver, an internal work queue, workers, and a scheduler in one image. Deployable on any container platform.
- **b. Serverless functions.** One function per webhook plus scheduled functions, with a managed queue.
- **c. Thin webhook relay plus GitHub Actions jobs in a protected central repo.** The relay triggers a workflow, and the job authenticates as the App using a private key stored as a secret in that repo.

## Decision Outcome

Chosen: tenancy option 1 (self-hosted per organization) and runtime option a (long-running container service). The core is kept stateless so option b can be added later without redesign.

The tool is planned for use by other organizations, not only its author. Each organization runs its own instance. The project ships:

- a GitHub App manifest, so an org can register its own App in a few clicks
- a signed container image
- reference deployment templates
- an install and operations guide, including key rotation and break-glass during an outage

A hosted multi-tenant offering is not ruled out. The stateless core keeps it possible, but it would need its own ADR covering custody of every customer's App key and the project's place in each customer's vendor-risk scope.

Rationale:

- Self-hosting keeps code, evidence, and AI traffic in the customer's boundary. A multi-tenant service would put this project in every customer's vendor-risk scope and make us custodian of every org's root-of-trust key.
- A single container covers webhooks, queueing, retries, and scheduled jobs without cloud-specific glue, and runs anywhere.
- Option c is rejected. It puts the App private key in Actions secrets, where anyone who can change the central repo's workflows or environments can exfiltrate it and post passing checks. It also adds runner queue latency.

### Runtime requirements

- **Webhook handling:** verify the `X-Hub-Signature-256` signature on every delivery. Deduplicate by delivery ID. Acknowledge quickly and process asynchronously.
- **Ordering:** coalesce events per PR. Evaluate only the latest head SHA and discard results computed for superseded SHAs before posting.
- **Git access:** fetch base and head objects with a short-lived installation token, using a partial or shallow fetch. Never check out into a path where anything executes (ADR 0005).
- **Key handling:** store the App private key in a cloud secret manager or KMS, never in an environment file or image. Rotate on a schedule and on any suspected exposure. Audit every access.
- **Least privilege:** request only the GitHub permissions needed: checks write, statuses write (required for the App to be selectable as a ruleset's expected status source), pull requests read and write (reviewer requests, labels, comments), contents read, metadata read, members read (team membership for separation-of-duties checks), and administration read (settings drift).
- **Scheduled jobs:** settings drift (ADR 0005), re-evaluation after a policy merge (ADR 0005), waiver expiry and break-glass reports (ADR 0009).
- **Observability:** structured logs carry the evaluation and evidence record IDs. Treat evaluation errors as alerts, because they mean every affected PR is blocked (ADR 0003).
- **Availability:** if the App is down, required checks stay pending and merges block. This is fail-closed by design. Break-glass (ADR 0009) is the documented way through an outage.

### Consequences

- Good, because the root-of-trust key and all customer data stay in the customer's environment.
- Good, because one image and one deployment model cover all phases from shadow to enforcing.
- Bad, because every adopting org has to run a service. A published manifest, a container image, and reference deployment templates reduce that cost.
- Bad, because an App outage blocks merges. That is intended, but it puts availability of the App on the critical path for every team.

### Confirmation

- Integration tests for signature rejection, duplicate delivery, out-of-order pushes, and stale-result discard.
- A reference deployment that passes a smoke test: install on a test org, open a PR, and receive a check from the App.
- A permission audit that compares the manifest's requested permissions with this ADR.

## Pros and Cons of the Options

### Self-hosted per org

- Good, because data and keys stay in the customer's boundary, and the customer's existing cloud controls apply.
- Bad, because every org has to operate a service.

### Multi-tenant hosted

- Good, because adoption is a single click.
- Bad, because we hold every customer's root-of-trust key and code, and we enter their audit scope.

### Long-running container

- Good, because it is portable and handles queueing and scheduling in one place.
- Bad, because it is always on, so there is a baseline cost and patching.

### Serverless

- Good, because there is no idle cost.
- Bad, because of cloud-specific glue, execution time limits, and cold starts.

### Relay plus Actions jobs

- Good, because there is the least infrastructure to run.
- Bad, because the root-of-trust key sits in Actions secrets, and there is runner latency.

## More Information

- Building blocks: palantir/go-githubapp (Go) and Probot (TypeScript) are both actively maintained App frameworks.
- policy-bot, as a reference for a self-hosted, manifest-registered App: <https://github.com/palantir/policy-bot>
