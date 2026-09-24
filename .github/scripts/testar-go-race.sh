#!/usr/bin/env bash
# Cada teste top-level pertence a um grupo; seus subtestes continuam juntos.
set -euo pipefail
export LC_ALL=C

app=assistente/internal/app
flags=(-race -short -count=1 -timeout=20m)

if [[ $# == 1 && $1 == others ]]; then
  inventory=$(go list -race ./...)
  mapfile -t packages < <(printf '%s\n' "$inventory" | awk -v app="$app" 'NF && $0 != app')
  if [[ $(printf '%s\n' "$inventory" | grep -Fxc "$app") != 1 || ${#packages[@]} == 0 ]]; then
    echo 'Inventário de pacotes vazio ou sem App único' >&2
    exit 1
  fi
  exec go test "${flags[@]}" "${packages[@]}"
fi

if [[ $# != 2 || ! $1 =~ ^(0|[1-9][0-9]*)$ || ! $2 =~ ^[1-9][0-9]*$ || ${#1} -gt 3 || ${#2} -gt 3 ]]; then
  echo 'Uso: testar-go-race.sh others | indice total (0 <= indice < total <= 999)' >&2
  exit 1
fi
index=$1
total=$2
if (( index >= total )); then
  echo 'Índice fora da quantidade de grupos' >&2
  exit 1
fi

# Descoberta e execução usam os mesmos flags/build tags. Não roda testes aqui.
# O comando ainda compila/abre o binário: uso real somente no runner de CI.
inventory=$(go test "${flags[@]}" -list . ./internal/app)
mapfile -t names < <(printf '%s\n' "$inventory" |
  awk '/^(Test|Fuzz|Example)[^[:space:]]*$/ && $0 != "TestMain"' | sort -u)
if (( ${#names[@]} == 0 )); then
  echo 'Go não descobriu testes, exemplos ou fuzz targets de App' >&2
  exit 1
fi
selected=()
for position in "${!names[@]}"; do
  if (( position % total == index )); then
    selected+=("${names[position]}")
  fi
done
if (( ${#selected[@]} == 0 )); then
  echo 'Grupo vazio: reduza a quantidade de grupos' >&2
  exit 1
fi
printf 'App: grupo %s/%s, %s de %s testes top-level\n' "$index" "$total" "${#selected[@]}" "${#names[@]}"
printf '%s\n' "${selected[@]}"
# Escape de metacaracteres antes de montar a expressão RE2 ancorada.
pattern=$(printf '%s\n' "${selected[@]}" | sed 's/[][\.^$*+?(){}|]/\\&/g' | paste -sd '|' -)
exec go test "${flags[@]}" -run "^($pattern)$" ./internal/app
