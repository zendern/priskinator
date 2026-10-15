#!/usr/bin/env bash
# Runs gremlins on one target (a package directory, or . for the module) with
# the thresholds in .gremlins.yaml. It also fails when more than
# MAX_TIMED_OUT_PERCENT of the mutants timed out: gremlins leaves timed-out
# mutants out of efficacy, so many of them hide results. A few are expected,
# since a mutant that turns a loop infinite can only time out. A target with
# no mutants at all passes.
#
# usage: scripts/gremlins.sh <gremlins binary> <target> [gremlins flags...]
set -euo pipefail

MAX_TIMED_OUT_PERCENT=${MAX_TIMED_OUT_PERCENT:-2}

gremlins=$1 target=$2
shift 2

report="$(mktemp)"
trap 'rm -f "$report"' EXIT

code=0
"$gremlins" unleash --output "$report" "$@" "$target" || code=$?

if [[ ! -s "$report" ]]; then
  echo "gremlins: no report for $target" >&2
  exit "$(( code == 0 ? 1 : code ))"
fi

timed_out=$(jq '[.files[]?.mutations[]? | select(.status == "TIMED OUT")] | length' "$report")
all=$(jq '[.files[]?.mutations[]?] | length' "$report")
if (( timed_out * 100 > all * MAX_TIMED_OUT_PERCENT )); then
  echo "gremlins: $timed_out of $all mutants timed out in $target (limit ${MAX_TIMED_OUT_PERCENT}%); their results are unknown." >&2
  echo "Raise timeout-coefficient in .gremlins.yaml or speed up the tests." >&2
  exit 1
fi

if (( code != 0 )) && [[ "$(jq -r '.mutants_total' "$report")" == 0 ]]; then
  echo "No mutants in $target."
  exit 0
fi
exit "$code"
