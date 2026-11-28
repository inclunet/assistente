#!/usr/bin/env bash
set -euo pipefail
script_dir=$(cd -- "$(dirname -- "$0")" && pwd)
repo_dir=$(cd -- "$script_dir/../.." && pwd -P)
test_root="$repo_dir/build/bin/ci/reutilizar-arvore-test"
mkdir -p "$test_root"
fixture=$(mktemp -d "$test_root/run.XXXXXX")
fixture=$(cd -- "$fixture" && pwd -P)
[[ $fixture == "$test_root/"* ]] || exit 1
trap 'rm -rf -- "$fixture"' EXIT

export REUSO_FIXTURE="$fixture" PATH="$fixture:$PATH"
export GITHUB_REPOSITORY=owner/repo GITHUB_RUN_ID=99
export PRIOR_SHA=cccccccccccccccccccccccccccccccccccccccc
arvore=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
outra_arvore=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
cat > "$fixture/gh" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
[[ $1 == api ]] || exit 81
case "$2" in
  --method)
    [[ $3 == GET && $4 == repos/owner/repo/actions/workflows/ci.yml/runs ]] || exit 82
    [[ $* == *' -f branch=codex/filho -f event=workflow_dispatch -f status=success -f created=>='* ]] || exit 83
    [[ ${MOCK_ERROR:-} != runs ]] || exit 84
    cat "$REUSO_FIXTURE/runs"
    ;;
  "repos/owner/repo/git/commits/$PRIOR_SHA")
    [[ ${MOCK_ERROR:-} != tree ]] || exit 85
    cat "$REUSO_FIXTURE/tree"
    ;;
  'repos/owner/repo/actions/runs/10/attempts/2/jobs?per_page=100')
    [[ ${MOCK_ERROR:-} != jobs ]] || exit 86
    printf '%s\n' "$2" >> "$REUSO_FIXTURE/calls"
    cat "$REUSO_FIXTURE/jobs"
    ;;
  *) exit 87 ;;
esac
MOCK
chmod +x "$fixture/gh"

: > "$fixture/calls"
run() { bash "$script_dir/reutilizar-ci-arvore.sh" "$1" "$2" "$3"; }
expect() {
  local found
  found=$(run 12 codex/filho "$arvore")
  [[ $found == "$1" ]] || { echo "Esperado '$1', recebido '$found'" >&2; exit 1; }
}
row() { printf '%s\t2\t%s\t%s\t%s\t%s\n' "$1" "$2" "$3" "$PRIOR_SHA" "$4" >> "$fixture/runs"; }

: > "$fixture/runs"
expect ''
row 99 workflow_dispatch owner/repo 12
row 10 pull_request owner/repo 12
row 10 workflow_dispatch other/repo 12
row 10 workflow_dispatch owner/repo 123
expect ''
[[ ! -s "$fixture/calls" ]]

row 10 workflow_dispatch owner/repo 11,12
printf '%s\n' "$outra_arvore" > "$fixture/tree"
expect ''
[[ ! -s "$fixture/calls" ]]

printf '%s\n' "$arvore" > "$fixture/tree"
printf '%s\n' scripts backend-checks backend-tests backend backend-race-build backend-race \
  frontend-tests frontend e2e bindings 'Terminal, instalador Windows e PATH do usuário' > "$fixture/jobs"
expect ''
printf '%s\n' e2e-tests >> "$fixture/jobs"
expect '10/attempts/2'

if MOCK_ERROR='tree' run 12 codex/filho "$arvore"; then exit 1; fi
if MOCK_ERROR='jobs' run 12 codex/filho "$arvore"; then exit 1; fi
if MOCK_ERROR=runs run 12 codex/filho "$arvore"; then exit 1; fi
if run 0 codex/filho "$arvore"; then exit 1; fi
if run 12 codex/filho 'inválida'; then exit 1; fi

# A checagem precisa de função de status explícita para evitar que o skip
# do guarda-dispatch se propague quando scripts teve sucesso.
workflow="$repo_dir/.github/workflows/ci.yml"
# shellcheck disable=SC2016
preparacao='    if: ${{ !cancelled() && needs.scripts.result == '\''success'\'' && needs.scripts.outputs.modo == '\''completo'\'' }}'
for job in backend-checks backend-tests backend-race-build backend-race-tests bindings frontend-tests e2e-tests windows-installer; do
  bloco=$(awk -v job="  $job:" '$0 == job { capture=1; next } capture && /^  [a-z]/ { exit } capture { print }' "$workflow")
  grep -Fxq '    needs: scripts' <<< "$bloco"
  grep -Fxq "$preparacao" <<< "$bloco"
done
bloco=$(awk '/^  backend-race-app:$/ { capture=1; next } capture && /^  [a-z]/ { exit } capture { print }' "$workflow")
grep -Fxq '    needs: backend-race-build' <<< "$bloco"
grep -Fxq "    if: \${{ !cancelled() && needs.backend-race-build.result == 'success' }}" <<< "$bloco"

echo 'PASS: PR, origem, árvore autenticada, tentativa, suítes completas e fallback'
