### 🔴 high risk, human review required

**Triggers:** ` G2.gate-governance ` (priskinator policy and prompts, trust policies, ownership and repository settings), ` policy-loosening ` · **Required reviewers:** ` platform `

**Decided by:** the triggers above. On its own the score is low risk.

**Score:** 0 points; medium starts at 20 and high at 50

<sub>Factors that added no points are hidden. [How priskinator scores a PR](https://github.com/zendern/priskinator/blob/main/docs/verdict.md)</sub>

<details>
<summary>Reasons (2)</summary>

| Rule | File | Detail |
|---|---|---|
| ` G2.gate-governance ` | ` .priskinator/policy.yaml ` | matches ".priskinator/\*\*" |
| ` policy-loosening ` | ` /limits/max_changed_lines ` | cannot raise 5000 to 9000 |

</details>
