# AEP-0029 — Apêndice Histórico: GitHub Actions + Pages

Status: Superseded

## Resumo

Este apêndice registrava uma proposta antiga de auto-update baseada em GitHub Pages e `update-manifest.json`. Esse fluxo foi substituído pela implementação atual documentada em `0029-auto-update.md`.

## Decisão Atual

O auto-update não usa GitHub Pages nem manifest estático. A fonte canônica é a API de GitHub Releases:

- o aplicativo consulta `https://api.github.com/repos/inclunet/assistente/releases/latest`;
- o workflow `.github/workflows/release.yml` roda quando um release é criado;
- os assets são anexados ao GitHub Release existente;
- `internal/updater/updater.go` adapta os metadados do release para o `Manifest` interno.

## Valor Histórico

Este arquivo permanece apenas para preservar o histórico de decisão da série AEP-0029. Não use este apêndice como guia de implementação ou operação.

Para o fluxo vigente, consulte:

- `0029-auto-update.md`;
- `docs/content/guias/RELEASE_QUICKSTART.md`;
- `.github/workflows/release.yml`;
- `internal/updater/updater.go`.
