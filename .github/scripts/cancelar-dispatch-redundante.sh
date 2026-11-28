#!/usr/bin/env bash
# O PR valida seu merge SHA; um dispatch no head da branch não o substitui.
set -euo pipefail
[[ ${GITHUB_EVENT_NAME:-} == workflow_dispatch && ${CI_PR_NUMBER:-0} != 0 ]] || exit 0
[[ ${CI_PR_NUMBER:-} =~ ^[1-9][0-9]*$ && ${GITHUB_RUN_ID:-} =~ ^[1-9][0-9]*$ ]] || {
  echo '::error::Identidade do PR/run inválida.'
  exit 1
}
: "${GITHUB_REPOSITORY:?}" "${GITHUB_SHA:?}"

# shellcheck source=.github/scripts/ci-efetivo-pr.sh
source "$(dirname -- "${BASH_SOURCE[0]}")/ci-efetivo-pr.sh"

# O evento do PR pode chegar alguns segundos depois do dispatch do bot.
for atraso in 0 5 10 15; do
  if (( atraso > 0 )); then sleep "$atraso"; fi
  run_id=$(ci_efetivo_pr "$CI_PR_NUMBER" "$GITHUB_SHA")
  [[ -n $run_id ]] || continue
  echo "CI do PR #$CI_PR_NUMBER já representado pelo run $run_id; solicitando cancelamento do dispatch $GITHUB_RUN_ID."
  # Os gates obrigatórios usam always(): cancel normal pode deixá-los vivos.
  gh api "repos/$GITHUB_REPOSITORY/actions/runs/$GITHUB_RUN_ID/force-cancel" --method POST --silent
  exit 0
done
echo "Sem CI efetivo do PR #$CI_PR_NUMBER neste head; dispatch preservado."
