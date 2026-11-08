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
export GITHUB_EVENT_NAME=workflow_dispatch CI_PR_NUMBER=12
cat > "$test_dir/gh" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
[[ $1 == api ]] || exit 90
case $2 in
  'repos/owner/repo/actions/workflows/ci.yml/runs?event=pull_request&head_sha=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&per_page=100')
    [[ ${MOCK_FAIL:-} != query ]] || exit 91
    cat "$DISPATCH_TEST_DIR/runs" ;;
  'repos/owner/repo/actions/runs/10/jobs?per_page=1') printf '%s\n' "${MOCK_JOBS:-0}" ;;
  repos/owner/repo/actions/runs/99/cancel)
    [[ $3 == --method && $4 == POST && $5 == --silent ]] || exit 92
    [[ ${MOCK_FAIL:-} != cancel ]] || exit 93
    echo cancel >> "$DISPATCH_TEST_DIR/calls" ;;
  *) echo "API inesperada: $2" >&2; exit 94 ;;
esac
MOCK
chmod +x "$test_dir/gh"
fail() { echo "FAIL: $*" >&2; exit 1; }
run() {
  : > "$test_dir/calls"
  bash "$script_dir/cancelar-dispatch-redundante.sh" > "$test_dir/output" 2>&1
}
expect() {
  run || { cat "$test_dir/output"; fail 'script falhou'; }
  [[ $(wc -l < "$test_dir/calls" | tr -d ' ') == "$1" ]] || fail "cancelamentos esperados=$1"
}
row() { printf '10\t%s\t%s\t%s\t%s\n' "$1" "$2" "$3" "$4" > "$test_dir/runs"; }
: > "$test_dir/runs"
expect 0
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
row pull_request "$GITHUB_SHA" success 12
GITHUB_EVENT_NAME=pull_request expect 0
CI_PR_NUMBER=0 expect 0
if CI_PR_NUMBER=invalid run; then fail 'aceitou PR inválido'; fi
if MOCK_FAIL=query run; then fail 'ocultou erro de consulta'; fi
if MOCK_FAIL=cancel run; then fail 'ocultou erro de cancelamento'; fi
row pull_request "$GITHUB_SHA" action_required 12
if MOCK_JOBS=invalid run; then fail 'aceitou inventário inválido'; fi
# Namespaces distintos protegem tanto PR ativo quanto PR pendente.
grep -Fq "group: ci-\${{ github.event_name == 'pull_request' && 'pr' || inputs.pr_number > 0 && 'dispatch' || 'ref' }}-" "$repo_dir/.github/workflows/ci.yml" || fail 'grupos de PR e dispatch deixaram de ser separados'
# O sentinela zero nunca identifica o grupo: manuais independentes usam a ref.
grep -Fq "\${{ github.event.pull_request.number || (inputs.pr_number > 0 && inputs.pr_number) || github.ref_name }}" "$repo_dir/.github/workflows/ci.yml" || fail 'dispatch sem PR deixou de usar a ref'
echo 'PASS: prioridade do PR, isolamento por PR/head/evento, aprovação sem jobs e falhas visíveis'
