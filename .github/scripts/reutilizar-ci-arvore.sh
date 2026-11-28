#!/usr/bin/env bash
set -euo pipefail

numero=$1
branch=$2
arvore=$3
[[ $numero =~ ^[1-9][0-9]*$ && $arvore =~ ^[a-f0-9]{40}$ &&
   ${GITHUB_RUN_ID:-} =~ ^[1-9][0-9]*$ ]] || exit 1
: "${GITHUB_REPOSITORY:?}"
git check-ref-format "refs/heads/$branch" >/dev/null

# No dispatch, o SHA publicado pelo GitHub é o checkout padrão do workflow.
# Em pull_request ele identifica o head, embora a suíte teste o merge sintético.
desde=$(date -u -d '24 hours ago' '+%Y-%m-%dT%H:%M:%SZ')
runs=$(gh api --method GET "repos/$GITHUB_REPOSITORY/actions/workflows/ci.yml/runs" \
  -f "branch=$branch" -f event=workflow_dispatch -f status=success -f "created=>=$desde" -f per_page=20 \
  --jq '.workflow_runs[] | select(.status == "completed" and .conclusion == "success") |
    [.id, .run_attempt, .event, .head_repository.full_name, .head_sha,
      ([.pull_requests[].number | tostring] | join(","))] | @tsv')

obrigatorios=(scripts backend-checks backend-tests backend backend-race-build backend-race
  frontend-tests frontend e2e-tests e2e bindings 'Terminal, instalador Windows e PATH do usuário')
while IFS=$'\t' read -r id tentativa evento repositorio sha prs; do
  [[ $id =~ ^[1-9][0-9]*$ && $tentativa =~ ^[1-9][0-9]*$ && $sha =~ ^[a-f0-9]{40}$ &&
     $id != "$GITHUB_RUN_ID" && $repositorio == "$GITHUB_REPOSITORY" &&
     $evento == workflow_dispatch && ",$prs," == *",$numero,"* ]] || continue
  arvore_anterior=$(gh api "repos/$GITHUB_REPOSITORY/git/commits/$sha" --jq '.tree.sha')
  [[ $arvore_anterior == "$arvore" ]] || continue

  jobs=$(gh api "repos/$GITHUB_REPOSITORY/actions/runs/$id/attempts/$tentativa/jobs?per_page=100" \
    --jq '.jobs[] | select(.status == "completed" and .conclusion == "success") | .name')
  for job in "${obrigatorios[@]}"; do
    grep -Fxq "$job" <<< "$jobs" || continue 2
  done
  printf '%s/attempts/%s\n' "$id" "$tentativa"
  exit 0
done <<< "$runs"
