#!/usr/bin/env bash
# Nunca executa Go real: o PATH aponta para um mock privado deste teste.
set -euo pipefail
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_dir=$(cd -- "$script_dir/../.." && pwd)
test_root="$repo_dir/build/bin/ci/race-script-test"
mkdir -p -- "$test_root"
test_root=$(cd -- "$test_root" && pwd -P)
test_dir=$(mktemp -d "$test_root/run.XXXXXX")
test_dir=$(cd -- "$test_dir" && pwd -P)
[[ $test_dir == "$test_root/"* ]] || exit 1
trap 'rm -rf -- "$test_dir"' EXIT
export RACE_MOCK_DIR="$test_dir"
export PATH="$test_dir:$PATH"
# Repositório sintético: nunca substituir um binário real do worktree.
mkdir -p "$test_dir/repo/internal/app" "$test_dir/repo/build/bin/ci"
cd "$test_dir/repo"
cat > "$test_dir/go" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$RACE_MOCK_DIR/calls"
if [[ $1 == list ]]; then
  [[ ${MOCK_FAIL:-} != packages ]] || exit 17
  cat "$RACE_MOCK_DIR/packages"
elif [[ $1 == tool ]]; then
  [[ ${MOCK_FAIL:-} != execution ]] || exit 19
  printf '%s\n' "$@" > "$RACE_MOCK_DIR/args"
  [[ $PWD == "$RACE_MOCK_DIR/repo/internal/app" ]] || exit 20
