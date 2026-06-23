# AEP-0029 — Apêndice Histórico: Quickstart Antigo

Status: Superseded

## Resumo

Este apêndice continha um quickstart antigo para publicar `update-manifest.json` em GitHub Pages. Esse procedimento não representa mais o fluxo de release nem o mecanismo de auto-update do Assistente.

## Fluxo Vigente

O fluxo atual é:

1. criar ou publicar um GitHub Release com tag semântica;
2. deixar o workflow `.github/workflows/release.yml` gerar e anexar os assets ao release;
3. permitir que o aplicativo consulte `releases/latest` pela API do GitHub;
4. baixar o asset compatível com a plataforma.

Não crie branch `gh-pages`, não publique `update-manifest.json` e não use este arquivo como checklist operacional.

## Referências

- `0029-auto-update.md`;
- `docs/content/guias/RELEASE_QUICKSTART.md`;
- `.github/workflows/release.yml`;
- `internal/updater/updater.go`.
