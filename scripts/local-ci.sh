#!/usr/bin/env bash
# Runs the checks from .github/workflows/ci.yml locally and writes a provenance
# record bound to the commit they ran against.
#
# Output goes to .local-ci/<commit>/:
#   attestation.json  in-toto Statement (v1) with a local-ci predicate
#   logs/<step>.log   full output of each step
#   summary.md        PR-ready summary: results, attestation, and logs
#
# The working tree must be clean, so the results describe exactly HEAD. Set
# LOCAL_CI_RUNNER to a short description of who or what ran it (for example
# "laptop of @alice" or "Claude Code cloud session"); it is recorded as given.
#
# Keep STEPS in sync with the jobs in .github/workflows/ci.yml and
# .github/workflows/mutation.yml. Mutation testing covers the packages changed
# since LOCAL_CI_BASE (default origin/main; set it to the PR's base branch).
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

taskfile_var() { sed -n "s/^  $1: //p" Taskfile.yml; }
TASK_VERSION=${TASK_VERSION:-$(taskfile_var TASK_VERSION)}
ZIZMOR_VERSION=${ZIZMOR_VERSION:-$(taskfile_var ZIZMOR_VERSION)}
LOCAL_CI_BASE=${LOCAL_CI_BASE:-origin/main}

# job|step|command
STEPS=(
  "checks|fmt-check|task fmt-check"
  "checks|lint|task lint"
  "checks|vuln|task vuln"
  "checks|licenses|task licenses"
  "checks|zizmor|task zizmor"
  "test|cover|task cover"
  "mutation|mutate-diff|task mutate-diff BASE=$LOCAL_CI_BASE"
)

for tool in git jq sha256sum task go zizmor; do
  command -v "$tool" >/dev/null || { echo "local-ci: $tool not found on PATH" >&2; exit 2; }
done

if [[ -n "$(git status --porcelain -- . ':!.local-ci')" ]]; then
  echo "local-ci: working tree is not clean; commit or stash first" >&2
  git status --short >&2
  exit 2
fi

zizmor_version=$(zizmor --version | awk '{print $2}')
if [[ "$zizmor_version" != "$ZIZMOR_VERSION" ]]; then
  echo "local-ci: zizmor $zizmor_version found, Taskfile.yml pins $ZIZMOR_VERSION" >&2
  exit 2
fi
task_version=$(task --version | sed -E 's/^(Task version: )?v?//; s/ .*//')
if [[ "$task_version" != "$TASK_VERSION" ]]; then
  echo "local-ci: task $task_version found, Taskfile.yml pins $TASK_VERSION" >&2
  exit 2
fi

git rev-parse --verify --quiet "$LOCAL_CI_BASE^{commit}" >/dev/null || {
  echo "local-ci: LOCAL_CI_BASE=$LOCAL_CI_BASE is not a commit; fetch it or set LOCAL_CI_BASE to the PR's base branch" >&2
  exit 2
}

commit=$(git rev-parse HEAD)
tree=$(git rev-parse 'HEAD^{tree}')
branch=$(git rev-parse --abbrev-ref HEAD)
remote=$(git remote get-url origin 2>/dev/null | sed -E 's#^https?://[^@/]*@#https://#' || echo unknown)

out=".local-ci/$commit"
rm -rf "$out"
mkdir -p "$out/logs"

# Install pinned tools up front so their download noise stays out of step logs.
task tools >"$out/logs/tools.log" 2>&1

now() { date -u +%Y-%m-%dT%H:%M:%SZ; }
started=$(now)
steps_json='[]'
overall=pass

for entry in "${STEPS[@]}"; do
  IFS='|' read -r job name cmd <<<"$entry"
  log="$out/logs/$name.log"
  echo "==> [$job] $name: $cmd"
  t0=$(date +%s)
  set +e
  bash -c "$cmd" >"$log" 2>&1
  code=$?
  set -e
  t1=$(date +%s)
  if (( code == 0 )); then status=pass; else status=fail; overall=fail; fi
  echo "    $status (exit $code, $((t1 - t0))s)"
  steps_json=$(jq -c \
    --arg job "$job" --arg name "$name" --arg cmd "$cmd" --arg status "$status" \
    --argjson code "$code" --argjson secs "$((t1 - t0))" \
    --arg sha "$(sha256sum "$log" | cut -d' ' -f1)" \
    '. + [{job: $job, name: $name, command: $cmd, status: $status, exitCode: $code, durationSeconds: $secs, logSha256: $sha}]' \
    <<<"$steps_json")
done
finished=$(now)

# A step that rewrote tracked files would make the results describe a different tree.
if [[ -n "$(git status --porcelain -- . ':!.local-ci')" ]]; then
  echo "local-ci: a step modified the working tree" >&2
  git status --short >&2
  overall=fail
