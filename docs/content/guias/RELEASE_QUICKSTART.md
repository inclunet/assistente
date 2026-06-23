---
title: "Release Quickstart"
weight: 4
---

# Release Rápido - GitHub Releases

## Setup Inicial (Uma Vez - 5 minutos)

### 1. Habilitar Permissões do Actions
```
Settings → Actions → General → Workflow permissions
✅ Read and write permissions
Save
```

### 2. Conferir workflow de release

O auto-update usa GitHub Releases. O workflow `.github/workflows/release.yml` roda quando um release é criado e anexa os assets ao próprio release.

### 3. Instalar GitHub CLI

```bash
gh auth login
```

Se preferir, o release também pode ser criado pela interface web do GitHub.

### 4. Commitar alterações da versão
```bash
git checkout main
git pull
git add .
git commit -m "chore: prepare release v1.0.1"
git push
```

## Pronto! Agora Fazer Releases é Simples

## Release Nova Versão (30 segundos)

```bash
# 1. Garanta que o release será criado a partir de main
git checkout main
git pull

# 2. Crie o release; o workflow será acionado por release.created
gh release create v1.0.1 --title "v1.0.1" --notes "Notas da versão"
```

GitHub Actions vai:
- Rodar testes backend e frontend
- Buildar CLI e app desktop
- Gerar checksums
- Anexar os assets ao GitHub Release criado

Aguarde ~10-15 minutos para builds completarem.

## Acompanhar Progresso

1. Vá em **Actions** no GitHub
2. Clique no workflow "Release"
3. Veja progresso dos builds

## URLs Úteis

Após release v1.0.1:

- **GitHub Release**: https://github.com/inclunet/assistente/releases/tag/v1.0.1
- **API usada pelo auto-update**: https://api.github.com/repos/inclunet/assistente/releases/latest

## Usuários Recebem Update

Automaticamente no próximo startup do app:
1. App detecta nova versão
2. Mostra questionário "Deseja atualizar?"
3. Baixa o asset compatível pelo GitHub Release
4. Aplica o update

## Problemas?

**Actions falhou?**
- Veja logs em Actions → Clique no job vermelho
- Problemas comuns: falta de permissões, erro no build

**Release ficou sem assets?**
- Veja o workflow "Release" em Actions
- Se o workflow falhar, o job de cleanup tenta remover o release/tag incompleto

**Auto-update não funciona?**
- Teste a API: `gh api repos/inclunet/assistente/releases/latest`
- Confira se os assets seguem os nomes esperados pelo updater, como `assistente-windows-amd64.exe` e `assistente-linux-amd64`
- Veja logs do app: procure "[Updater]"

## Documentação Completa

Ver: `aep/0029-auto-update.md`
