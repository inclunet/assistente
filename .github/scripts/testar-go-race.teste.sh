#!/usr/bin/env bash
# Nunca executa Go real: o PATH aponta para um mock privado deste teste.
set -euo pipefail
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
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
printf '%s\n' TestZ TestA TestB TestC TestD TestE TestF TestG FuzzInput Example ExampleOutput TestAção > "$test_dir/expected"
{
  cat "$test_dir/expected"
  printf '%s\n' BenchmarkSpeed TestMain 'ok assistente/internal/app 0.001s'
} > "$test_dir/names"

run() { bash "$script_dir/testar-go-race.sh" "$@" > "$test_dir/output" 2>&1; }
reject() { if run "$@"; then echo "Deveria falhar: $*" >&2; exit 1; fi; }
assert_flags() {
  for flag in -race -short -count=1 -timeout=20m; do
    grep -Fxq -- "$flag" "$test_dir/args"
  done
}

: > "$test_dir/selected"
for index in {0..7}; do
  run "$index" 8
  assert_flags
  pattern=$(awk 'previous == "-run" {print; exit} {previous=$0}' "$test_dir/args")
  [[ $pattern == '^('* && $pattern == *')$' ]]
  grep -E "$pattern" "$test_dir/expected" >> "$test_dir/selected"
  # Não deve selecionar Benchmark/TestMain nem nomes apenas parcialmente iguais.
  if printf '%s\n' BenchmarkSpeed TestMain TestAB | grep -Eq "$pattern"; then exit 1; fi
done
diff <(LC_ALL=C sort "$test_dir/expected") <(LC_ALL=C sort "$test_dir/selected")
run others
assert_flags
grep -Fxq 'list -race ./...' "$test_dir/calls"
if grep -Fxq assistente/internal/app "$test_dir/args"; then
  echo 'O grupo others não pode repetir App' >&2
  exit 1
fi
grep -Fxq assistente/internal/app/child "$test_dir/args"
grep -Fxq assistente/internal/auth "$test_dir/args"
for args in '' '8 8' '-1 8' '0 0' '08 8' '0 08' '9999 8' 'x 8' 'others 8'; do
  # Separação intencional: são argumentos inválidos fixos, sem entrada externa.
  read -r -a invalid <<< "$args"
  reject "${invalid[@]}"
done
MOCK_FAIL=discovery reject 0 8
MOCK_FAIL=execution reject 0 8
MOCK_FAIL=packages reject others
MOCK_FAIL=execution reject others
: > "$test_dir/names"
reject 0 8
printf '%s\n' TestOnly > "$test_dir/names"
reject 7 8
: > "$test_dir/packages"
reject others
echo 'PASS: partição completa/disjunta, nomes Unicode, flags, pacotes e falhas'
