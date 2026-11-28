#!/usr/bin/env bash
set -euo pipefail

base=${1:-}
head=${2:-}
default_mode() { echo 'modo=completo' >> "${GITHUB_OUTPUT:?}"; }

if [[ ! $base =~ ^[a-f0-9]{40}$ || ! $head =~ ^[a-f0-9]{40}$ ]] ||
   ! git merge-base --is-ancestor "$base" "$head"; then
  default_mode
  exit 0
fi

# Registros NUL preservam espaços, quebras de linha e outros bytes em nomes.
diff=$(mktemp)
trap 'rm -f -- "$diff"' EXIT
if ! git diff --no-renames --raw -z "$base" "$head" > "$diff" || [[ ! -s $diff ]]; then
  default_mode
  exit 0
fi

while IFS= read -r -d '' metadata && IFS= read -r -d '' path; do
  case "$path" in
    AGENTS.md|CLAUDE.md|README.md|CONTRIBUTING.md|aep/*.md|docs/content/*.md) ;;
    *) default_mode; exit 0 ;;
  esac
  read -r old_mode new_mode _ <<< "${metadata#:}"
  if [[ ! $old_mode =~ ^(000000|100644)$ || ! $new_mode =~ ^(000000|100644)$ ]]; then
    default_mode
    exit 0
  fi
done < "$diff"

# Markdown usa espaços no fim da linha para quebras de linha explícitas.
git -c core.whitespace=-blank-at-eol diff --check --no-renames "$base" "$head"
echo 'modo=documentacao' >> "${GITHUB_OUTPUT:?}"
