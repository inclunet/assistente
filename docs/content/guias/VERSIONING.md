---
title: "Versionamento"
weight: 5
---

# Sistema de Versionamento

## Versão do Aplicativo

A versão é injetada em **build time** através de ldflags.

### Desenvolvimento Local

Por padrão, a versão é `"dev"`:

```bash
wails dev
# ou
go run .
```

### Build com Versão Específica

```powershell
wails build -ldflags "-X assistente/internal/app.AppVersion=1.0.1"
```

Para uma plataforma específica:

```bash
wails build -platform linux/amd64 -ldflags "-X assistente/internal/app.AppVersion=1.0.1"
```

### GitHub Actions

A versão é **automaticamente** extraída da tag do release:

```bash
gh release create v1.0.1 --target main --title "v1.0.1" --notes "Notas da versão"
```

O workflow automaticamente:
1. Extrai a versão da tag (remove 'v' se existir)
2. Injeta no app desktop: `-X assistente/internal/app.AppVersion=1.0.1`
3. Gera binários com versão correta

A CLI possui sua própria variável em `cmd/asst/main.go`; somente o build da
CLI usa `-X main.AppVersion=1.0.1`.

## Verificação de Checksum

O updater **não requer checksums** obrigatoriamente. Se o campo `Checksum` estiver vazio no manifest, a verificação é pulada com um warning no log.

### Futuro: Adicionar Checksums

Para adicionar verificação de checksums ao workflow:

1. No step de prepare artifacts, gerar checksums
2. Criar um arquivo `checksums.txt` com formato:
   ```
   sha256:abc123... assistente-windows-amd64.exe
   sha256:def456... assistente-linux-amd64
   ```
3. Modificar o updater para buscar e parsear `checksums.txt`

## Como Funciona

### Backend desktop (`internal/app/app_updater.go`)

```go
// AppVersion é sobrescrita em build time.
var AppVersion = "dev"
```

### Build Time

```bash
wails build -ldflags "-X assistente/internal/app.AppVersion=1.0.1"
```

Isso substitui o valor de `AppVersion` no binário compilado.

### GitHub Actions (.github/workflows/release.yml)

```yaml
- name: Build application
  run: |
    VERSION=${GITHUB_REF#refs/tags/}
    VERSION=${VERSION#v}
    LDFLAGS="-X assistente/internal/app.AppVersion=$VERSION"
    wails build -ldflags "$LDFLAGS"
```

## Exemplo Completo

```powershell
# 1. Garanta que as mudanças já estejam na main
git checkout main
git pull

# 2. Crie o GitHub Release
gh release create v1.0.1 --target main --generate-notes

# 3. GitHub Actions automaticamente:
#    - Faz build com versão 1.0.1
#    - Faz upload dos binários
#    - Updater detectará a nova versão
```

## Verificando a Versão

### No aplicativo

Menu → Sobre → Ver versão instalada

### No backend

```go
import "assistente/internal/app"

version := app.AppVersion
// Em dev: "dev"
// Em release: "1.0.1"
```

### No Frontend

```typescript
import { GetAppVersion } from '../../wailsjs/go/app/App';

const version = await GetAppVersion();
```
