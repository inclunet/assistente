#!/usr/bin/env bash
# Imprime o ID do CI associado ao PR e ao commit. Saída vazia significa que
# ainda não há CI efetivo. Erros da API são propagados ao chamador.
ci_efetivo_pr() {
  local numero=$1 sha=$2 runs run_id event head conclusion prs status actor jobs
  runs=$(gh api "repos/$GITHUB_REPOSITORY/actions/workflows/ci.yml/runs?event=pull_request&head_sha=$sha&per_page=100" \
    --jq '.workflow_runs[] | [.id, .event, .head_sha, (.conclusion // "none"), ((.pull_requests | map(.number | tostring) | join(",")) | if . == "" then "-" else . end), .status, .triggering_actor.login] | @tsv') || return 1
  while IFS=$'\t' read -r run_id event head conclusion prs status actor; do
    [[ $event == pull_request && $head == "$sha" && ",$prs," == *",$numero,"* ]] || continue
    [[ $run_id =~ ^[1-9][0-9]*$ ]] || { echo '::error::Run retornado pela API é inválido.' >&2; return 1; }
    # O evento do bot pode aparecer antes de o GitHub registrar a exigência de
    # aprovação. Um run já aprovado pode ficar na fila antes de ter jobs.
    if [[ $conclusion == action_required || $status == waiting ||
          ( $conclusion == none && $status != in_progress && $actor == 'github-actions[bot]' ) ]]; then
      jobs=$(gh api "repos/$GITHUB_REPOSITORY/actions/runs/$run_id/jobs?per_page=1" --jq '.total_count') || return 1
      [[ $jobs =~ ^[0-9]+$ ]] || { echo '::error::Inventário de jobs inválido.' >&2; return 1; }
      [[ $jobs != 0 ]] || continue
    fi
    printf '%s\n' "$run_id"
    return 0
  done <<< "$runs"
  return 0
}