elif [[ " $* " == *' -list '* ]]; then
  [[ ${MOCK_FAIL:-} != discovery ]] || exit 18
  if [[ ${!#} == ./internal/chat ]]; then
    cat "$RACE_MOCK_DIR/chat-names"
  else
    cat "$RACE_MOCK_DIR/names"
  fi
else
  [[ ${MOCK_FAIL:-} != execution ]] || exit 19
  printf '%s\n' "$@" > "$RACE_MOCK_DIR/args"
fi
MOCK
chmod +x "$test_dir/go"

printf '%s\n' assistente/internal/app assistente/internal/app/child assistente/internal/chat assistente/internal/chat/child assistente/internal/wailsapi assistente/internal/wailsapi/child assistente/internal/database assistente/internal/database/child assistente/internal/auth > "$test_dir/packages"

cat > "$test_dir/repo/build/bin/ci/app-race.test" <<'BINARY'
#!/usr/bin/env bash
set -euo pipefail
[[ $PWD == "$RACE_MOCK_DIR/repo/internal/app" ]] || exit 20
[[ $* == '-test.short -test.list .' ]] || exit 21
[[ ${MOCK_FAIL:-} != discovery ]] || exit 18
cat "$RACE_MOCK_DIR/names"
BINARY
chmod +x "$test_dir/repo/build/bin/ci/app-race.test"

# Famílias antigas e nomes novos entram pelo mesmo contrato de descoberta.
printf '%s\n' TestCommandSettingsA TestContextualDeckA TestCommandEditorA TestChatA TestChatB TestFutureTimeline TestAção FuzzInput Example ExampleOutput > "$test_dir/expected"
{
  cat "$test_dir/expected"
  printf '%s\n' BenchmarkSpeed TestMain 'ok assistente/internal/app 0.001s'
} > "$test_dir/names"
cp "$test_dir/names" "$test_dir/chat-names"

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

expected_groups=(pacotes-gerais pacotes-database-wailsapi pacote-chat-0 pacote-chat-1 pacote-app-0 pacote-app-1 pacote-app-2 pacote-app-3)
mapfile -t script_groups < <(bash "$script_dir/testar-go-race.sh" --groups)
printf '%s\n' "${script_groups[@]}" > "$test_dir/script-groups"
printf '%s\n' "${expected_groups[@]}" > "$test_dir/expected-groups"
assert_unique 'testar-go-race.sh --groups' "$test_dir/script-groups"
diff -u "$test_dir/expected-groups" "$test_dir/script-groups"

# A matriz deve espelhar --groups sem omissões, inclusões ou duplicatas.
awk '
  $0 == "        group:" { capture = 1; next }
  capture && /^          - / { sub(/^          - /, ""); print; next }
  capture { capture = 0 }
' "$repo_dir/.github/workflows/ci.yml" > "$test_dir/ci-groups"
cp "$test_dir/script-groups" "$test_dir/script-ci-groups"
[[ -s "$test_dir/ci-groups" ]] || fail 'Não foi possível ler a lista matrix.group em ci.yml'
assert_unique 'matriz backend-race-tests do CI' "$test_dir/ci-groups"
diff -u "$test_dir/script-ci-groups" "$test_dir/ci-groups"

# Produtor e executores restauram o mesmo namespace de objetos instrumentados.
printf '%s\n' go.sum .github/scripts/testar-go-race.sh > "$test_dir/cache-expected"
for job in backend-race-build backend-race-tests backend-race-app; do
  awk -v job="  $job:" '
    $0 == job { capture=1; next }
    capture && /^  [a-z]/ { exit }
    capture { print }
  ' "$repo_dir/.github/workflows/ci.yml" > "$test_dir/cache-job"
  awk '
    /^          cache-dependency-path: \|$/ { capture=1; next }
    capture && /^            / { sub(/^            /, ""); print; next }
    capture { exit }
  ' "$test_dir/cache-job" > "$test_dir/cache-paths"
  diff -u "$test_dir/cache-expected" "$test_dir/cache-paths"
done

# Executa o gate real do workflow: só sucesso de todas as dependências aprova.
awk '/^  backend-race:$/ { capture=1; next } capture && /^  [a-z]/ { exit } capture { print }' "$repo_dir/.github/workflows/ci.yml" > "$test_dir/gate"
grep -Fq 'needs: [backend-race-build, backend-race-tests, backend-race-app]' "$test_dir/gate"
# Expressões literais do GitHub, não expansão pelo shell.
# shellcheck disable=SC2016
for binding in 'BUILD_RESULT: ${{ needs.backend-race-build.result }}' 'RACE_RESULT: ${{ needs.backend-race-tests.result }}' 'APP_RESULT: ${{ needs.backend-race-app.result }}'; do
  grep -Fq "$binding" "$test_dir/gate" || fail 'dependência do gate desconectada'
done
# shellcheck disable=SC2016
grep -Fq 'if: ${{ always() }}' "$test_dir/gate"
awk '/^        run: \|$/ { capture=1; next } capture && /^          / { sub(/^          /, ""); print }' "$test_dir/gate" > "$test_dir/gate.sh"
[[ -s "$test_dir/gate.sh" ]] || fail 'gate não encontrado'
BUILD_RESULT=success RACE_RESULT=success APP_RESULT=success bash -e "$test_dir/gate.sh"
for dependency in BUILD_RESULT RACE_RESULT APP_RESULT; do
  for result in failure cancelled skipped ''; do
    if env BUILD_RESULT=success RACE_RESULT=success APP_RESULT=success "$dependency=$result" bash -e "$test_dir/gate.sh"; then
      fail "gate aprovou $dependency=$result"
    fi
  done
done

# Testa o contrato da compilação sem executar Go real.
run --build-app
mapfile -t got < "$test_dir/args"
[[ ${got[*]:0:4} == 'test -race -c -o' && ${got[4]} == "$test_dir/repo/build/bin/ci/app-race.test" && ${got[5]} == ./internal/app && ${#got[@]} == 6 ]] || fail 'compilação não preservou race/pacote/destino'
MOCK_FAIL=execution reject --build-app

# União disjunta, nomes futuros e estabilidade; sem replicar o hash no teste.
for package in chat app; do
  : > "$test_dir/selected"
  for group in "${expected_groups[@]}"; do
    [[ $group == pacote-$package-* ]] || continue
    run "$group"
    mapfile -t got < "$test_dir/args"
    if [[ $package == app ]]; then
      [[ ${got[*]:0:5} == 'tool test2json -t -p assistente/internal/app' && ${got[5]} == "$test_dir/repo/build/bin/ci/app-race.test" && ${got[*]:6:5} == '-test.v=test2json -test.short -test.count=1 -test.timeout=20m -test.run' && ${#got[@]} == 12 ]] || fail "invocação App incorreta: ${got[*]}"
      pattern=${got[11]}
    else
      assert_flags 'test -race -short -count=1 -timeout=20m'
      [[ ${#got[@]} == 9 && ${got[5]} == -v && ${got[6]} == -run && ${got[8]} == ./internal/chat ]] || fail 'invocação Chat incorreta'
      pattern=${got[7]}
    fi
    [[ $pattern == '^('* && $pattern == *')$' ]] || fail "regex não ancorada: $pattern"
    grep -E "$pattern" "$test_dir/expected" >> "$test_dir/selected" || fail "grupo vazio: $group"
    grep -E "$pattern" "$test_dir/expected" > "$test_dir/before"
    if printf '%s\n' BenchmarkSpeed TestMain TestChatAB | grep -Eq "$pattern"; then
      fail 'selecionou benchmark, TestMain ou prefixo parcial'
    fi
    # Adicionar um nome não desloca nenhum teste preexistente.
    cp "$test_dir/names" "$test_dir/original"
    printf '%s\n' TestNewUnrelatedName >> "$test_dir/names"
    cp "$test_dir/names" "$test_dir/chat-names"
    run "$group"
    mapfile -t added < "$test_dir/args"
    index=11
    [[ $package == app ]] || index=7
    grep -E "${added[index]}" "$test_dir/expected" > "$test_dir/after"
    diff -u "$test_dir/before" "$test_dir/after"
    cp "$test_dir/original" "$test_dir/names"
    cp "$test_dir/original" "$test_dir/chat-names"
  done
  assert_unique "grupos de $package" "$test_dir/selected"
  diff -u <(LC_ALL=C sort "$test_dir/expected") <(LC_ALL=C sort "$test_dir/selected")
done

run pacotes-gerais
assert_flags 'test -race -short -count=1 -timeout=20m'
[[ ${#got[@]} == 11 && ${got[0]} == test && ${got[5]} == -json && ${got[6]} == assistente/internal/app/child && ${got[7]} == assistente/internal/chat/child && ${got[8]} == assistente/internal/wailsapi/child && ${got[9]} == assistente/internal/database/child && ${got[10]} == assistente/internal/auth ]] ||
  fail "pacotes-gerais repetiu App/Chat/WailsAPI/Database ou mudou a lista: ${got[*]}"

run pacotes-database-wailsapi
assert_flags 'test -race -short -count=1 -timeout=20m'
[[ ${#got[@]} == 8 && ${got[5]} == -json && ${got[6]} == ./internal/database && ${got[7]} == ./internal/wailsapi ]] ||
  fail "pacotes-database-wailsapi não executou os dois pacotes inteiros exatamente uma vez: ${got[*]}"

grep -Fxq 'list -race ./...' "$test_dir/calls"
grep -Fxq 'test -race -short -count=1 -timeout=20m -list . ./internal/chat' "$test_dir/calls"
# Nenhum consumidor App recompila o pacote.
[[ $(grep -c '^test -race -c' "$test_dir/calls") == 2 ]] || fail 'compilação App repetida'

for args in '' '8 8' '-1 8' '0 0' '08 8' '0 08' '9999 8' 'x 8' 'others 8'; do
  # Separação intencional: são argumentos inválidos fixos, sem entrada externa.
  read -r -a invalid <<< "$args"
  reject "${invalid[@]}"
done
MOCK_FAIL=discovery reject pacote-app-0
MOCK_FAIL=execution reject pacote-app-0
MOCK_FAIL=packages reject pacotes-gerais
MOCK_FAIL=execution reject pacotes-gerais
MOCK_FAIL=execution reject pacotes-database-wailsapi
MOCK_FAIL=execution reject pacote-chat-0
MOCK_FAIL=discovery reject pacote-chat-0
: > "$test_dir/chat-names"
reject pacote-chat-0
: > "$test_dir/names"
reject pacote-app-0
printf '%s\n' TestOnly > "$test_dir/names"
# Com um teste, ao menos três partições devem falhar por estarem vazias.
failures=0
for group in pacote-app-{0..3}; do
  if ! run "$group"; then failures=$((failures + 1)); fi
done
[[ $failures == 3 ]] || fail 'grupo vazio não foi recusado'
rm "$test_dir/repo/build/bin/ci/app-race.test"
reject pacote-app-0
: > "$test_dir/packages"
reject pacotes-gerais
printf '%s\n' assistente/internal/app assistente/internal/wailsapi assistente/internal/database assistente/internal/auth > "$test_dir/packages"
reject pacotes-gerais
printf '%s\n' assistente/internal/chat assistente/internal/wailsapi assistente/internal/database assistente/internal/auth > "$test_dir/packages"
reject pacotes-gerais
printf '%s\n' assistente/internal/app assistente/internal/chat assistente/internal/database assistente/internal/auth > "$test_dir/packages"
reject pacotes-gerais
printf '%s\n' assistente/internal/app assistente/internal/chat assistente/internal/wailsapi assistente/internal/auth > "$test_dir/packages"
reject pacotes-gerais
printf '%s\n' assistente/internal/app assistente/internal/chat assistente/internal/wailsapi assistente/internal/database assistente/internal/database assistente/internal/auth > "$test_dir/packages"
reject pacotes-gerais

echo 'PASS: compilação única, partições completas/disjuntas/estáveis, flags, cwd, matriz, pacotes e falhas'
