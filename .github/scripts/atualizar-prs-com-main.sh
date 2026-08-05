#!/usr/bin/env bash
#
# Desce a main (e, no PR empilhado, a branch-base) em cada PR aberto e pede o CI.
#
# Por que empurrar em vez de só avisar: o veredito que interessa é o do encontro
# entre a branch e o que veio depois dela, e esse encontro só existe como commit.
# Um aviso de "está atrasado" mandaria a pessoa fazer à mão exatamente isto.
#
# Por que a base entra junto: o CI pedido aqui roda no topo da branch, não no
# merge efêmero que o evento pull_request monta. Num PR empilhado, topo sem a
# base é meia verdade — verde ali não diz nada sobre a integração com o pai.
# Descendo a base, o topo passa a ser o mesmo conteúdo que o merge testaria.
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
sem_ci=()

# Draft fica de fora: é trabalho em curso, e commit de merge chegando por baixo
# atrapalha quem ainda está reescrevendo a branch. PR de fork também: o token
# desta execução não escreve no repositório de outra pessoa. E PR parado há mais
# de um mês fica quieto: descer a main nele não aproxima nenhum merge, só pede
# review de novo e enche a lista de notificação.
parado_em=$(( 30 * 24 * 60 * 60 ))
prs=$(gh pr list --state open --limit 100 \
  --json number,headRefName,baseRefName,isDraft,isCrossRepository,updatedAt \
  --jq ".[] | select(.isDraft == false and .isCrossRepository == false) \
    | select((.updatedAt | fromdateiso8601) > (now - $parado_em)) \
    | [.number, .headRefName, .baseRefName] | @tsv")

if [ -z "$prs" ]; then
  echo 'Nenhum PR aberto para atualizar.'
  exit 0
fi

declare -A numero_de=()
declare -A base_de=()
arestas=''

while IFS=$'\t' read -r numero branch base; do
  [ -n "$numero" ] || continue
  numero_de["$branch"]=$numero
  base_de["$branch"]=$base
  arestas+="$base $branch"$'\n'
done <<< "$prs"

# Pai antes de filho: atualizar o pai muda o que o filho tem de receber, e a
# ordem que o gh devolve não conhece a pilha. tsort ordena pelas arestas
# base→branch; nós que não são PR (a main, por exemplo) saem junto e são pulados.
ordem=$(printf '%s' "$arestas" | tsort)

while read -r branch; do
  [ -n "$branch" ] || continue
  numero=${numero_de["$branch"]:-}
  [ -n "$numero" ] || continue

  base=${base_de["$branch"]}
  git fetch origin "$branch" --quiet
  alvos=('origin/main')
  if [ "$base" != 'main' ]; then
    git fetch origin "$base" --quiet
    alvos+=("origin/$base")
  fi

  faltando=()
  for alvo in "${alvos[@]}"; do
    if ! git merge-base --is-ancestor "$alvo" "origin/$branch"; then
      faltando+=("$alvo")
    fi
  done

  if [ "${#faltando[@]}" -eq 0 ]; then
    echo "PR #$numero ($branch): já tem tudo o que precisava."
    continue
  fi

  git checkout --quiet -B "atualizar/$branch" "origin/$branch"

  conflitou=''
  for alvo in "${faltando[@]}"; do
    if ! git merge --no-edit "$alvo"; then
      git merge --abort || true
      echo "::warning::PR #$numero ($branch) conflita com $alvo; resolva à mão."
      conflitados+=("#$numero ($branch) × $alvo")
      conflitou='sim'
      break
    fi
  done
  [ -z "$conflitou" ] || continue

  if ! git push origin "HEAD:refs/heads/$branch"; then
    echo "::warning::PR #$numero ($branch): não consegui empurrar a atualização."
    falhados+=("#$numero ($branch)")
    continue
  fi

  # O push já valeu; se o pedido de CI falhar, o lote segue. Parar aqui deixaria
  # os outros PRs sem atualização e ainda comeria o resumo do run.
  if ! gh workflow run ci.yml --ref "$branch"; then
    echo "::warning::PR #$numero ($branch): atualizado, mas o CI não foi pedido."
    sem_ci+=("#$numero ($branch)")
    continue
  fi

  echo "PR #$numero ($branch): atualizado e CI pedido."
  atualizados+=("#$numero ($branch)")
done <<< "$ordem"

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
  echo '### Atualizados sem CI pedido'
  lista "${sem_ci[@]+"${sem_ci[@]}"}"
  echo ''
  echo '### Falha ao empurrar'
  lista "${falhados[@]+"${falhados[@]}"}"
} >> "$GITHUB_STEP_SUMMARY"

# Conflito não derruba o run: ele é notícia sobre o PR, não defeito deste
# workflow. Push que não foi e CI que não foi pedido são outra história — aí
# algo aqui não funcionou, e o silêncio deixaria o PR verde por engano.
if [ "${#falhados[@]}" -gt 0 ] || [ "${#sem_ci[@]}" -gt 0 ]; then
  exit 1
fi
