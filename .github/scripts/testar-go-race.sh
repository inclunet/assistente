#!/usr/bin/env bash
# Cada teste top-level pertence a um grupo; seus subtestes continuam juntos.
set -euo pipefail
export LC_ALL=C

app=assistente/internal/app
flags=(-race -short -count=1 -timeout=20m)
groups=(comandos-configuracao comandos-dispositivos
  comandos-contexto-deck comandos-contexto-paleta comandos-contexto-workspace comandos-contexto-base comandos-execucao
  comandos-jobs comandos-interface comandos-seguranca comandos-outros app-chat app-demais)

if [[ $# == 1 && $1 == --groups ]]; then
  printf '%s\n' pacotes-gerais "${groups[@]}"
  exit 0
fi

if [[ $# == 1 && $1 == pacotes-gerais ]]; then
  inventory=$(go list -race ./...)
  mapfile -t packages < <(printf '%s\n' "$inventory" | awk -v app="$app" 'NF && $0 != app')
  if [[ $(printf '%s\n' "$inventory" | grep -Fxc "$app") != 1 || ${#packages[@]} == 0 ]]; then
    echo 'Inventário de pacotes vazio ou sem App único' >&2
    exit 1
  fi
  exec go test "${flags[@]}" "${packages[@]}"
fi

if [[ $# != 1 || " ${groups[*]} " != *" $1 "* ]]; then
  echo 'Informe um grupo válido (consulte --groups)' >&2
  exit 1
fi

# Regras em ordem de precedência. O residual inclui automaticamente nomes novos;
# não há lista manual de testes a manter nem casos descartados por renomeação.
classify() {
  case "$1" in
    TestCommandSettings*|TestCommandConfig*|TestCommandLayer*|TestCommandBuiltin*|TestCommandImport*|TestCommandExport*|TestCommandReset*) echo comandos-configuracao ;;
    TestCommandDeck*|TestCommandKeyboard*|TestCommandGlobal*|TestCommandHotkey*|TestCommandHost*|TestCommandPhysical*|TestCommandStreamDeck*) echo comandos-dispositivos ;;
    # Contexto excedeu o orçamento acumulado mesmo sem teste individual preso.
    # Famílias específicas primeiro; o residual preserva nomes futuros.
    TestContextualDeck*) echo comandos-contexto-deck ;;
    TestContextualPalette*|TestContextualPagePalette*|TestContextualLayerPalette*) echo comandos-contexto-paleta ;;
    TestCommandWorkspace*) echo comandos-contexto-workspace ;;
    TestContextual*|TestCommandContext*|TestCommandProfile*|TestCommandScope*) echo comandos-contexto-base ;;
    TestCommandProduct*|TestCommandExecution*|TestCommandExecute*|TestCommandInvocation*|TestCommandEnvelope*|TestCommandReplay*|TestCommandReceipt*|TestCommandDecision*|TestCommandExternal*|TestCommandPublic*|TestCommandBridge*|TestCommandTool*|TestCommandRuntime*) echo comandos-execucao ;;
    TestCommandJob*|TestCommandMaintenance*|TestCommandTasklist*|TestCommandTerminal*|TestCommandSource*|TestCommandEvent*|TestTasklistService*) echo comandos-jobs ;;
    TestCommandChat*|TestCommandEditor*|TestCommandPage*|TestCommandLocal*|TestCommandFrontend*|TestCommandNavigation*|TestCommandFocus*|TestCommandTab*|TestCommandUI*|TestCommandPalette*|TestCommandSurface*) echo comandos-interface ;;
    TestCommandAuth*|TestCommandSecurity*|TestCommandSession*|TestCommandCredential*|TestCommandVault*|TestCommandPrincipal*|TestCommandLifecycle*|TestCommandBootstrap*|TestCommandRebuild*|TestCommandStorage*|TestCommandOS*|TestAppCommand*) echo comandos-seguranca ;;
    TestCommand*) echo comandos-outros ;;
    TestChat*|TestConversa*|TestSend*|TestMessage*|TestCompact*|TestSpeak*|TestVoice*|TestAudio*|TestTTS*|TestSTT*) echo app-chat ;;
    *) echo app-demais ;;
  esac
}

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
for name in "${names[@]}"; do
  if [[ $(classify "$name") == "$1" ]]; then
    selected+=("$name")
  fi
done
if (( ${#selected[@]} == 0 )); then
  echo 'Grupo vazio: confira a classificação e a matriz do CI' >&2
  exit 1
fi
printf 'App: grupo %s, %s de %s testes top-level\n' "$1" "${#selected[@]}" "${#names[@]}"
printf '%s\n' "${selected[@]}"
# Escape de metacaracteres antes de montar a expressão RE2 ancorada.
pattern=$(printf '%s\n' "${selected[@]}" | sed 's/[][\.^$*+?(){}|]/\\&/g' | paste -sd '|' -)
exec go test "${flags[@]}" -run "^($pattern)$" ./internal/app
