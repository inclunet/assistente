#!/usr/bin/env bash
# Os cifrões abaixo são literais do workflow inspecionado.
# shellcheck disable=SC2016
set -euo pipefail

workflow=.github/workflows/release.yml
desktop='LDFLAGS="-X assistente/internal/app.AppVersion=$VERSION"'
cli='LDFLAGS="-X main.AppVersion=$VERSION"'

contar() {
  grep -Fc "$1" "$workflow" || true
}

desktop_count=$(contar "$desktop")
cli_count=$(contar "$cli")

if [ "$desktop_count" -ne 2 ]; then
  echo "Esperados 2 ldflags desktop para internal/app.AppVersion; encontrados: $desktop_count"
  exit 1
fi

if [ "$cli_count" -ne 1 ]; then
  echo "Esperado 1 ldflag da CLI para main.AppVersion; encontrados: $cli_count"
  exit 1
fi

if ! grep -Fq 'go build -ldflags "$LDFLAGS" -o "$OUTPUT" ./cmd/asst/' "$workflow"; then
  echo "O build da CLI deixou de consumir seu LDFLAGS."
  exit 1
fi

if [ "$(grep -Fc 'wails build -platform ${{ matrix.platform }} -ldflags "$LDFLAGS"' "$workflow")" -ne 3 ]; then
  echo "Os builds Wails deixaram de consumir o LDFLAGS desktop esperado."
  exit 1
fi

echo "ldflags de versão do release estão corretos"
