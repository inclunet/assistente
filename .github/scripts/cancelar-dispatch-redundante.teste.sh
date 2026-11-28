#!/usr/bin/env bash
set -euo pipefail
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_dir=$(cd -- "$script_dir/../.." && pwd)
test_root="$repo_dir/build/bin/ci/dispatch-script-test"
mkdir -p "$test_root"
test_dir=$(mktemp -d "$test_root/run.XXXXXX")
[[ $test_dir == "$test_root/"* ]] || exit 1
trap 'rm -rf -- "$test_dir"' EXIT
export DISPATCH_TEST_DIR="$test_dir" PATH="$test_dir:$PATH"
export GITHUB_REPOSITORY=owner/repo GITHUB_SHA=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa GITHUB_RUN_ID=99
export EXPECTED_DISPATCH_RUN_ID=99
export GITHUB_EVENT_NAME=workflow_dispatch CI_PR_NUMBER=12
cat > "$test_dir/gh" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
[[ $1 == api ]] || exit 90
case $2 in
  'repos/owner/repo/actions/workflows/ci.yml/runs?event=pull_request&head_sha=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&per_page=100')
    printf 'consulta\n' >> "$DISPATCH_TEST_DIR/queries"
    [[ ${MOCK_FAIL:-} != query ]] || exit 91
    consultas=$(wc -l < "$DISPATCH_TEST_DIR/queries")
    if [[ ( ${MOCK_FAIL:-} == first-query && $consultas -eq 1 ) ||
          ( ${MOCK_FAIL:-} == last-query && $consultas -eq 4 ) ]]; then
      exit 91
    fi
    cat "$DISPATCH_TEST_DIR/runs" ;;
  'repos/owner/repo/actions/runs/10/jobs?per_page=1') printf '%s\n' "${MOCK_JOBS:-0}" ;;
  "repos/owner/repo/actions/runs/$EXPECTED_DISPATCH_RUN_ID/force-cancel")
    [[ $3 == --method && $4 == POST && $5 == --silent ]] || exit 92
    [[ ${MOCK_FAIL:-} != cancel ]] || exit 93
    echo force-cancel >> "$DISPATCH_TEST_DIR/calls" ;;
  *) echo "API inesperada: $2" >&2; exit 94 ;;
esac
MOCK
cat > "$test_dir/sleep" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
if [[ ${MOCK_PR_ATRASADO:-} == yes ]]; then
  cp "$DISPATCH_TEST_DIR/atrasado" "$DISPATCH_TEST_DIR/runs"
fi
MOCK
chmod +x "$test_dir/gh" "$test_dir/sleep"
fail() { echo "FAIL: $*" >&2; exit 1; }
run() {
  : > "$test_dir/calls"
  : > "$test_dir/queries"
  bash "$script_dir/cancelar-dispatch-redundante.sh" > "$test_dir/output" 2>&1
}
expect() {
  run || { cat "$test_dir/output"; fail 'script falhou'; }
  [[ $(wc -l < "$test_dir/calls" | tr -d ' ') == "$1" ]] || fail "cancelamentos esperados=$1"
}
row() { printf '10\t%s\t%s\t%s\t%s\t%s\t%s\n' "$1" "$2" "$3" "$4" "${5:-completed}" "${6:-aprovador}" > "$test_dir/runs"; }
: > "$test_dir/runs"
expect 0
[[ $(wc -l < "$test_dir/queries") -eq 4 ]] || fail 'fallback não esperou pelo PR'
printf '10\tpull_request\t%s\tnone\t12\tqueued\taprovador\n' "$GITHUB_SHA" > "$test_dir/atrasado"
MOCK_PR_ATRASADO=yes expect 1
[[ $(wc -l < "$test_dir/queries") -eq 2 ]] || fail 'não aproveitou o PR publicado durante a espera'
for conclusion in none success failure cancelled timed_out skipped; do
  row pull_request "$GITHUB_SHA" "$conclusion" 12
  expect 1
