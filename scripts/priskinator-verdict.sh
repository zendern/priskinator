#!/usr/bin/env bash
# Evaluates HEAD against a base with priskinator itself and writes the verdict
# to OUT (report-only dogfooding, ADR 0013 stage 1). It never gates: the exit
# status is 0 whatever the verdict, and non-zero only when the engine could not
# be built.
#
#   scripts/priskinator-verdict.sh <base> <out-dir>
#
# The change is judged from base to HEAD, starting at their merge base. The
# engine is built from the merge base when it has the evaluate command, so a
# PR can't change the code that judges it; otherwise it is built from HEAD and
# engine.json says so.
#
# Writes to OUT: verdict.json, verdict.txt, and engine.json (which commit built
# the engine, the merge base, and evaluate's exit code).
set -euo pipefail

base=${1:?usage: priskinator-verdict.sh <base> <out-dir>}
out=${2:?usage: priskinator-verdict.sh <base> <out-dir>}

cd "$(git rev-parse --show-toplevel)"
mkdir -p "$out"
out=$(cd "$out" && pwd)
# Clear the previous run first, so a failure below leaves no stale verdict.
rm -f "$out/verdict.json" "$out/verdict.txt" "$out/engine.json"

head=$(git rev-parse HEAD)
merge_base=$(git merge-base "$base" HEAD)

engine_from="base"
engine_commit=$merge_base
if ! git cat-file -e "$merge_base:cmd/priskinator/evaluate.go" 2>/dev/null; then
  engine_from="head"
  engine_commit=$head
fi

work=$(mktemp -d)
trap 'git worktree remove --force "$work/src" >/dev/null 2>&1 || true; rm -rf "$work"' EXIT
git worktree add --quiet --detach "$work/src" "$engine_commit"
(cd "$work/src" && go build -trimpath -ldflags "-X main.version=$engine_commit" -o "$work/priskinator" ./cmd/priskinator)

code=0
"$work/priskinator" evaluate --base "$merge_base" --head "$head" --format json >"$out/verdict.json" || code=$?
"$work/priskinator" evaluate --base "$merge_base" --head "$head" >"$out/verdict.txt" || true

jq -n --arg from "$engine_from" --arg commit "$engine_commit" \
  --arg mergeBase "$merge_base" --arg head "$head" --argjson code "$code" \
  '{engineFrom: $from, engineCommit: $commit, mergeBase: $mergeBase, head: $head, exitCode: $code}' >"$out/engine.json"
