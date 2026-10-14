# Contributing

## Ground rules

- **Decisions first.** Design decisions are recorded as ADRs in [`docs/adr/`](docs/adr/README.md). A change that alters a decision adds or supersedes an ADR in the same PR.
- **Tests first.** Write the test or golden fixture (`internal/fixture`) before the implementation.
- **Signed, conventional commits.** Every commit is signed and follows [Conventional Commits](https://www.conventionalcommits.org/) (`feat(policy): add layering`). The `commit-msg` hook checks the format.
- **Small, atomic commits.** One logical change per commit.
- **Pinned dependencies.** Check for the latest release before adding or bumping a dependency. Pin GitHub Actions by full commit SHA, and pin runner images to a specific version (`ubuntu-24.04`), never `ubuntu-latest`.

## Setup

1. Install Go (version in `go.mod`), GNU Make, and [lefthook](https://github.com/evilmartians/lefthook).
2. Run `lefthook install` to enable the hooks:
   - pre-commit: `gofumpt`, `golangci-lint`, and `zizmor` on changed workflows
   - commit-msg: Conventional Commits check
   - pre-push: `make test`
3. Run `make check` before opening a PR.

Pinned tools install into `./bin` on first use. See the Makefile for versions.

## Pull requests

- Use the PR template and link the issue.
- Stacked PRs are fine: base the PR on the branch it builds on.
- Changes under `.github/`, policy files, or `docs/adr/` require review from a code owner.
