# Contributing

## Ground rules

- **Decisions first.** Design decisions are recorded as ADRs in [`docs/adr/`](docs/adr/README.md). A change that alters a decision adds or supersedes an ADR in the same PR.
- **Tests first.** Write the test or golden fixture (`internal/fixture`) before the implementation.
- **Signed, conventional commits.** Every commit is signed and follows [Conventional Commits](https://www.conventionalcommits.org/) (`feat(policy): add layering`). The `commit-msg` hook ([conventional-pre-commit](https://github.com/compilerla/conventional-pre-commit)) checks the format.
- **Small, atomic commits.** One logical change per commit.
- **Pinned dependencies.** Check for the latest release before adding or bumping a dependency. Pin GitHub Actions by full commit SHA (zizmor fails a tag like `actions/checkout@v5`; after adding or changing a `uses:` line, `task pin` pins it with [ratchet](https://github.com/sethvargo/ratchet), using `GITHUB_TOKEN` if set, and Renovate keeps it current), and pin runner images to a specific version (`ubuntu-24.04`), never `ubuntu-latest`. Give every workflow job a `timeout-minutes`, so a hung run is cancelled long before GitHub's 6-hour default. A job that calls a reusable workflow (`uses:`) can't set one; the called workflow's jobs carry the limits instead. The shared `no-latest-runners` and `require-timeouts` hooks check both.

## Setup

1. Install Go 1.21 or newer, then [Task](https://taskfile.dev) at the `TASK_VERSION` pinned in `Taskfile.yml`:
   ```sh
   go install github.com/go-task/task/v3/cmd/task@v"$(sed -n 's/^  TASK_VERSION: *//p' Taskfile.yml)"
   ```
2. Run `task setup` (alias `task install-tools`) in the main checkout, not a worktree: worktrees share its git hooks, and the hooks run the prek in the `./bin` of whichever checkout installed them. It installs every other tool into `./bin` at the versions pinned in `Taskfile.yml` ([prek](https://prek.j178.dev), golangci-lint, gofumpt, govulncheck, go-licenses, go-test-coverage, gremlins), then the hooks in `.pre-commit-config.yaml`:
   - pre-commit: file hygiene checks built into prek, then `gofumpt`, `golangci-lint`, `go mod tidy -diff`, [zizmor](https://docs.zizmor.sh) on changed workflows and actions, and the Renovate config validator on `renovate.json`, in parallel
   - commit-msg: Conventional Commits check
   - pre-push: `task cover` (tests plus the coverage gate)
3. Run `task check` while you work. `task --list` shows every task.

Every `go` command Task runs uses the exact toolchain in `go.mod` (`GOTOOLCHAIN`), which Go downloads when yours is a different version, so local results match CI. The Go tools are built with it too. prek installs each remote hook's tool, zizmor included, at the version its `rev` pins.

## Quality gates

- **Import boundaries** are depguard rules in `.golangci.yml`, so `task lint` enforces them (`task imports` runs only them). The evaluation core (`changeset`, `content`, `policy`, `scoring`, `triggers`, `verdict`) has no network imports and does not import `forge`, `ai`, or `evidence` (ADR 0011). Only `changeset` and `content` may import `os/exec` (ADR 0005); tests may too. A new boundary is a new rule there, with the ADR it comes from in its `desc`.
- **No tagless switches**: write `if` statements instead of `switch { case cond: ... }`. gremlins can't mutate case conditions, because Go's coverage profile has no statement on a case line, so those mutants would never run. A gocritic ruleguard rule in `tools/lint/rules.go` enforces this in `task lint`. Tagged (`switch x {`) and type switches are fine, and so is anything in tests, which aren't mutated.
- **Coverage**: `task cover` runs the race tests with a coverage profile and fails under 95% for any file, package, or the total (`.testcoverage.yml`). CI runs it in place of `task test`. Excluding a path needs a reason in that file; today only `cmd/priskinator/main.go`, which holds just `main`, is excluded.
- **Mutation testing**: [gremlins](https://gremlins.dev) mutates covered code and fails under 95% test efficacy or 95% mutant coverage (`.gremlins.yaml`), or when more than 2% of mutants time out, because gremlins leaves timed-out mutants out of efficacy. It takes minutes rather than seconds, so it isn't in `task check` or the hooks. `task mutate-diff BASE=<branch>` mutates every package with Go changes since that branch, tests included, so a PR that only adds tests is measured too. The `mutation` workflow runs that on non-draft PRs that target `main` (a stacked PR runs it once it is retargeted to `main`) and `task mutate` on the whole module weekly. A surviving (`LIVED`) mutant is a line whose change no test notices: add the assertion, or say in the PR why it doesn't matter (for example, an equivalent mutant).

## Pull requests

- Use the PR template and link the issue.
- Stacked PRs are fine: base the PR on the branch it builds on.
- Changes under `.github/`, policy files, or `docs/adr/` require review from a code owner.
