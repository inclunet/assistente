---
title: "Release Quickstart"
weight: 4
---

# Release Rápido - GitHub Actions + Pages

## Setup Inicial (Uma Vez - 5 minutos)

### 1. Habilitar GitHub Pages
```
Settings → Pages → Source: main → /docs → Save
```

### 2. Habilitar Permissões do Actions
```
Settings → Actions → General → Workflow permissions
✅ Read and write permissions
✅ Allow GitHub Actions to create and approve pull requests
Save
```

### 3. Commitar Arquivos Iniciais
```bash
git add docs/ .github/workflows/
git commit -m "feat: setup auto-update with GitHub Actions"
git push
```

### 4. Verificar
Aguarde 2-3 minutos, depois:
```bash
curl https://inclunet.github.io/assistente/update-manifest.json
```

## ✅ Pronto! Agora Fazer Releases é Simples

## Release Nova Versão (30 segundos)

```bash
# 1. Atualizar versão em app.go
# const AppVersion = "1.0.1"

# 2. Commit
git add app.go
git commit -m "chore: bump version to 1.0.1"
git push

# 3. Criar e push tag
git tag v1.0.1
git push origin v1.0.1
```

**Pronto!** GitHub Actions vai:
- ✅ Buildar para Windows, macOS e Linux
- ✅ Publicar no GitHub Releases
- ✅ Atualizar site de downloads
- ✅ Atualizar manifest de auto-update

Aguarde ~10-15 minutos para builds completarem.

## Acompanhar Progresso

1. Vá em **Actions** no GitHub
2. Clique no workflow "Release"
3. Veja progresso dos builds

## URLs Úteis

Após release v1.0.1:

- **GitHub Release**: https://github.com/inclunet/assistente/releases/tag/v1.0.1
- **Site de Downloads**: https://inclunet.github.io/assistente/
- **Manifest**: https://inclunet.github.io/assistente/update-manifest.json

## Usuários Recebem Update

Automaticamente no próximo startup do app:
1. App detecta nova versão
2. Mostra questionário "Deseja atualizar?"
3. Download e aplica update
4. Notifica para reiniciar

## Problemas?

**Actions falhou?**
- Veja logs em Actions → Clique no job vermelho
- Problemas comuns: falta de permissões, erro no build

**Site não aparece?**
- Aguarde 2-5 minutos para GitHub Pages propagar
- Verifique em Settings → Pages se está ativo

**Auto-update não funciona?**
- Teste o manifest: `curl https://inclunet.github.io/assistente/update-manifest.json`
- Veja logs do app: procure "[Updater]"

## Documentação Completa

Ver: `docs/AUTO_UPDATE_GITHUB_ACTIONS.md`
