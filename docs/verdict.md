# Reading a priskinator verdict

priskinator rates every pull request low, medium or high risk. Two things
decide the rating, and the higher of the two wins.

## Triggers

A trigger is a rule for changes that are risky by nature, whatever their size:
CI workflows, security code, new dependencies and so on (ADR 0006). Each
trigger sets a minimum risk and names the reviewers the change needs. The
verdict comment lists every trigger that fired with what it is for. When a
trigger sets a higher risk than the score, the comment says the triggers
decided it.

| Triggers | Covers | Risk | Reviewers |
|---|---|---|---|
| G1, G2 | CI/CD pipelines, priskinator's own policy and repository settings | high | platform |
| G3 to G6 | Lint config, hook managers, test harness, build scripts | medium | platform |
| G7 | Security-sensitive paths and logic | high | security |
| G8 | New dependencies, registries, install hooks, base images | high | security |
| G9 | Suppressions, skipped or deleted tests, CI escape hatches | medium | platform |
| G10 | Submodules, symlinks, newly executable files | medium | platform |

## Score

Five factors each add points. The comment shows only the factors that added
some.

| Factor | Measures | Points |
|---|---|---|
| size | Changed lines (added plus removed) | up to 50: 0, up to 200: 5, up to 500: 15, up to 1000: 25, more: 40 |
| files | Files changed | up to 10: 0, up to 30: 5, up to 100: 15, more: 25 |
| diffusion | Directories touched | up to 2: 0, up to 5: 5, up to 10: 15, more: 25 |
| complexity_delta | Complexity added (not measured yet) | up to 5: 0, up to 15: 10, more: 25 |
| test_signal | Source lines changed when no test changed | up to 20: 0, up to 100: 5, more: 15 |

The total sets the score's risk: under 20 points is low, 20 to 49 is medium,
and 50 or more is high. Lockfiles, vendored code and files marked
`linguist-generated` are left out of every factor. A floor in the policy can
raise the risk for particular paths, and the comment names any that applied.

## Your repository's numbers

These are the built-in defaults from `internal/policy/defaults.yaml`. A
repository's `.priskinator/policy.yaml` can change them. Any change that
weakens a default fires the `policy-loosening` trigger, so it needs human
review before it takes effect (ADR 0010). The JSON output
(`priskinator evaluate --format json`) shows the exact values used for a
verdict.
