#!/usr/bin/env bash
# Cada teste top-level pertence a um grupo; seus subtestes continuam juntos.
set -euo pipefail
export LC_ALL=C

app=assistente/internal/app
chat=assistente/internal/chat
wailsapi=assistente/internal/wailsapi
flags=(-race -short -count=1 -timeout=20m)
groups=(pacote-wailsapi pacote-chat-0 pacote-chat-1 comandos-configuracao comandos-dispositivos
  comandos-contexto-deck-paginas comandos-contexto-deck-mermaid comandos-contexto-deck-camadas comandos-contexto-deck-base
  comandos-contexto-paleta comandos-contexto-workspace comandos-contexto-base comandos-execucao
  comandos-jobs comandos-interface comandos-seguranca comandos-outros app-chat app-demais)

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

package=./internal/app
verbosity=()
if [[ $1 == pacote-chat-* ]]; then
  package=./internal/chat
  verbosity=(-v)
fi

# Regras em ordem de precedência. O residual inclui automaticamente nomes novos;
# não há lista manual de testes a manter nem casos descartados por renomeação.
classify() {
  if [[ $package == ./internal/chat ]]; then
    # Hash estável do nome: novos testes não redistribuem os existentes.
    # A descoberta inclui automaticamente todos os testes, exemplos e fuzz seeds.
    local checksum
    checksum=$(printf '%s' "$1" | cksum)
    printf 'pacote-chat-%s\n' "$(( ${checksum%% *} % 2 ))"
    return
  fi
  case "$1" in
    TestCommandSettings*|TestCommandConfig*|TestCommandLayer*|TestCommandBuiltin*|TestCommandImport*|TestCommandExport*|TestCommandReset*) echo comandos-configuracao ;;
    TestCommandDeck*|TestCommandKeyboard*|TestCommandGlobal*|TestCommandHotkey*|TestCommandHost*|TestCommandPhysical*|TestCommandStreamDeck*) echo comandos-dispositivos ;;
    # Contexto excedeu o orçamento acumulado mesmo sem teste individual preso.
    # Famílias específicas primeiro; o residual preserva nomes futuros.
    TestContextualDeckPage*) echo comandos-contexto-deck-paginas ;;
    TestContextualDeckMermaid*) echo comandos-contexto-deck-mermaid ;;
    TestContextualDeckLayer*) echo comandos-contexto-deck-camadas ;;
    TestContextualDeck*) echo comandos-contexto-deck-base ;;
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
inventory=$(go test "${flags[@]}" -list . "$package")
mapfile -t names < <(printf '%s\n' "$inventory" |
  awk '/^(Test|Fuzz|Example)[^[:space:]]*$/ && $0 != "TestMain"' | sort -u)
if (( ${#names[@]} == 0 )); then
  echo "Go não descobriu testes, exemplos ou fuzz targets de $package" >&2
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
printf '%s: grupo %s, %s de %s testes top-level\n' "$package" "$1" "${#selected[@]}" "${#names[@]}"
printf '%s\n' "${selected[@]}"
# Escape de metacaracteres antes de montar a expressão RE2 ancorada.
pattern=$(printf '%s\n' "${selected[@]}" | sed 's/[][\.^$*+?(){}|]/\\&/g' | paste -sd '|' -)
exec go test "${flags[@]}" "${verbosity[@]}" -run "^($pattern)$" "$package"
