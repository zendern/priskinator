# priskinator

priskinator runs in CI against pull requests. It decides, deterministically, when a human must review a change: changes to CI, linters, hooks, tests, auth, or the gate itself always require review, and everything else is tiered by a transparent risk score. An optional AI pass can raise the risk level but never lower it. Every decision is recorded as evidence for SOC 2 and ISO 27001.

Design decisions live in [`docs/adr/`](docs/adr/README.md).

## Development

Requirements: Go (see `go.mod`), [Task](https://taskfile.dev), [prek](https://prek.j178.dev), and [zizmor](https://docs.zizmor.sh). See [CONTRIBUTING](CONTRIBUTING.md) for versions and setup. Linters and other Go tools are installed at pinned versions into `./bin` on first use.

```sh
task test       # go test with the race detector
task lint       # golangci-lint
task fmt        # gofumpt
task check      # fmt-check, lint, test with coverage thresholds
task cover      # tests with the coverage gate (.testcoverage.yml)
task imports    # import boundary rules only (also part of lint)
task mutate     # mutation testing with gremlins (slow; mutate-diff BASE=<ref> for changed lines)
task vuln       # govulncheck
task licenses   # dependency license check
task build      # ./bin/priskinator
task ci-local   # every CI job, plus a provenance record in .local-ci/
```

Layout:

| Path | Purpose |
|---|---|
| `cmd/priskinator` | CLI entry point |
| `internal/changeset` | Change set from git, read as data |
| `internal/policy` | Policy schema, validation, layering |
| `internal/triggers` | Mandatory human-review rules (G1-G10) |
| `internal/content` | ast-grep rule packs over added lines |
| `internal/scoring` | Risk score and factor breakdown |
| `internal/verdict` | Fail-closed verdict |
| `internal/evidence` | Evidence records and sinks |
| `internal/forge/github` | GitHub adapter |
| `internal/ai` | Optional AI assessment |
| `internal/fixture` | Golden-fixture test harness |

## License

[Apache-2.0](LICENSE)
