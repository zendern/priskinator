---
status: proposed
date: 2026-10-15
decision-makers: Nathan Zender
---

# First standard-change classes, and a lighter bar for enabling them

## Context and Problem Statement

ADR 0004 makes bot approval of simple PRs the goal (option B) but defines no change classes, so `bot_eligible` is always false. Its preconditions for enabling a class are a GRC-approved standard-change procedure, an auditor walkthrough, and a measured escape rate below an agreed threshold. No GRC function or auditor is engaged (ADR README, "Compliance scope"), so as written no class can ever be enabled, and every PR keeps needing a human. The point of the project is to let simple changes through.

Which changes are the first standard-change classes, and what evidence is enough to turn one on?

## Decision Drivers

- Reduce reviewer load on changes a human adds nothing to (ADR 0002).
- A wrong bot approval is an unreviewed change, so each class must be narrow enough that the deterministic rules can prove membership.
- Every class still passes through every mandatory trigger (ADR 0006). A class narrows what is eligible; it never waives a trigger.
- Enabling a class is a loosening, so it needs a human's review and an explicit callout (ADR 0010 as revised on 2026-10-15).
- The bar has to be reachable by a single-maintainer project with no GRC function, and tighten automatically for an organization that has one.

## Considered Options

1. Keep ADR 0004's preconditions unchanged
2. Define the first classes, keep the safety mechanisms, and replace the GRC and auditor gates with policy-owner sign-off backed by shadow data
3. Enable classes by configuration alone, with no shadow period

## Decision Outcome

Chosen option: "Define the first classes, policy-owner sign-off backed by shadow data", because it lets simple changes through within weeks while keeping every mechanism that fails safe: shadow evidence first, a canary, a kill switch, retrospective sampling, and automatic fallback on an escape.

This amends the preconditions in ADR 0004 option B and the exit criteria of phase 4 in ADR 0013. Everything else in ADR 0004 stands, including the eligibility conditions every class shares.

### Conditions every class shares (unchanged from ADR 0004)

- `risk: low`, no triggers, `evaluation: ok`
- Within the class's size cap
- The evidence record was written
- The approval canary is passing (ADR 0004)
- The App's approval is for the current head SHA; a new push dismisses it and the PR is evaluated again

### Classes

| Class | Matches when | Size cap | AI must agree |
|---|---|---|---|
| `docs-only` | Every changed file matches the class's docs globs. Defaults: `**/*.md`, `**/*.mdx`, `**/*.rst`, excluding `.github/**`. | 300 changed lines | No |
| `dependency-patch` | The PR author is a configured dependency bot (default: Renovate and Dependabot, matched by account ID, not login text). Every changed file is a supported manifest or its lockfile, and every change is a patch-level bump of an existing dependency. | none beyond the policy's oversize limits, which already exclude lockfiles | No |
| `tests-only` | Every changed file matches the class's test globs (defaults: `**/*_test.go`, `**/*.test.*`, `**/*.spec.*`, `**/test_*.py`, `**/tests/**`). No test file is deleted and the assertion count does not go down. | 300 changed lines | Yes, when the AI layer is enabled |

What keeps each class narrow:

- **docs-only.** The globs match by extension, so a script under `docs/` does not match. Markdown that drives behavior is either covered by a trigger (AI prompt files under G2) or excluded by default (`.github/**`, which holds PR and issue templates). A repo whose docs carry decisions, such as this one's `docs/adr/**`, narrows the class in its repo policy so ADR changes still get a human.
- **dependency-patch.** A new direct dependency, a registry change, or an install hook fires G8. A bump in any file G6 covers, such as tool versions pinned in `Taskfile.yml`, fires G6, so those Renovate PRs still need a human. Changes to the language version (`go` or `toolchain` lines in `go.mod`, `engines` in `package.json`) do not match. Only manifests with a parser match: `go.mod` and `go.sum` first, other ecosystems as parsers are added. An unsupported manifest never matches.
  The bot approves and no human is involved. ADR 0015 v1 already records these PRs as `responsible_user: unverified`. Retrospective sampling (below) is the human control for this class.
- **tests-only.** Skips, suppressions and assertion loss fire G9. Test harness and coverage config fire G5. A golden or snapshot change with no non-test source change fires G9, so golden updates never match. The residual risk is a shared test helper weakened so that other tests pass vacuously without losing assertions. That is semantic, so this class needs the AI's agreement whenever the policy enables the AI layer.

### Policy shape

