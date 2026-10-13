---
status: accepted
date: 2026-10-13
decision-makers: Nathan Zender
---

# Implementation language for the core

## Context and Problem Statement

The core (ADR 0011) is a security control that runs on every PR. It parses diffs, matches paths, runs content rules over syntax trees, computes complexity, calls forge and LLM APIs, and must be shipped as a signed, pinned artifact. Which language should it be written in?

This ADR compares the candidates and records the choice.

## Decision Drivers

- Distribution: a single artifact that runs the same way on GitHub, GitLab, and locally.
- Supply-chain surface of the tool's own dependencies.
- Ecosystem fit: git and diff parsing, glob matching, tree-sitter, forge API clients, LLM SDKs.
- Determinism and testability.
- Contributor familiarity and iteration speed.

## Considered Options

1. Go
2. Rust
3. Python
4. TypeScript

## Comparison

| Criterion | Go | Rust | Python | TypeScript |
|---|---|---|---|---|
| Distribution | Static binary | Static binary | Needs an interpreter; ship as a container | Native GitHub Action runtime; container elsewhere |
| Dependency surface | Small; strong standard library | Small to medium | Medium to large | Largest (npm) |
| Diff and git parsing | go-gitdiff, go-git | gix (gitoxide) | unidiff, PyDriller | parse-diff |
| Glob matching | doublestar | globset | pathspec | picomatch |
| tree-sitter | Official bindings, require cgo | First-class bindings | Official bindings | web-tree-sitter (wasm) |
| Structural search | Shell out to ast-grep or Semgrep | Can embed ast-grep as a library | Semgrep is Python-native | Shell out |
| Complexity metrics | Implement on tree-sitter, or shell out to lizard | Implement on tree-sitter | lizard and radon are native | Implement, or shell out |
| Forge API | go-github | octocrab | PyGithub | Octokit (first-class) |
| GitHub App framework (ADR 0014) | palantir/go-githubapp, used by policy-bot | octocrab app auth; no established framework | gidgethub (lower level) | Probot |
| LLM SDK | Official Anthropic Go SDK | HTTP API or community crate | Official SDK | Official SDK |
| Precedent in this tool category | policy-bot, actionlint, gitleaks, scc, OSV-Scanner | zizmor, ast-grep, difftastic | Semgrep CLI, lizard | Danger JS, most JS Actions |
| Iteration speed | High | Lower | Highest | High |

## Decision Outcome

Chosen option: "Go". Content rules run through a pinned ast-grep binary (a single MIT-licensed binary built on tree-sitter), shipped with the tool in the container image and the CLI release bundles. The core therefore needs no in-process tree-sitter and no cgo. If a future rule needs syntax trees in-process, the official Go tree-sitter bindings (cgo) are the fallback, decided in a new ADR. The GitHub App is built on palantir/go-githubapp (ADR 0014).

The deciding question was: will content rules (ADR 0006, groups G7 and G9) run in-process on syntax trees, or will the tool shell out to pinned ast-grep or Semgrep binaries?

- Shell out: Go wins on simplicity, distribution, contributor pool, and a small dependency tree. cgo for tree-sitter is needed only for suppression detection that ignores string literals, and that can also be delegated to ast-grep.
- In-process: Rust wins, because tree-sitter and ast-grep are native Rust and the analysis core becomes one statically linked binary with no subprocess boundary.

Python is the fastest to prototype, because lizard, radon, PyDriller, and Semgrep are native. It is a reasonable choice for a throwaway spike that validates rules and scoring against historical PRs before committing to a production language. TypeScript is the weakest fit for a security gate, given the size of the npm dependency tree and the repeated npm registry compromises of 2025.

### Consequences

- Go: Good, because it is simple to ship and audit. Bad, because of cgo friction for tree-sitter, and complexity metrics need implementing or shelling out.
- Rust: Good, because tree-sitter and ast-grep are native and it produces one binary. Bad, because iteration is slower and fewer contributors know it.
- Python: Good, because it is the fastest path to a working prototype. Bad, because it needs an interpreter, has a larger dependency tree, and is slower at scale.
- TypeScript: Good, because it is native on GitHub Actions. Bad, because of npm supply-chain risk and weaker fit outside GitHub.

### Confirmation

A spike implementing G9 suppression detection for two languages through ast-grep, plus path matching with renames, measured for binary size, cold-start time, and dependency count. Dependency versions are checked for the latest release when added.

## More Information

- Related: ADR 0011 requires reproducible, signed builds. All four languages can meet that, with different effort.
