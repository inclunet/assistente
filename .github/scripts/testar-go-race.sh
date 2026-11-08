#!/usr/bin/env bash
# Cada teste top-level pertence a um grupo; seus subtestes continuam juntos.
set -euo pipefail
export LC_ALL=C

app=assistente/internal/app
chat=assistente/internal/chat
wailsapi=assistente/internal/wailsapi
flags=(-race -short -count=1 -timeout=20m)
groups=(pacote-wailsapi pacote-chat-0 pacote-chat-1 pacote-app-0 pacote-app-1 pacote-app-2 pacote-app-3)
app_binary="$(pwd)/build/bin/ci/app-race.test"

# Compilação única por execução do workflow; não executa testes nem TestMain.
if [[ $# == 1 && $1 == --build-app ]]; then
  mkdir -p "$(dirname -- "$app_binary")"
  exec go test -race -c -o "$app_binary" ./internal/app
fi

if [[ $# == 1 && $1 == --groups ]]; then
  printf '%s\n' pacotes-gerais "${groups[@]}"
  exit 0
fi

if [[ $# == 1 && $1 == pacotes-gerais ]]; then
  inventory=$(go list -race ./...)
  mapfile -t packages < <(printf '%s\n' "$inventory" | awk -v app="$app" -v chat="$chat" -v wailsapi="$wailsapi" 'NF && $0 != app && $0 != chat && $0 != wailsapi')
  if [[ $(printf '%s\n' "$inventory" | grep -Fxc "$app") != 1 || $(printf '%s\n' "$inventory" | grep -Fxc "$chat") != 1 || $(printf '%s\n' "$inventory" | grep -Fxc "$wailsapi") != 1 || ${#packages[@]} == 0 ]]; then
    echo 'Inventário de pacotes vazio ou sem App/Chat/WailsAPI únicos' >&2
    exit 1
  fi
  exec go test "${flags[@]}" -json "${packages[@]}"
fi

if [[ $# != 1 || " ${groups[*]} " != *" $1 "* ]]; then
  echo 'Informe um grupo válido (consulte --groups)' >&2
  exit 1
fi

# O pacote inteiro tem runner próprio; novos testes entram automaticamente.
# Evita que o tempo acumulado dos outros pacotes o corte no fim do job.
if [[ $1 == pacote-wailsapi ]]; then
  exec go test "${flags[@]}" -json ./internal/wailsapi
fi

package=./internal/chat
shards=2
if [[ $1 == pacote-app-* ]]; then
  package=./internal/app
  shards=4
  [[ -x $app_binary ]] || { echo 'Binário race do App ausente ou sem permissão de execução' >&2; exit 1; }
  # go test executa a partir do diretório do pacote; preservar fixtures relativas.
  cd -- "$package"
  inventory=$("$app_binary" -test.short -test.list .)
else
  inventory=$(go test "${flags[@]}" -list . "$package")
fi

# Hash estável: novos testes não redistribuem os existentes. A descoberta
# inclui automaticamente testes, exemplos e fuzz seeds, sem famílias manuais.
classify() {
  local checksum
  checksum=$(printf '%s' "$1" | cksum)
  printf '%s\n' "$(( ${checksum%% *} % shards ))"
}

mapfile -t names < <(printf '%s\n' "$inventory" |
  awk '/^(Test|Fuzz|Example)[^[:space:]]*$/ && $0 != "TestMain"' | sort -u)
if (( ${#names[@]} == 0 )); then
  echo "Go não descobriu testes, exemplos ou fuzz targets de $package" >&2
  exit 1
fi
selected=()
for name in "${names[@]}"; do
  if [[ $(classify "$name") == "${1##*-}" ]]; then
    selected+=("$name")
  fi
done
if (( ${#selected[@]} == 0 )); then
  echo 'Grupo vazio: confira a classificação e a matriz do CI' >&2
  exit 1
fi
printf '%s: grupo %s, %s de %s testes top-level\n' "$package" "$1" "${#selected[@]}" "${#names[@]}"
printf '%s\n' "${selected[@]}"
# Escape de metacaracteres antes de montar a expressão RE2 ancorada.
pattern=$(printf '%s\n' "${selected[@]}" | sed 's/[][\.^$*+?(){}|]/\\&/g' | paste -sd '|' -)
if [[ $1 == pacote-app-* ]]; then
  exec go tool test2json -t -p "$app" "$app_binary" -test.v=test2json -test.short -test.count=1 -test.timeout=20m -test.run "^($pattern)$"
fi
exec go test "${flags[@]}" -v -run "^($pattern)$" "$package"
