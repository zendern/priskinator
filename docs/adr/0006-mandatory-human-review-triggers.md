---
status: accepted
date: 2026-10-13
decision-makers: Nathan Zender
---

# Mandatory human-review triggers: path rules plus content rules

## Context and Problem Statement

Some changes must always get human review regardless of their size or risk score: anything that could weaken the checks themselves, touch security-sensitive logic, or hide what is really changing. The initial list was auth code, CI workflows, linter config, and hook-manager config (pre-commit, lefthook, prek). That list is path-based, but much of the gaming happens inside files nobody would think to watch, such as a `package.json` script, a `pyproject.toml` coverage threshold, or a single `# noqa`. Which changes trigger mandatory review, and how are they detected?

## Decision Drivers

- Threats T1–T7 from ADR 0002.
- Deterministic and explainable: every trigger points at a file and, where possible, a line.
- Low noise: routine dependency bumps must not flood reviewers, or reviewers will start rubber-stamping.
- Configurable per repo, but a repo cannot switch off the org baseline (ADR 0010).

## Considered Options

1. Path-only rules, delegated entirely to CODEOWNERS and ruleset required-reviewer rules
2. Path rules plus content rules evaluated by pr-riskinator
3. AI-only detection

## Decision Outcome

Chosen option: "Path rules plus content rules", because path rules alone cannot see suppressions, test tampering, or script redirection, and AI-only detection is not deterministic. Native forge path rules stay in place as defense in depth.

### Trigger groups (default baseline)

Each rule has a stable ID used in verdicts, evidence, routing, and waivers.

**G1 — CI/CD pipeline definitions** (T1, T2)
`.github/workflows/**`, `.github/actions/**`, `action.yml`, `.gitlab-ci.yml`, `.gitlab/**`, `Jenkinsfile`, `.circleci/**`, `azure-pipelines.yml`, `.buildkite/**`, `bitbucket-pipelines.yml`

**G2 — Gate and governance files** (T1)
pr-riskinator policy and prompt files, `CODEOWNERS` in any supported location, `.gitattributes`, `.gitmodules`, `.github/dependabot.yml`, `renovate.json*`, settings-as-code files

**G3 — Lint, format, and type-check configuration** (T2)
`.eslintrc*`, `eslint.config.*`, `.prettierrc*`, `biome.json*`, `ruff.toml`, `.ruff.toml`, `.flake8`, `.pylintrc`, `mypy.ini`, `pyrightconfig.json`, `.golangci.y*ml`, `.rubocop.yml`, `tsconfig*.json`, `.stylelintrc*`, `clippy.toml`, `rustfmt.toml`, plus tool sections inside `pyproject.toml` and `setup.cfg` (`[tool.ruff]`, `[tool.mypy]`, `[tool.pylint]`, and similar)

**G4 — Hook managers and commit gates** (T2)
`.pre-commit-config.yaml` (also used by prek), `lefthook.yml`, `lefthook-local.yml`, `.lefthook/**`, `.husky/**`, `.lintstagedrc*`, `.githooks/**`, `commitlint.config.*`

**G5 — Test harness and thresholds** (T2, T4)
`pytest.ini`, `tox.ini`, `noxfile.py`, `conftest.py`, `jest.config.*`, `vitest.config.*`, `karma.conf.*`, `.mocharc*`, `playwright.config.*`, `codecov.yml`, `.coveragerc`, `.nycrc*`, plus `[tool.pytest.*]` and `[tool.coverage.*]` sections and coverage keys in `package.json`

**G6 — Build and task scripts that CI invokes** (T2)
`Makefile`, `justfile`, `Taskfile.y*ml`, `mise.toml` tasks, the `scripts` key in `package.json`, `scripts/ci/**`, test-skipping build options such as Maven Surefire `skipTests`

**G7 — Security-sensitive code** (T5)
- Paths, configured per repo, for example `**/auth/**`, `**/authz/**`, `**/security/**`, `**/crypto/**`, `**/session*/**`, `**/middleware/*auth*`
- Content rules, per language: removed auth decorators or guards, disabled TLS verification, JWT verification without an algorithm allow-list, wildcard CORS with credentials, cookie security flags turned off, hardcoded credentials, new raw SQL construction