fi

# Module version and the Go that built it, read from the binary itself.
tool_version() { go version -m "$1" | awk 'NR == 1 {built = $2} $1 == "mod" {print $3 " (" built ")"}'; }

jq -n \
  --arg remote "$remote" --arg branch "$branch" --arg commit "$commit" --arg tree "$tree" \
  --arg started "$started" --arg finished "$finished" --arg result "$overall" \
  --arg label "${LOCAL_CI_RUNNER:-unspecified}" \
  --arg ciSha "$(sha256sum .github/workflows/ci.yml | cut -d' ' -f1)" \
  --arg mutationSha "$(sha256sum .github/workflows/mutation.yml | cut -d' ' -f1)" \
  --arg base "$LOCAL_CI_BASE" --arg baseCommit "$(git rev-parse "$LOCAL_CI_BASE^{commit}")" \
  --arg taskfileSha "$(sha256sum Taskfile.yml | cut -d' ' -f1)" \
  --arg os "$(. /etc/os-release 2>/dev/null && echo "$PRETTY_NAME" || uname -s)" \
  --arg kernel "$(uname -sr)" --arg arch "$(uname -m)" \
  --arg go "$(go version)" \
  --arg golangci "$(tool_version bin/golangci-lint)" \
  --arg gofumpt "$(tool_version bin/gofumpt)" \
  --arg govulncheck "$(tool_version bin/govulncheck)" \
  --arg goLicenses "$(tool_version bin/go-licenses)" \
  --arg goTestCoverage "$(tool_version bin/go-test-coverage)" \
  --arg gremlins "$(tool_version bin/gremlins)" \
  --arg zizmor "$zizmor_version" \
  --arg task "$task_version" \
  --argjson steps "$steps_json" \
  '{
    _type: "https://in-toto.io/Statement/v1",
    subject: [{name: ("git+" + $remote + "@" + $branch), digest: {gitCommit: $commit, gitTree: $tree}}],
    predicateType: "https://github.com/zendern/priskinator/attestations/local-ci/v1",
    predicate: {
      result: $result,
      startedOn: $started,
      finishedOn: $finished,
      definition: {ciWorkflowSha256: $ciSha, mutationWorkflowSha256: $mutationSha, taskfileSha256: $taskfileSha},
      mutationBase: {ref: $base, gitCommit: $baseCommit},
      runner: {label: $label, os: $os, kernel: $kernel, arch: $arch},
      tools: {go: $go, task: $task, golangciLint: $golangci, gofumpt: $gofumpt, govulncheck: $govulncheck, goLicenses: $goLicenses, goTestCoverage: $goTestCoverage, gremlins: $gremlins, zizmor: $zizmor},
      steps: $steps
    }
  }' >"$out/attestation.json"

{
  echo "## Local CI provenance"
  echo
  echo "CI checks from \`.github/workflows/ci.yml\` were run locally with \`scripts/local-ci.sh\`."
  echo
  echo "- Commit: \`$commit\` (tree \`$tree\`)"
  echo "- Result: **$overall**"
  echo "- Ran: $started to $finished"
  echo "- Runner: ${LOCAL_CI_RUNNER:-unspecified}, $(uname -sm)"
  echo
  echo "| Job | Step | Command | Result | Time |"
  echo "|---|---|---|---|---|"
  jq -r '.predicate.steps[] | "| \(.job) | \(.name) | `\(.command)` | \(.status) | \(.durationSeconds)s |"' "$out/attestation.json"
  echo
  echo "Differences from hosted CI: zizmor runs with \`--offline\`, tools run from \`./bin\` at the versions pinned in Taskfile.yml, and mutation testing covers the packages changed since \`$LOCAL_CI_BASE\` (\`$(git rev-parse --short "$LOCAL_CI_BASE^{commit}")\`)."
  echo
  echo "<details><summary>Attestation (in-toto Statement)</summary>"
  echo
  echo '```json'
  cat "$out/attestation.json"
  echo '```'
  echo
  echo "</details>"
  for entry in "${STEPS[@]}"; do
    IFS='|' read -r _ name _ <<<"$entry"
    echo
    echo "<details><summary>Log: $name (sha256 $(sha256sum "$out/logs/$name.log" | cut -c1-12)…)</summary>"
    echo
    echo '```text'
    tail -n 200 "$out/logs/$name.log"
    echo '```'
    echo
    echo "</details>"
  done
} >"$out/summary.md"

echo
echo "result: $overall"
echo "provenance: $out/attestation.json"
echo "summary:    $out/summary.md"
[[ "$overall" == pass ]]
