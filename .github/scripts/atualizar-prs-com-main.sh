#!/usr/bin/env bash
#
# Desce a main nos PRs abertos e pede o CI de cada um.
#
# Por que empurrar em vez de só avisar: o veredito que interessa é o do encontro
# entre a branch e a main, e esse encontro só existe como commit. Um aviso de
# "está atrasado" mandaria a pessoa fazer à mão exatamente isto.
#
# Por que pedir o CI explicitamente: push feito com o GITHUB_TOKEN não dispara
# workflow (é a trava do GitHub contra recursão). A exceção documentada é o
# workflow_dispatch, então é por ele que o CI é chamado.

set -euo pipefail

git config user.name 'github-actions[bot]'
git config user.email '41898282+github-actions[bot]@users.noreply.github.com'

git fetch origin main --quiet

atualizados=()
conflitados=()
falhados=()

# Draft fica de fora: é trabalho em curso, e commit de merge chegando por baixo
# atrapalha quem ainda está reescrevendo a branch. PR de fork também: o token
# desta execução não escreve no repositório de outra pessoa. E PR parado há mais
# de um mês fica quieto: descer a main nele não aproxima nenhum merge, só pede
# review de novo e enche a lista de notificação.
parado_em=$(( 30 * 24 * 60 * 60 ))
prs=$(gh pr list --state open --limit 100 \
  --json number,headRefName,isDraft,isCrossRepository,updatedAt \
  --jq ".[] | select(.isDraft == false and .isCrossRepository == false) \
    | select((.updatedAt | fromdateiso8601) > (now - $parado_em)) \
    | [.number, .headRefName] | @tsv")

if [ -z "$prs" ]; then
  echo 'Nenhum PR aberto para atualizar.'
  exit 0
fi

while IFS=$'\t' read -r numero branch; do
  [ -n "$numero" ] || continue

  git fetch origin "$branch" --quiet

  if git merge-base --is-ancestor origin/main "origin/$branch"; then
    echo "PR #$numero ($branch): já tem a main."
    continue
  fi

  git checkout --quiet -B "atualizar/$branch" "origin/$branch"

  if ! git merge --no-edit origin/main; then
    git merge --abort || true
    echo "::warning::PR #$numero ($branch) conflita com a main; resolva à mão."
    conflitados+=("#$numero ($branch)")
    continue
  fi

  if ! git push origin "HEAD:refs/heads/$branch"; then
    echo "::warning::PR #$numero ($branch): não consegui empurrar a atualização."
    falhados+=("#$numero ($branch)")
    continue
  fi

  gh workflow run ci.yml --ref "$branch"
  echo "PR #$numero ($branch): main descida e CI pedido."
  atualizados+=("#$numero ($branch)")
done <<< "$prs"

{
  echo '## PRs abertos diante da main'
  echo ''
  lista() {
    if [ "$#" -eq 0 ]; then
      echo '- nenhum'
    else
      printf -- '- %s\n' "$@"
    fi
  }
  echo '### Atualizados (CI pedido)'
  lista "${atualizados[@]+"${atualizados[@]}"}"
  echo ''
  echo '### Em conflito (precisam de mão)'
  lista "${conflitados[@]+"${conflitados[@]}"}"
  echo ''
  echo '### Falha ao empurrar'
  lista "${falhados[@]+"${falhados[@]}"}"
} >> "$GITHUB_STEP_SUMMARY"

# Conflito não derruba o run: ele é notícia sobre o PR, não defeito deste
# workflow. Falha de push é outra história — aí algo aqui não funcionou.
if [ "${#falhados[@]}" -gt 0 ]; then
  exit 1
fi
