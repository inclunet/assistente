#!/usr/bin/env bash
# Nunca executa Go real: o PATH aponta para um mock privado deste teste.
set -euo pipefail
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_dir=$(cd -- "$script_dir/../.." && pwd)
test_dir=$(mktemp -d)
trap 'rm -rf -- "$test_dir"' EXIT
export RACE_MOCK_DIR="$test_dir"
export PATH="$test_dir:$PATH"
cat > "$test_dir/go" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$RACE_MOCK_DIR/calls"
if [[ $1 == list ]]; then
  [[ ${MOCK_FAIL:-} != packages ]] || exit 17
  cat "$RACE_MOCK_DIR/packages"
elif [[ " $* " == *' -list '* ]]; then
  [[ ${MOCK_FAIL:-} != discovery ]] || exit 18
  cat "$RACE_MOCK_DIR/names"
else
  [[ ${MOCK_FAIL:-} != execution ]] || exit 19
  printf '%s\n' "$@" > "$RACE_MOCK_DIR/args"
fi
MOCK
chmod +x "$test_dir/go"

printf '%s\n' assistente/internal/app assistente/internal/app/child assistente/internal/auth > "$test_dir/packages"

add_fixture() {
  local group=$1
  shift
  local name
  for name in "$@"; do
    printf '%s\t%s\n' "$group" "$name" >> "$test_dir/expected-map"
    printf '%s\n' "$name" >> "$test_dir/expected"
  done
}

: > "$test_dir/expected-map"
: > "$test_dir/expected"
# Mantém representatividade das famílias preexistentes do fixture.
add_fixture comandos-configuracao TestCommandSettingsA
add_fixture comandos-dispositivos TestCommandDeckA
add_fixture comandos-contexto-base TestContextualA
add_fixture comandos-execucao TestCommandExecutionA
add_fixture comandos-jobs TestCommandJobA
add_fixture comandos-interface TestCommandEditorA
add_fixture comandos-seguranca TestCommandAuthA
add_fixture comandos-outros TestCommandFutureA
add_fixture app-chat TestChatA
add_fixture app-demais TestA FuzzInput Example ExampleOutput TestAção

# Famílias específicas devem vencer os padrões residuais mais amplos.
add_fixture comandos-contexto-deck-paginas TestContextualDeckPageA TestContextualDeckPageFuture
add_fixture comandos-contexto-deck-mermaid TestContextualDeckMermaidA TestContextualDeckMermaidFuture
add_fixture comandos-contexto-deck-camadas TestContextualDeckLayerA TestContextualDeckLayerFuture
add_fixture comandos-contexto-deck-base TestContextualDeckA TestContextualDeckFutureBase
add_fixture comandos-contexto-paleta TestContextualPaletteA TestContextualPagePaletteA TestContextualLayerPaletteA
add_fixture comandos-contexto-workspace TestCommandWorkspaceA
add_fixture comandos-contexto-base \
  TestCommandContextA TestCommandProfileA TestCommandScopeA \
  TestContextualFutureBase TestCommandContextFutureBase TestCommandProfileFutureBase TestCommandScopeFutureBase

{
  cat "$test_dir/expected"
  printf '%s\n' BenchmarkSpeed TestMain 'ok assistente/internal/app 0.001s'
} > "$test_dir/names"

run() { bash "$script_dir/testar-go-race.sh" "$@" > "$test_dir/output" 2>&1; }
reject() { if run "$@"; then echo "Deveria falhar: $*" >&2; exit 1; fi; }
fail() { echo "$*" >&2; exit 1; }
assert_unique() {
  local label=$1 file=$2 duplicates
  duplicates=$(LC_ALL=C sort "$file" | uniq -d)
  [[ -z $duplicates ]] || fail "$label contém duplicatas: $duplicates"
}
assert_flags() {
  local expected_command=$1
  mapfile -t got < "$test_dir/args"
  [[ ${got[*]:0:5} == "$expected_command" ]] || fail "flags Go alteradas: ${got[*]}"
  [[ ${got[1]} == -race && ${got[2]} == -short && ${got[3]} == -count=1 && ${got[4]} == -timeout=20m ]] ||
    fail "flags -race/-short/-count/-timeout não preservadas: ${got[*]}"
}

