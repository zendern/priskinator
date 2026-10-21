### 🟡 medium risk, human review required

> [!WARNING]
> Evaluation degraded: AI assessment unavailable

**Triggers:** ` G7.security-paths ` · **Required reviewers:** ` security `

**Score:** 5 points, low tier, medium risk after floors

| Factor | Points | Value | Band | Detail |
|---|--:|--:|---|---|
| size | +5 | 60 | 51-200 | 60 lines |
| complexity_delta | – | – | – | not measured |

Floor: auth code raises the risk to medium for ` auth/login.go `

Excluded from scoring: ` go.sum `

Note: a note

<details>
<summary>Reasons (2)</summary>

| Rule | File | Detail |
|---|---|---|
| ` G7.security-paths ` | ` auth/login.go ` | matches "auth/\*\*" |
| ` score.size ` |  | 60 added and 0 removed lines in 1 file |

</details>