Classes live in policy, so the org baseline can turn them off and a repo can narrow them.

```yaml
standard_changes:
  docs-only:
    mode: shadow          # off | shadow | on
    paths: ["**/*.md", "**/*.mdx", "**/*.rst"]
    exclude_paths: [".github/**", "docs/adr/**"]   # docs/adr is this repo's narrowing
    max_changed_lines: 300
  dependency-patch:
    mode: shadow
    authors: [renovate, dependabot]  # resolved to account IDs
    ecosystems: [gomod]
  tests-only:
    mode: shadow
    paths: ["**/*_test.go"]
    max_changed_lines: 300
```

- Built-in defaults ship every class in `shadow`, so `bot_eligible` and `change_class` start accumulating evidence on every PR from phase 1.
- Moving a class to `on`, widening its globs, raising its cap, or adding an author is a loosening. The PR that does it fires `policy-loosening` and needs a human (ADR 0010).
- Narrowing, lowering a cap, or moving to `off` is a tightening and needs nothing extra.

### Enabling a class

Replaces the ADR 0004 preconditions and the ADR 0013 phase 4 exit criteria:

1. **Shadow data.** The class has run in `shadow` for at least 3 weeks and has at least 15 shadow-eligible PRs.
2. **No escapes in shadow.** An escape is a shadow-eligible PR where the human reviewer requested changes, or that was reverted or labeled `priskinator/incident` within 14 days of merging. With samples this small, the threshold is zero. Each escape is explained in the enabling PR, and the class is narrowed until the escape would no longer have matched, which restarts the shadow count.
3. **Policy-owner sign-off.** A PR to the policy moves the class to `on`, with the shadow metrics attached. The policy owner's approval of that PR is the sign-off. An organization with a GRC function requires GRC approval of that PR in its own baseline, for example through CODEOWNERS on the policy repository. The auditor walkthrough is no longer a precondition to enabling. It is still needed before the bot approvals are relied on as audit evidence (ADR 0009).

### While a class is on

- **Retrospective sampling.** The policy owner reviews at least 10% of bot-approved PRs per class, and at least one a week when there are any, and records the result in the evidence store. A finding counts as an escape.
- **Automatic fallback.** An escape on a bot-approved PR moves that class back to `shadow`, as does a failing canary for every class. Falling back is the fail-safe direction, so it happens without a human. Turning the class back on goes through "Enabling a class" again.
- **Kill switch.** Unchanged from ADR 0004: one setting moves every class in every repo back to option A.

### Consequences

- Good, because simple PRs can merge without a human weeks after shadow mode starts, instead of never.
- Good, because each class is defined by deterministic rules that already exist (triggers, scoring, path globs), plus two new checks: patch-level bump detection and assertion counting.
- Good, because an organization with GRC tightens the bar in its baseline without a code change.
- Bad, because policy-owner sign-off is weaker audit evidence than GRC sign-off. A SOC 2 audit period that relies on bot approvals still needs the auditor walkthrough first.
- Bad, because zero escapes over 15 PRs says little statistically. Sampling and automatic fallback are what limit the damage of a class that turns out to be wrong.
- Bad, because `dependency-patch` merges PRs that no human touches, and ADR 0015 v1 has no accountable human for them.

### Confirmation

- Golden fixtures per class: a matching PR, a PR that fails each class condition, and a PR that matches the class but fires a trigger (not eligible).
- A test that a class in `shadow` sets `change_class` and `bot_eligible` but never produces an approving review.
- A policy-layer test that moving a class to `on` or widening it is reported as a loosening.
- A test that an escape label on a bot-approved PR moves its class back to `shadow`.

## Pros and Cons of the Options

### Keep ADR 0004's preconditions unchanged

- Good, because it is the strongest audit position.
- Bad, because with no GRC function no class can ever be enabled, so the tool never reduces the number of reviews.

### Classes plus policy-owner sign-off on shadow data

- Good, because it is reachable now, and every fail-safe mechanism stays.
- Bad, because the evidence for each class is a small sample.

### Enable by configuration alone

- Good, because it is fastest.
- Bad, because a class would approve PRs before anyone has seen how it behaves on real ones.

## More Information

- ITIL 4 calls pre-authorized, low-risk, well-understood changes "standard changes". These classes are that.
- Emergency changes already have a path: break-glass in ADR 0009.
- Background on risk levels and frameworks: the project's risk-levels note (2026-10-15), which recommends keeping three risk levels and treating these classes as a separate change type.
