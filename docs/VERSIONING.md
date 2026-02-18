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

Use o script PowerShell:

```powershell
# Build com versão personalizada
.\build-release.ps1 1.0.1

# Build sem versão (usa "dev")
wails build
```

### Build Manual com Versão

```bash
# Windows
wails build -ldflags "-X main.AppVersion=1.0.1"

# Linux
wails build -platform linux/amd64 -ldflags "-X main.AppVersion=1.0.1"
```

### GitHub Actions

A versão é **automaticamente** extraída da tag do release:

```bash
# Criar release (a versão vem da tag)
git tag v1.0.1
git push origin v1.0.1

# Criar release no GitHub
gh release create v1.0.1
```

O workflow automaticamente:
1. Extrai a versão da tag (remove 'v' se existir)
2. Injeta via ldflags: `-X main.AppVersion=1.0.1`
3. Gera binários com versão correta

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

### Backend (app.go)

```go
const (
    // AppVersion é sobrescrito em build time
    AppVersion = "dev"
)
```

### Build Time

```bash
go build -ldflags "-X main.AppVersion=1.0.1" .
```

Isso substitui o valor de `AppVersion` no binário compilado.

### GitHub Actions (.github/workflows/release.yml)

```yaml
- name: Build application
  run: |
    VERSION=${GITHUB_REF#refs/tags/}
    VERSION=${VERSION#v}
    LDFLAGS="-X main.AppVersion=$VERSION"
    wails build -ldflags "$LDFLAGS"
```

## Exemplo Completo

```powershell
# 1. Atualizar versão no código (opcional, será sobrescrito)
# app.go: const AppVersion = "dev"

# 2. Commit e tag
git add .
git commit -m "Prepare release v1.0.1"
git tag v1.0.1
git push origin main
git push origin v1.0.1

# 3. Criar release no GitHub
gh release create v1.0.1 --generate-notes

# 4. GitHub Actions automaticamente:
#    - Faz build com versão 1.0.1
#    - Faz upload dos binários
#    - Updater detectará a nova versão
```

## Verificando a Versão

### No aplicativo

Menu → Sobre → Ver versão instalada

### Via API

```go
import "assistente/main"

version := main.AppVersion
// Em dev: "dev"
// Em release: "1.0.1"
```

### No Frontend

```typescript
import { GetAppVersion } from '../../wailsjs/go/main/App';

const version = await GetAppVersion();
console.log('Versão:', version);
```