done
row pull_request "$GITHUB_SHA" success 112
expect 0
row pull_request bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb success 12
expect 0
row push "$GITHUB_SHA" success 12
expect 0
row pull_request "$GITHUB_SHA" success 11,12
expect 1
row pull_request "$GITHUB_SHA" action_required 12
MOCK_JOBS=0 expect 0
MOCK_JOBS=1 expect 1
row pull_request "$GITHUB_SHA" none 12 queued 'github-actions[bot]'
MOCK_JOBS=0 expect 0
MOCK_JOBS=1 expect 1
row pull_request "$GITHUB_SHA" none 12 waiting aprovador
MOCK_JOBS=0 expect 0
row pull_request "$GITHUB_SHA" none 12 queued aprovador
MOCK_JOBS=0 expect 1
row pull_request "$GITHUB_SHA" success 12
GITHUB_RUN_ID=100 EXPECTED_DISPATCH_RUN_ID=100 expect 1
GITHUB_EVENT_NAME=pull_request expect 0
CI_PR_NUMBER=0 expect 0
if CI_PR_NUMBER=invalid run; then fail 'aceitou PR inválido'; fi
if GITHUB_RUN_ID=invalid run; then fail 'aceitou run inválido'; fi
MOCK_FAIL=first-query expect 1
[[ $(wc -l < "$test_dir/queries") -eq 2 ]] || fail 'não recuperou consulta antes de cancelar'
: > "$test_dir/runs"
MOCK_FAIL=first-query expect 0
[[ $(wc -l < "$test_dir/queries") -eq 4 ]] || fail 'não retomou a espera após erro transitório'
if MOCK_FAIL=query run; then fail 'ocultou erro persistente de consulta'; fi
grep -Fq 'Não foi possível confirmar a ausência do CI' "$test_dir/output" || fail 'erro persistente sem diagnóstico'
if MOCK_FAIL=last-query run; then fail 'aceitou resposta vazia anterior após erro na última consulta'; fi
[[ ! -s "$test_dir/calls" ]] || fail 'cancelou sem confirmação da API'
grep -Fq 'Não foi possível confirmar a ausência do CI' "$test_dir/output" || fail 'erro na última consulta sem diagnóstico'
row pull_request "$GITHUB_SHA" success 12
if MOCK_FAIL=cancel run; then fail 'ocultou erro de cancelamento'; fi
row pull_request "$GITHUB_SHA" action_required 12
if MOCK_JOBS=invalid run; then fail 'aceitou inventário inválido'; fi
# Namespaces distintos protegem tanto PR ativo quanto PR pendente.
grep -Fq "group: ci-\${{ github.event_name == 'pull_request' && 'pr' || inputs.pr_number > 0 && 'dispatch' || 'ref' }}-" "$repo_dir/.github/workflows/ci.yml" || fail 'grupos de PR e dispatch deixaram de ser separados'
# O sentinela zero nunca identifica o grupo: manuais independentes usam a ref.
grep -Fq "\${{ github.event.pull_request.number || (inputs.pr_number > 0 && inputs.pr_number) || github.ref_name }}" "$repo_dir/.github/workflows/ci.yml" || fail 'dispatch sem PR deixou de usar a ref'
# Jobs sem sobrescrita não podem herdar escrita do padrão do repositório.
grep -A2 '^permissions:$' "$repo_dir/.github/workflows/ci.yml" | grep -Fq '  contents: read' || fail 'padrão contents deixou de ser leitura'
grep -A2 '^permissions:$' "$repo_dir/.github/workflows/ci.yml" | grep -Fq '  actions: read' || fail 'padrão actions deixou de ser leitura'
# Em um DAG válido, uma única raiz implica que todo trabalho aguarda o guard.
# Novos jobs sem dependência também devem falhar nesta prova.
awk '
  /^jobs:$/ { in_jobs=1; next }
  in_jobs && /^  [A-Za-z_][A-Za-z0-9_-]*:$/ {
    job=$1; sub(/:$/, "", job); jobs[job]=1
  }
  in_jobs && /^    needs:/ { dependencies[job]=1 }
  END {
    if (!("guarda-dispatch" in jobs) || ("guarda-dispatch" in dependencies)) exit 1
    for (job in jobs) if (job != "guarda-dispatch" && !(job in dependencies)) exit 1
  }
' "$repo_dir/.github/workflows/ci.yml" || fail 'job de trabalho pode iniciar antes do guard'
echo 'PASS: prioridade do PR, isolamento por PR/head/evento, aprovação sem jobs e falhas visíveis'
