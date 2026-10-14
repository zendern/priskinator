#!/usr/bin/env bash
# Rejects commit messages whose first line is not a Conventional Commit.
set -euo pipefail

first_line=$(head -n1 "$1")

case "$first_line" in
  Merge\ *|fixup!\ *|squash!\ *|amend!\ *) exit 0 ;;
esac

pattern='^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)(\([a-z0-9._/-]+\))?!?: [^A-Z].*$'
if ! [[ "$first_line" =~ $pattern ]]; then
  echo "commit message must follow Conventional Commits, e.g. 'feat(policy): add layering'" >&2
  echo "got: $first_line" >&2
  exit 1
fi
if (( ${#first_line} > 72 )); then
  echo "first line is ${#first_line} characters; keep it to 72 or fewer" >&2
  exit 1
fi
