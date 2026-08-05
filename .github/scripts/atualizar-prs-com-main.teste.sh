#!/usr/bin/env bash
# Banco de provas do atualizar-prs-com-main.sh: monta um repositório de mentira
# com uma pilha (pai ← filho), um PR que conflita e um PR já em dia, e roda o
# script contra um `gh` de brinquedo.
set -euo pipefail

raiz=$(mktemp -d)
script=$raiz/atualizar-prs-com-main.sh
cp "$(dirname "$0")/atualizar-prs-com-main.sh" "$script"
chmod +x "$script"

export GIT_AUTHOR_NAME=teste GIT_AUTHOR_EMAIL=teste@exemplo
export GIT_COMMITTER_NAME=teste GIT_COMMITTER_EMAIL=teste@exemplo

git init --quiet --bare "$raiz/origin.git"
git clone --quiet "$raiz/origin.git" "$raiz/trabalho"
cd "$raiz/trabalho"
git checkout --quiet -b main
echo 'contrato v1' > contrato.txt
echo 'comum' > comum.txt
git add .
git commit --quiet -m 'inicio'
git push --quiet -u origin main

git checkout --quiet -b pai
echo 'pai' > pai.txt
git add .
git commit --quiet -m 'pai'
git push --quiet -u origin pai

git checkout --quiet -b filho
echo 'filho' > filho.txt
git add .
git commit --quiet -m 'filho'
git push --quiet -u origin filho

git checkout --quiet -b conflitante main
echo 'versao da branch' > comum.txt
git commit --quiet -am 'toca o arquivo comum'
git push --quiet -u origin conflitante

git checkout --quiet -b em-dia main
echo 'em dia' > em-dia.txt
git add .
git commit --quiet -m 'em dia'
git push --quiet -u origin em-dia

# Órfã: PR cuja base foi apagada do remoto, o que acontece toda vez que o PR pai
# é mergeado e a branch dele some antes do GitHub reapontar o filho.
git checkout --quiet -b orfa main
echo 'orfa' > orfa.txt
git add .
git commit --quiet -m 'orfa'
git push --quiet -u origin orfa

git checkout --quiet main
echo 'contrato v2' > contrato.txt
echo 'versao da main' > comum.txt
git commit --quiet -am 'main anda'
git push --quiet origin main

git checkout --quiet em-dia
git merge --quiet --no-edit main
git push --quiet origin em-dia

git checkout --quiet main

# gh de brinquedo: responde a lista de PRs e anota cada CI pedido.
mkdir -p "$raiz/bin"
cat > "$raiz/bin/gh" <<'FIM'
#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  pr)
    cat "$RAIZ_TESTE/prs.tsv"
    ;;
  workflow)
    prev=''
    for arg in "$@"; do
      if [ "$prev" = '--ref' ]; then
        echo "$arg" >> "$RAIZ_TESTE/ci-pedido.txt"
      fi
      prev=$arg
    done
    ;;
esac
FIM
chmod +x "$raiz/bin/gh"

printf '%s\n' \
  $'11\tfilho\tpai' \
  $'14\torfa\tbase-que-sumiu' \
  $'10\tpai\tmain' \
  $'12\tconflitante\tmain' \
  $'13\tem-dia\tmain' > "$raiz/prs.tsv"

export RAIZ_TESTE=$raiz
export PATH=$raiz/bin:$PATH
export GITHUB_STEP_SUMMARY=$raiz/resumo.md
: > "$GITHUB_STEP_SUMMARY"
: > "$raiz/ci-pedido.txt"

saida=0
"$script" > "$raiz/log.txt" 2>&1 || saida=$?

echo "=== log ==="
cat "$raiz/log.txt"
echo "=== resumo ==="
cat "$GITHUB_STEP_SUMMARY"
echo "=== CI pedido ==="
cat "$raiz/ci-pedido.txt"
echo "=== saída: $saida ==="

falhas=0
conferir() {
  local descricao=$1 esperado=$2 obtido=$3
  if [ "$esperado" = "$obtido" ]; then
    echo "ok: $descricao"
  else
    echo "FALHA: $descricao (esperado '$esperado', obtido '$obtido')"
    falhas=$((falhas + 1))
  fi
}

git -C "$raiz/trabalho" fetch --quiet origin

pai_tem_main=nao
git -C "$raiz/trabalho" merge-base --is-ancestor origin/main origin/pai && pai_tem_main=sim
conferir 'pai recebeu a main' sim "$pai_tem_main"

filho_tem_pai=nao
git -C "$raiz/trabalho" merge-base --is-ancestor origin/pai origin/filho && filho_tem_pai=sim
conferir 'filho recebeu o pai já atualizado' sim "$filho_tem_pai"

filho_tem_main=nao
git -C "$raiz/trabalho" merge-base --is-ancestor origin/main origin/filho && filho_tem_main=sim
conferir 'filho recebeu a main' sim "$filho_tem_main"

conflitante_intacta=sim
git -C "$raiz/trabalho" merge-base --is-ancestor origin/main origin/conflitante && conflitante_intacta=nao
conferir 'branch em conflito ficou como estava' sim "$conflitante_intacta"

orfa_tem_main=nao
git -C "$raiz/trabalho" merge-base --is-ancestor origin/main origin/orfa && orfa_tem_main=sim
conferir 'PR com base apagada recebeu a main assim mesmo' sim "$orfa_tem_main"

conferir 'CI pedido para quem mudou, não para quem já estava em dia' \
  'filho orfa pai' "$(sort "$raiz/ci-pedido.txt" | tr '\n' ' ' | sed 's/ *$//')"

conferir 'conflito é notícia, não falha do run' 0 "$saida"

grep -q 'conflitante' "$GITHUB_STEP_SUMMARY" && encontrou=sim || encontrou=nao
conferir 'resumo lista o conflito' sim "$encontrou"

grep -q 'base-que-sumiu' "$GITHUB_STEP_SUMMARY" && encontrou=sim || encontrou=nao
conferir 'resumo diz que a base sumiu' sim "$encontrou"

rm -rf "$raiz"
if [ "$falhas" -gt 0 ]; then
  echo "$falhas verificação(ões) falharam"
  exit 1
fi
echo 'tudo certo'
