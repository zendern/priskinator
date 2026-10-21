# AGENTS.md

Guidance for coding agents working in this repository. It points into [README](README.md), [CONTRIBUTING](CONTRIBUTING.md), and [`docs/adr/`](docs/adr/README.md) rather than repeating them; read those for the full rules.

## What this is

priskinator is a Go CLI that runs in CI against pull requests and decides, deterministically, when a human must review a change. Mandatory triggers (changes to CI, linters, hooks, tests, auth, or the gate itself) always require review. Everything else is tiered by a transparent additive risk score. An optional AI pass can raise the risk, never lower it. Every decision is recorded as evidence for SOC 2 and ISO 27001.

The project is early. `changeset`, `policy`, and `fixture` are implemented. `triggers`, `content`, `scoring`, `verdict`, `evidence`, `forge/github`, and `ai` hold only a package doc comment, and the CLI has only `version`. The design is ahead of the code: each `internal/` package's doc comment names its ADR, so read that ADR before changing or filling in the package.

## Commands

Everything runs through [Task](https://taskfile.dev). The Go tools (golangci-lint, gofumpt, govulncheck, go-licenses, go-test-coverage, gremlins) install at the versions pinned in `Taskfile.yml` into `./bin` on first use.

```sh
task check                                          # fmt-check, lint, tests with the coverage gate: run before calling work done
task test                                           # go test -race ./...
go test -race -count=1 -run TestMergeMixed ./internal/policy/   # a single test
task lint                                           # golangci-lint, including the import boundaries and the ruleguard rule
task fmt                                            # gofumpt -w .
task cover                                          # tests with the 95% coverage gate
task mutate-diff BASE=origin/main                   # mutation-test the packages changed since BASE (slow)
task vuln                                           # govulncheck
task licenses                                       # go-licenses against the allow list
task build                                          # ./bin/priskinator
```

Hooks run through [prek](https://prek.j178.dev) (`prek install` once): gofumpt, golangci-lint and zizmor before a commit, Conventional Commits on the message, and `task cover` before a push. CI runs every hook with `--all-files`, plus `task vuln`, `task licenses`, `task cover`, and mutation testing on PRs that target `main`.

## Architecture

The verdict pipeline (ADR 0003): change set, then mandatory triggers, content rules, and the risk score (plus the optional AI result), combined into a fail-closed verdict that is written as evidence.

- **`internal/changeset`**: `FromGit` builds the change set between two full commit IDs. The PR head is untrusted (ADR 0005), so git runs only as plumbing with a scrubbed environment, no hooks or fsmonitor, no textconv or external diff, attributes read from an empty tree, and binary judged from content. Keep every new git call inside those settings. Tests build scratch repositories with real `git`.
- **`internal/policy`**: policy schema version 1. The JSON Schema (`policy.v1.schema.json`) and the built-in `defaults.yaml` are embedded. `Parse` validates one layer: a single YAML document with no duplicate keys, a supported version, the schema (unknown keys are errors), then the checks the schema cannot express. `Resolve` layers the defaults, the org baseline (`Overlay`: trusted, may replace anything), then the repo policy (`Merge`: tighten-only). A repo loosening is dropped field by field and fires `policy-weakening-attempt`. An invalid repo policy is ignored and fires `config-invalid`. An invalid org baseline is an error. What "tighter" means per field is documented on `Merge`.
- **`internal/fixture`**: golden fixtures, each a YAML synthetic PR with the expected risk, review requirement, triggers, and reviewers (`testdata/valid/`). Unknown fields are rejected. Expectations are not exhaustive, so fixtures survive new rules. Unit tests and the planned `priskinator test` command share this harness (ADR 0010).
- **`cmd/priskinator`**: `main` only calls `run(args, stdout, stderr) int`. Logic and tests go in `run.go`; `main.go` is the only coverage exclusion.

Fail closed: an unknown or invalid value must make a verdict stricter, never looser. For example, `Risk.Rank` ranks unknown risks highest.

## Rules CI enforces

- **Import boundaries** (depguard in `.golangci.yml`, from ADR 0011 and ADR 0005). The core (`changeset`, `content`, `policy`, `scoring`, `triggers`, `verdict`) imports nothing from the network (`net`, `net/http`, `crypto/tls`, go-github) and never imports `forge`, `ai`, or `evidence`: pass their data in instead. Only `changeset` and `content` may import `os/exec`; tests may too. Use `go.yaml.in/yaml/v3`, not `gopkg.in/yaml.*`, and `errors` with `fmt.Errorf("%w")`, not `github.com/pkg/errors`. A new boundary is a new depguard rule that names its ADR.
- **No tagless switches** (`switch { case cond: ... }`) outside tests. Write `if` statements. gremlins cannot mutate case conditions, so `tools/lint/rules.go` bans them. Tagged and type switches are fine.
- **Coverage**: at least 95% for every file, every package, and the total (`.testcoverage.yml`). Lowering a threshold or excluding a path needs a reason in that file.
- **Mutation testing**: at least 95% efficacy and 95% mutant coverage, and at most 2% timed-out mutants (`.gremlins.yaml`, `scripts/gremlins.sh`). A surviving mutant needs an assertion that kills it, or a reason in the PR.
- **`//nolint`** must name the linter and give a reason.

## Working conventions

- Write the test or golden fixture before the implementation (aka use Mutation Test Driven Development MTDD, so RED test, GREEN test, MUTATE code, REFACTOR test and code).
- A change that alters a design decision adds or supersedes an ADR in the same PR (start from `docs/adr/template.md` and update the index in `docs/adr/README.md`).
- Commits are signed, follow Conventional Commits, and hold one logical change each.
- Pin everything: actions by full commit SHA with a `# vX.Y.Z` comment, runners by version (never `*-latest`), remote prek hooks by frozen SHA, and tool versions as `Taskfile.yml` vars with a `# renovate:` comment so Renovate bumps them. Check for the latest release before adding or bumping a dependency.
- Stacked PRs are normal: base a PR on the branch it builds on. Use `.github/pull_request_template.md`.
- Changes under `.github/`, policy files, or `docs/adr/` need a code owner's review.
