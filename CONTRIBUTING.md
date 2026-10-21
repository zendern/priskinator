# Contributing

## Ground rules

- **Decisions first.** Design decisions are recorded as ADRs in [`docs/adr/`](docs/adr/README.md). A change that alters a decision adds or supersedes an ADR in the same PR.
- **Tests first.** Write the test or golden fixture (`internal/fixture`) before the implementation.
- **Signed, conventional commits.** Every commit is signed and follows [Conventional Commits](https://www.conventionalcommits.org/) (`feat(policy): add layering`). The `commit-msg` hook ([conventional-pre-commit](https://github.com/compilerla/conventional-pre-commit)) checks the format.
- **Small, atomic commits.** One logical change per commit.
- **Pinned dependencies.** Check for the latest release before adding or bumping a dependency. Pin GitHub Actions by full commit SHA, and pin runner images to a specific version (`ubuntu-24.04`), never `ubuntu-latest`.

## Setup

1. Install Go (version in `go.mod`), [Task](https://taskfile.dev), [prek](https://prek.j178.dev), and [zizmor](https://docs.zizmor.sh). Task and zizmor must match `TASK_VERSION` and `ZIZMOR_VERSION` in `Taskfile.yml`:
   ```sh
   go install github.com/go-task/task/v3/cmd/task@v"$(sed -n 's/^  TASK_VERSION: *//p' Taskfile.yml)"
   uv tool install prek
   uv tool install zizmor=="$(sed -n 's/^  ZIZMOR_VERSION: *//p' Taskfile.yml)"
   ```
2. Run `prek install` to enable the hooks in `.pre-commit-config.yaml`:
   - pre-commit: file hygiene checks built into prek, then `gofumpt`, `golangci-lint`, `go mod tidy -diff`, `zizmor` on changed workflows, and the Renovate config validator on `renovate.json`, in parallel
   - commit-msg: Conventional Commits check
   - pre-push: `task cover` (tests plus the coverage gate)
3. Run `task check` while you work. `task --list` shows every task.

The other Go tools (golangci-lint, gofumpt, govulncheck, go-licenses, go-test-coverage, gremlins) install into `./bin` on first use, at the versions pinned in `Taskfile.yml`, built with the `go.mod` toolchain.

## Quality gates

- **Import boundaries** are depguard rules in `.golangci.yml`, so `task lint` enforces them (`task imports` runs only them). The evaluation core (`changeset`, `content`, `policy`, `scoring`, `triggers`, `verdict`) has no network imports and does not import `forge`, `ai`, or `evidence` (ADR 0011). Only `changeset` and `content` may import `os/exec` (ADR 0005); tests may too. A new boundary is a new rule there, with the ADR it comes from in its `desc`.
- **No tagless switches**: write `if` statements instead of `switch { case cond: ... }`. gremlins can't mutate case conditions, because Go's coverage profile has no statement on a case line, so those mutants would never run. A gocritic ruleguard rule in `tools/lint/rules.go` enforces this in `task lint`. Tagged (`switch x {`) and type switches are fine, and so is anything in tests, which aren't mutated.
- **Coverage**: `task cover` runs the race tests with a coverage profile and fails under 95% for any file, package, or the total (`.testcoverage.yml`). CI runs it in place of `task test`. Excluding a path needs a reason in that file; today only `cmd/priskinator/main.go`, which holds just `main`, is excluded.
- **Mutation testing**: [gremlins](https://gremlins.dev) mutates covered code and fails under 95% test efficacy or 95% mutant coverage (`.gremlins.yaml`), or when more than 2% of mutants time out, because gremlins leaves timed-out mutants out of efficacy. It takes minutes rather than seconds, so it isn't in `task check` or the hooks. `task mutate-diff BASE=<branch>` mutates every package with Go changes since that branch, tests included, so a PR that only adds tests is measured too. The `mutation` workflow runs that on non-draft PRs that target `main` (a stacked PR runs it once it is retargeted to `main`) and `task mutate` on the whole module weekly. A surviving (`LIVED`) mutant is a line whose change no test notices: add the assertion, or say in the PR why it doesn't matter (for example, an equivalent mutant).

## Pull requests

- Use the PR template and link the issue.
- Run `task ci-local` on the final commit and paste `.local-ci/<commit>/summary.md` into the PR. It runs every CI job locally and records which commit, tools, and results it saw, so reviewers have proof the checks ran even when hosted CI does not. Re-run it after every push. Set `LOCAL_CI_BASE` to the PR's base branch (default `origin/main`) so its mutation step covers what the PR changes.
- Stacked PRs are fine: base the PR on the branch it builds on.
- Changes under `.github/`, policy files, or `docs/adr/` require review from a code owner.
