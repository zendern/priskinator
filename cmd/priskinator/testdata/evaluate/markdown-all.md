### 🟡 medium risk, human review required

> [!WARNING]
> Evaluation degraded: AI assessment unavailable

**Triggers:** ` G7.security-paths ` (Security \*paths\*) · **Required reviewers:** ` security `

**Decided by:** the score.

**Score:** 5 points; medium starts at 20 and high at 50

| Factor | Points | Detail |
|---|--:|---|
| size | +5 | 60 lines |

<sub>Factors that added no points are hidden. [How priskinator scores a PR](https://github.com/zendern/priskinator/blob/main/docs/verdict.md)</sub>

Floor: auth code raises the risk to medium for ` auth/login.go `

Excluded from scoring: ` go.sum `

Note: a note

<details>
<summary>Reasons (1)</summary>

| Rule | File | Detail |
|---|---|---|
| ` G7.security-paths ` | ` auth/login.go ` | matches "auth/\*\*" |

</details>
