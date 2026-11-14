#!/usr/bin/env bash
# Publishes a measurement run's rows to main.
#
# The run commits its rows on top of the repository as it was when it
# started, and then main may have moved under it: another measurement job
# reached main first and appended rows after the same base last line.
# Append-only files cannot be reordered by a plain rebase, so the rebase
# used to fail the whole job and the row was lost — the measurement had
# happened, the series simply never learned about it.
#
# The series files are append-only: keeping every row from both sides is
# the only correct answer. Conflict markers are dropped and exact
# duplicates are not written twice. Anything else that conflicts — a
# board card two runs both created, an edit nobody expected in an append
# -only file — is not the series' business, and the run stops loudly
# instead of writing a file nobody can read.
#
#   publish-series.sh <commit-message>
set -euo pipefail
commit_message="${1:?commit message}"

git config user.name "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"

# The board is a directory of cards, and an empty board is not in git at
# all — so the paths are added wholesale rather than by name. Only these
# paths: `git add -A` would sweep in whatever the run touched, and the
# run touches go.sum — `go mod download all` writes the sums of modules
# the build does not need.
git add measurements .project-board
if git diff --cached --quiet; then
  echo "the series has nothing new"
  exit 0
fi
git commit -m "$commit_message"

# The run touched other things, and a rebase refuses to run with a dirty
# tree. The commit is already made; what is left is the run's own dust.
git checkout -- .

git fetch origin main
if ! git rebase origin/main; then
  unmerged=$(git diff --name-only --diff-filter=U)
  for file in $unmerged; do
    case "$file" in
      measurements/*.md)
        awk '
          /^<<<<<<< / { next }
          /^=======$/ { next }
          /^>>>>>>> / { next }
          { if (!seen[$0]++) print }
        ' "$file" > "$file.series-union" && mv "$file.series-union" "$file"
        echo "unioned the append-only series: $file"
        ;;
      *)
        echo "cannot union-resolve $file; aborting the rebase"
        git rebase --abort
        exit 1
        ;;
    esac
  done
  # Nothing that looks like a marker may survive the union; a marker left
  # in the series is a file nobody can read, and pushing it would be worse
  # than the lost row.
  if grep -nE '^(<<<<<<<|>>>>>>>|=======$)' measurements/*.md 2>/dev/null; then
    echo "conflict markers remain after the union; aborting the rebase"
    git rebase --abort
    exit 1
  fi
  git add measurements
  GIT_EDITOR=true git rebase --continue
fi
git push origin HEAD:main
