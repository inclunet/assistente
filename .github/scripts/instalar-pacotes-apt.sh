#!/usr/bin/env bash
#
# Instala pacotes de sistema no runner sem depender da boa vontade do espelho.
#
# O `apt-get update` do runner às vezes prende num espelho que aceita a conexão e
# nunca responde. Sem prazo próprio, o passo fica pendurado até o timeout do job
# (20~25 min) e mata o CI inteiro por um problema que nada tem a ver com o
# código. Aqui cada tentativa tem prazo, a falha é rápida e ainda há retentativa
# antes de desistir.

set -euo pipefail

if [ "$#" -eq 0 ]; then
  echo "uso: $0 <pacote> [pacote...]" >&2
  exit 2
fi

# DPkg::Lock::Timeout evita quebrar quando o unattended-upgrades do runner ainda
# segura o lock do dpkg.
opcoes_apt=(
  -o Acquire::Retries=3
  -o Acquire::http::Timeout=20
  -o Acquire::https::Timeout=20
  -o DPkg::Lock::Timeout=120
)

tentativas=${APT_TENTATIVAS:-3}
prazo=${APT_PRAZO_SEGUNDOS:-150}

executar_apt() {
  sudo timeout --signal=KILL "${prazo}" apt-get "${opcoes_apt[@]}" "$@"
}

for tentativa in $(seq 1 "${tentativas}"); do
  if executar_apt update -qq && executar_apt install -y -qq "$@"; then
    exit 0
  fi
  echo "::warning::apt falhou na tentativa ${tentativa}/${tentativas} para: $*"
  sleep $((tentativa * 10))
done

echo "::error::não foi possível instalar os pacotes: $*"
exit 1
