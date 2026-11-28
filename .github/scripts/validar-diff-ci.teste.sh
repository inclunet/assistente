#!/usr/bin/env bash
set -euo pipefail
script_dir=$(cd -- "$(dirname -- "$0")" && pwd)
test_root="$script_dir/../../build/bin/ci/diff-ci-test"
mkdir -p "$test_root"
repo=$(mktemp -d "$test_root/run.XXXXXX")
[[ $repo == "$test_root/"* ]] || exit 1
trap 'rm -rf -- "$repo"' EXIT
cd "$repo"
git init --quiet
export GIT_AUTHOR_NAME=teste GIT_AUTHOR_EMAIL=teste@exemplo
export GIT_COMMITTER_NAME=teste GIT_COMMITTER_EMAIL=teste@exemplo
export GITHUB_OUTPUT="$repo/result"
export GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=commit.gpgsign GIT_CONFIG_VALUE_0=false

commit() {
  git add -A -- . ':!result'
  git commit --quiet -m "$1"
}
expect() {
  : > "$GITHUB_OUTPUT"
  bash "$script_dir/validar-diff-ci.sh" "$1" "$(git rev-parse HEAD)"
  grep -Fxq "modo=$2" "$GITHUB_OUTPUT"
}

mkdir -p docs/content/guias frontend
printf '# Regras\n' > AGENTS.md
printf '# Guia\n' > docs/content/guias/guia.md
printf 'source\n' > frontend/arquivo.ts
touch result
commit base
base=$(git rev-parse HEAD)
expect "$base" completo

printf 'Atualização  \nContinuação\n' >> AGENTS.md
printf 'Detalhes\n' >> docs/content/guias/guia.md
commit docs
docs=$(git rev-parse HEAD)
expect "$base" documentacao
expect '' completo

git rm --quiet frontend/arquivo.ts
printf '# Novo guia\n' > 'docs/content/guias/novo guia.md'
commit misto
expect "$base" completo

git reset --quiet --hard "$docs"
printf '<<<<<<< HEAD\n' >> AGENTS.md
commit conflito
: > "$GITHUB_OUTPUT"
if bash "$script_dir/validar-diff-ci.sh" "$docs" "$(git rev-parse HEAD)"; then
  echo 'O caminho documental aceitou marcador de conflito.' >&2
  exit 1
fi
[[ ! -s $GITHUB_OUTPUT ]]

git reset --quiet --hard "$docs"
git update-index --chmod=+x AGENTS.md
git commit --quiet -m permissao
expect "$docs" completo

git reset --quiet --hard "$docs"
printf 'config\n' > docs/hugo.toml
commit config
expect "$docs" completo
echo 'PASS: diff documental, misto, vazio, sem base, quebra Markdown, conflito, modo de arquivo e configuração'