**G8 — Supply chain** (T6)
- A new direct dependency in any manifest
- Registry or index configuration: `.npmrc`, `.yarnrc.yml`, `pip.conf`, index URLs in `pyproject.toml` or `uv.toml`, `.cargo/config.toml`, `replace` directives in `go.mod`
- Package install hooks: `preinstall`, `install`, `postinstall`, `prepare`
- Container base image changes in `Dockerfile*`

Version bumps of existing dependencies are scored as risk (ADR 0007), not mandatory triggers, so Renovate and Dependabot PRs do not flood reviewers. A repo can make them mandatory.

**G9 — Gaming signals in added lines** (T3, T4)
Evaluated on added lines only, using tree-sitter so matches inside string literals are ignored:
- Suppressions: `# noqa`, `# type: ignore`, `# pylint: disable`, `# pragma: no cover`, `eslint-disable*`, `@ts-ignore`, `@ts-expect-error`, `@ts-nocheck`, `istanbul ignore`, `//nolint`, `#[allow(...)]`, `@SuppressWarnings`, `rubocop:disable`
- Skips: `pytest.mark.skip`, `pytest.mark.xfail`, `pytest.skip(`, `unittest.skip`, `it.skip`, `describe.skip`, `test.skip`, `xit`, `xdescribe`, `t.Skip(`, `#[ignore]`, `@Disabled`, `@Ignore`
- CI escape hatches: `continue-on-error: true`, `|| true`, `if: false`, `--no-verify`, `exit 0` added to scripts
- Test deletion: deleted test files, or a net decrease in assertion count in test files
- Snapshot or golden-file changes with no non-test source change in the same PR

**G10 — Hidden-change vectors** (T7)
- Submodule pointer changes
- Added or changed symlinks
- File mode changes to executable
- Binary files and Git LFS pointer changes
- Files above a size cap
- Any rename or deletion whose old path matches a rule in G1–G8

### Matching rules

- Every rule is evaluated against both the old and new path of renames.
- Paths are normalized. Case-insensitive matching is on by default, because macOS and Windows checkouts are case-insensitive.
- Multi-purpose files (`pyproject.toml`, `package.json`, `setup.cfg`) are matched by section or key where a parser exists, so a version bump does not trip the lint-config rule. Until a parser exists for a file type, the whole file matches.
- Each trigger maps to a reviewer team for routing (for example G1 to platform, G7 to security).
- A trigger instance can be waived only through a waiver in base-branch policy (ADR 0009), never by anything inside the PR, such as a code comment.

### Consequences

- Good, because the main gaming techniques are detected deterministically, with file and line evidence.
- Good, because CODEOWNERS and ruleset rules remain an independent second layer for paths.
- Bad, because content rules need per-language maintenance and will produce false positives at first. Shadow mode (ADR 0013) is how we tune them.
- Bad, because the list will never be complete. The AI layer (ADR 0008) and periodic review of escaped issues cover the remainder.

### Confirmation

- One or more golden fixtures per rule ID: a positive case, a negative case, and a rename case.
- Fixtures for every example in threat table T1–T7.
- A generated report of the effective rule set per repo, reviewed when the baseline changes.

## Pros and Cons of the Options

### Path-only via CODEOWNERS and rulesets

- Good, because it is native, already enforced by the forge, and the PR cannot edit ruleset configuration.
- Bad, because it cannot see suppressions, skips, script redirection, or semantic auth changes.

### Path plus content rules

- Good, because it covers both what changed and how it changed.
- Bad, because it means more code to maintain and test.

### AI-only

- Good, because it can catch novel tricks.
- Bad, because it is not deterministic and can be prompt-injected.

## More Information

- Follow-up: generate ruleset required-reviewer rules from the G1–G8 path lists so the native layer stays in sync with the policy.
- GitHub required reviewer rule: <https://github.blog/changelog/2026-02-17-required-reviewer-rule-is-now-generally-available/>
