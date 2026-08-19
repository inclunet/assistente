#!/usr/bin/env bash
#
# Instala pacotes de sistema no runner sem depender da boa vontade do espelho.
#
# O espelho padrão do runner (azure.archive.ubuntu.com) às vezes aceita a conexão
# e nunca responde. Sem prazo próprio, o passo fica pendurado até o timeout do
# job (20~25 min) e mata o CI inteiro por um problema que nada tem a ver com o
# código. Aqui o índice tem prazo curto, a falha é rápida e, antes de desistir, o
# apt é apontado para o archive canônico — troca que vale para todo apt que rodar
# depois no mesmo job, inclusive o que o Playwright dispara por conta própria.
#
# Sem pacotes na linha de comando, só atualiza o índice: serve para preparar o
# apt (e corrigir o espelho) antes de entregá-lo a outra ferramenta.

set -euo pipefail

# DPkg::Lock::Timeout evita quebrar quando o unattended-upgrades do runner ainda
# segura o lock do dpkg.
opcoes_apt=(
  -o Acquire::Retries=3
  -o Acquire::http::Timeout=20
  -o Acquire::https::Timeout=20
  -o DPkg::Lock::Timeout=120
)

tentativas=${APT_TENTATIVAS:-3}
prazo_indice=${APT_PRAZO_INDICE:-120}
prazo_instalacao=${APT_PRAZO_INSTALACAO:-900}
espelho_trocado=0

# Só o update leva SIGKILL: ele é o passo que trava no espelho e não deixa nada
# pela metade. Interromper uma instalação à força arriscaria o estado do dpkg,
# então ali o prazo é largo e o sinal é o padrão.
atualizar_indice() {
  sudo timeout --signal=KILL "${prazo_indice}" apt-get "${opcoes_apt[@]}" update -qq
}

instalar_pacotes() {
  sudo timeout "${prazo_instalacao}" apt-get "${opcoes_apt[@]}" install -y -qq "$@"
}

apontar_para_archive() {
  local arquivo
  for arquivo in /etc/apt/sources.list /etc/apt/sources.list.d/*.sources /etc/apt/sources.list.d/*.list; do
    [ -f "${arquivo}" ] || continue
    sudo sed -i -E 's#://[a-z0-9.-]*archive\.ubuntu\.com#://archive.ubuntu.com#g' "${arquivo}"
  done
}

alvo="$*"
[ -n "${alvo}" ] || alvo="atualização do índice"

for tentativa in $(seq 1 "${tentativas}"); do
  if atualizar_indice && { [ "$#" -eq 0 ] || instalar_pacotes "$@"; }; then
    exit 0
  fi
  echo "::warning::apt falhou na tentativa ${tentativa}/${tentativas}: ${alvo}"
  if [ "${espelho_trocado}" -eq 0 ]; then
    echo "apontando o apt para archive.ubuntu.com"
    apontar_para_archive
    espelho_trocado=1
  fi
  sleep $((tentativa * 10))
done

echo "::error::não foi possível concluir o apt: ${alvo}"
exit 1
