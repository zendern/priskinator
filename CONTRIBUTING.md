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
   uv tool install zizmor==1.31.0
   ```
2. Run `prek install` to enable the hooks in `.pre-commit-config.yaml`:
   - pre-commit: file hygiene checks built into prek, then `gofumpt`, `golangci-lint`, and `zizmor` on changed workflows, in parallel
   - commit-msg: Conventional Commits check
   - pre-push: `task test`
3. Run `task check` while you work. `task --list` shows every task.

The other Go tools (golangci-lint, gofumpt, govulncheck, go-licenses) install into `./bin` on first use, at the versions pinned in `Taskfile.yml`, built with the `go.mod` toolchain.

## Pull requests

- Use the PR template and link the issue.
- Run `task ci-local` on the final commit and paste `.local-ci/<commit>/summary.md` into the PR. It runs every CI job locally and records which commit, tools, and results it saw, so reviewers have proof the checks ran even when hosted CI does not. Re-run it after every push.
- Stacked PRs are fine: base the PR on the branch it builds on.
- Changes under `.github/`, policy files, or `docs/adr/` require review from a code owner.
