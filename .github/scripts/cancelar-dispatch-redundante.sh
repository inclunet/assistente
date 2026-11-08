#!/usr/bin/env bash
# O PR valida seu merge SHA; um dispatch no head da branch não o substitui.
set -euo pipefail
[[ ${GITHUB_EVENT_NAME:-} == workflow_dispatch && ${CI_PR_NUMBER:-0} != 0 ]] || exit 0
[[ ${CI_PR_NUMBER:-} =~ ^[1-9][0-9]*$ && ${GITHUB_RUN_ID:-} =~ ^[1-9][0-9]*$ ]] || {
  echo '::error::Identidade do PR/run inválida.'
  exit 1
}
: "${GITHUB_REPOSITORY:?}" "${GITHUB_SHA:?}"

# A consulta é feita ao começar o dispatch. PR e dispatch têm grupos distintos:
# um PR criado depois da consulta pode duplicar trabalho, nunca perder checks.
runs=$(gh api "repos/$GITHUB_REPOSITORY/actions/workflows/ci.yml/runs?event=pull_request&head_sha=$GITHUB_SHA&per_page=100" \
  --jq '.workflow_runs[] | [.id, .event, .head_sha, (.conclusion // "none"), ((.pull_requests | map(.number | tostring) | join(",")) | if . == "" then "-" else . end)] | @tsv')
while IFS=$'\t' read -r run_id event sha conclusion prs; do
  [[ $event == pull_request && $sha == "$GITHUB_SHA" && ",$prs," == *",$CI_PR_NUMBER,"* ]] || continue
  [[ $run_id =~ ^[1-9][0-9]*$ ]] || { echo '::error::Run retornado pela API é inválido.'; exit 1; }
  # Um evento do bot aguardando aprovação e sem jobs não valida o head.
  # Falha/cancelamento de um PR efetivo, porém, nunca vira verde pelo dispatch.
  if [[ $conclusion == action_required ]]; then
    jobs=$(gh api "repos/$GITHUB_REPOSITORY/actions/runs/$run_id/jobs?per_page=1" --jq '.total_count')
    [[ $jobs =~ ^[0-9]+$ ]] || { echo '::error::Inventário de jobs inválido.'; exit 1; }
    [[ $jobs != 0 ]] || continue
  fi
  echo "CI do PR #$CI_PR_NUMBER já representado pelo run $run_id; solicitando cancelamento do dispatch $GITHUB_RUN_ID."
  gh api "repos/$GITHUB_REPOSITORY/actions/runs/$GITHUB_RUN_ID/cancel" --method POST --silent
  exit 0
done <<< "$runs"
echo "Sem CI efetivo do PR #$CI_PR_NUMBER neste head; dispatch preservado."