expected_groups=(
  pacotes-gerais comandos-configuracao comandos-dispositivos
  comandos-contexto-deck-paginas comandos-contexto-deck-mermaid comandos-contexto-deck-camadas comandos-contexto-deck-base
  comandos-contexto-paleta comandos-contexto-workspace comandos-contexto-base
  comandos-execucao comandos-jobs comandos-interface comandos-seguranca comandos-outros app-chat app-demais
)
mapfile -t script_groups < <(bash "$script_dir/testar-go-race.sh" --groups)
printf '%s\n' "${script_groups[@]}" > "$test_dir/script-groups"
printf '%s\n' "${expected_groups[@]}" > "$test_dir/expected-groups"
assert_unique 'testar-go-race.sh --groups' "$test_dir/script-groups"
diff -u "$test_dir/expected-groups" "$test_dir/script-groups"

# A matriz deve espelhar --groups sem omissões, inclusões ou duplicatas.
awk '
  $0 == "        group:" { capture = 1; next }
  capture && /^          - / { sub(/^          - /, ""); print; next }
  capture { exit }
' "$repo_dir/.github/workflows/ci.yml" > "$test_dir/ci-groups"
cp "$test_dir/script-groups" "$test_dir/script-ci-groups"
[[ -s "$test_dir/ci-groups" ]] || fail 'Não foi possível ler a lista matrix.group em ci.yml'
assert_unique 'matriz backend-race-tests do CI' "$test_dir/ci-groups"
diff -u "$test_dir/script-ci-groups" "$test_dir/ci-groups"

assert_unique 'fixture de nomes' "$test_dir/expected"
mapfile -t fixture_names < "$test_dir/expected"
for group in "${expected_groups[@]:1}"; do
  run "$group"
  assert_flags 'test -race -short -count=1 -timeout=20m'
  [[ ${#got[@]} == 8 && ${got[0]} == test && ${got[5]} == -run && ${got[7]} == ./internal/app ]] ||
    fail "invocação de execução do grupo alterada: ${got[*]}"
  pattern=${got[6]}
  [[ $pattern == '^('* && $pattern == *')$' ]] || fail "regex não ancorada para $group: $pattern"

  awk -F '\t' -v group="$group" '$1 == group { print $2 }' "$test_dir/expected-map" > "$test_dir/expected-selection"
  grep -E "$pattern" "$test_dir/expected" > "$test_dir/actual-selection" || true
  diff -u <(LC_ALL=C sort "$test_dir/expected-selection") <(LC_ALL=C sort "$test_dir/actual-selection") ||
    fail "o grupo $group não selecionou exatamente sua família"

  # Confirma que a lista de execução impressa também corresponde ao destino,
  # em vez de validar apenas uma união disjunta dos testes.
  : > "$test_dir/printed-selection"
  for name in "${fixture_names[@]}"; do
    if grep -Fxq -- "$name" "$test_dir/output"; then
      printf '%s\n' "$name" >> "$test_dir/printed-selection"
    fi
  done
  diff -u <(LC_ALL=C sort "$test_dir/expected-selection") <(LC_ALL=C sort "$test_dir/printed-selection") ||
    fail "a saída do grupo $group divergiu da seleção esperada"
  if printf '%s\n' BenchmarkSpeed TestMain TestAB | grep -Eq "$pattern"; then
    fail "o grupo $group selecionou benchmark, TestMain ou prefixo parcial"
  fi
done

run pacotes-gerais
assert_flags 'test -race -short -count=1 -timeout=20m'
[[ ${#got[@]} == 7 && ${got[0]} == test && ${got[5]} == assistente/internal/app/child && ${got[6]} == assistente/internal/auth ]] ||
  fail "pacotes-gerais repetiu App ou mudou a lista: ${got[*]}"
grep -Fxq 'list -race ./...' "$test_dir/calls"
grep -Fxq 'test -race -short -count=1 -timeout=20m -list . ./internal/app' "$test_dir/calls"

for args in '' '8 8' '-1 8' '0 0' '08 8' '0 08' '9999 8' 'x 8' 'others 8'; do
  # Separação intencional: são argumentos inválidos fixos, sem entrada externa.
  read -r -a invalid <<< "$args"
  reject "${invalid[@]}"
done
MOCK_FAIL=discovery reject comandos-configuracao
MOCK_FAIL=execution reject comandos-configuracao
MOCK_FAIL=packages reject pacotes-gerais
MOCK_FAIL=execution reject pacotes-gerais
: > "$test_dir/names"
reject comandos-configuracao
printf '%s\n' TestOnly > "$test_dir/names"
reject comandos-configuracao
: > "$test_dir/packages"
reject pacotes-gerais
echo 'PASS: destino exato dos grupos, fallback residual, flags, matriz CI, pacotes e falhas'
