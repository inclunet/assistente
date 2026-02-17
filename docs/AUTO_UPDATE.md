# Sistema de Auto-Update

Sistema de atualização automática do Assistente usando GitHub Pages e go-update.

## Arquitetura

```
┌─────────────────┐
│  Aplicativo     │
│  (Go + Wails)   │
└────────┬────────┘
         │ Verifica periodicamente
         ↓
┌─────────────────────────────────┐
│  GitHub Pages                   │
│  https://inclunet.github.io/    │
│       assistente/               │
│                                 │
│  ├── update-manifest.json       │ ← Metadados de versões
│  └── releases/                  │
│      ├── 1.0.0/                 │
│      │   ├── assistente-*.exe   │ ← Binários
│      │   └── assistente-*       │
│      └── 1.0.1/                 │
└─────────────────────────────────┘
```

## Componentes

### 1. Backend (Go)

**`internal/updater/updater.go`**
- `CheckForUpdates()`: Verifica se há nova versão
- `ApplyUpdate()`: Baixa e aplica atualização
- Usa `go-update` para substituir binário
- Verifica checksums SHA256

**`app.go`**
- Inicializa updater no startup
- Verifica atualizações automaticamente (6h)
- Usa sistema de questionário para perguntar ao usuário
- Métodos exportados para frontend via Wails

### 2. GitHub Pages

**Branch `gh-pages`:**
```
/
├── update-manifest.json        # Manifest principal
├── releases/
│   ├── 1.0.0/
│   │   ├── assistente-windows-amd64.exe
│   │   ├── assistente-darwin-amd64
│   │   ├── assistente-darwin-arm64
│   │   └── assistente-linux-amd64
│   └── 1.0.1/
│       └── ...
└── index.html (opcional)
```

**Manifest Format (`update-manifest.json`):**
```json
{
  "version": "1.0.1",
  "released": "2026-02-16T10:00:00Z",
  "notes": "- Nova funcionalidade X\n- Correção de bug Y",
  "builds": {
    "windows-amd64": {
      "url": "https://inclunet.github.io/assistente/releases/1.0.1/assistente-windows-amd64.exe",
      "checksum": "sha256:abc123...",
      "size": 50000000
    },
    "darwin-amd64": { ... },
    "darwin-arm64": { ... },
    "linux-amd64": { ... }
  }
}
```

### 3. Build & Release

**Script: `build-release.ps1`**
```powershell
.\build-release.ps1 1.0.1
```

**Processo:**
1. Builda binários para todas as plataformas
2. Calcula checksums SHA256
3. Gera `update-manifest.json`
4. Organiza em `releases/<version>/`

## Fluxo de Atualização

### Automático (Startup)

```
App Startup
    ↓
Aguarda 5s
    ↓
Verifica updates (background)
    ↓
┌─ Nova versão? ─┐
│      Não       │ → Log e continua
└────────────────┘
         │ Sim
         ↓
Questionário ao usuário
    ↓
┌─ Confirma? ─┐
│     Não     │ → Cancela
└─────────────┘
      │ Sim
      ↓
Download binário
      ↓
Verifica checksum
      ↓
Aplica update
      ↓
Notifica usuário
   (reiniciar)
```

### Manual (Frontend)

Frontend pode chamar:
- `CheckForUpdates()`: Verifica manualmente
- `ApplyUpdate()`: Aplica atualização
- `GetAppVersion()`: Obtém versão atual

## Configuração do GitHub Pages

### Passo 1: Criar branch gh-pages

```bash
# Cria branch órfã (sem histórico)
git checkout --orphan gh-pages
git rm -rf .

# Cria estrutura inicial
mkdir -p releases/1.0.0
echo '{"version":"1.0.0","released":"2026-02-16T00:00:00Z","notes":"Versão inicial","builds":{}}' > update-manifest.json
echo '<h1>Assistente Updates</h1>' > index.html

# Commit inicial
git add .
git commit -m "Inicializa GitHub Pages para updates"
git push origin gh-pages
```

### Passo 2: Configurar GitHub Pages

1. Vá em **Settings** → **Pages**
2. Source: **Deploy from a branch**
3. Branch: **gh-pages** / **(root)**
4. Save

Aguarde alguns minutos. O site estará disponível em:
```
https://inclunet.github.io/assistente/
```

### Passo 3: Verificar

```bash
curl https://inclunet.github.io/assistente/update-manifest.json
```

## Processo de Release

### 1. Atualizar Versão

**Em `app.go`:**
```go
const AppVersion = "1.0.1"  // Incrementa versão
```

### 2. Build & Test

```bash
# Testa build local
wails build

# Executa testes
go test ./...
```

### 3. Gerar Release

```powershell
# Gera binários e manifest
.\build-release.ps1 1.0.1

# Revisa arquivos gerados
ls releases/1.0.1/
cat update-manifest.json
```

### 4. Cross-compile (Outras Plataformas)

**macOS (em macOS):**
```bash
wails build -platform darwin/amd64
wails build -platform darwin/arm64
```

**Linux (em Linux):**
```bash
wails build -platform linux/amd64
```

Ou use GitHub Actions (recomendado).

### 5. Calcular Checksums

```powershell
# Windows
Get-FileHash -Algorithm SHA256 releases/1.0.1/assistente-windows-amd64.exe

# Linux/Mac
shasum -a 256 releases/1.0.1/assistente-darwin-amd64
```

Atualize o manifest com os hashes reais.

### 6. Deploy para GitHub Pages

```bash
# Muda para branch gh-pages
git checkout gh-pages

# Copia arquivos de release
cp -r releases/1.0.1/ releases/
cp update-manifest.json .

# Commit e push
git add releases/1.0.1 update-manifest.json
git commit -m "Release v1.0.1"
git push origin gh-pages

# Volta para main
git checkout main
```

### 7. Tag na branch main (Opcional)

```bash
git tag v1.0.1
git push origin v1.0.1
```

## Segurança

### Checksums SHA256
- Verifica integridade do download
- Previne binários corrompidos/modificados

### HTTPS
- GitHub Pages serve via HTTPS automático
- Previne ataques man-in-the-middle

### Repositório Privado
- Binários servidos via GitHub Pages (público)
- Código-fonte permanece privado

### Rollback Automático
- `go-update` faz rollback se a atualização falhar
- Binário original é preservado

## Limitações & Considerações

### Repositório Privado
✅ **Funciona**: GitHub Pages pode ser público mesmo com repo privado
- Configure em Settings → Pages → Visibility

### Tamanho dos Binários
- GitHub Pages tem limite de 1GB por repositório
- Mantenha apenas últimas 3-5 versões
- Binários Wails: ~40-60MB cada

### Plataformas
- **Windows**: Funciona perfeitamente
- **macOS**: Requer codesigning para distribuição
- **Linux**: Funciona, mas pode precisar de AppImage

### Permissões
- App precisa de permissão para substituir seu próprio binário
- No Windows: pode precisar de elevação UAC
- No macOS/Linux: depende de onde está instalado

## Troubleshooting

### "Update failed: permission denied"
- **Windows**: Execute como Administrador
- **Mac/Linux**: `chmod +x` no binário ou instale em ~/Applications

### "Checksum mismatch"
- Binário corrompido durante upload/download
- Recalcule e atualize o manifest
- Verifique se o hash no manifest está correto

### "Cannot fetch manifest"
- GitHub Pages ainda está propagando (aguarde 5-10min)
- Verifique URL: `curl https://inclunet.github.io/assistente/update-manifest.json`
- Confirme que branch gh-pages está publicada

### Build falha em outras plataformas
- Wails requer toolchain nativa para cross-compile
- Use GitHub Actions ou máquinas virtuais
- Alternativamente, compile em cada plataforma

## GitHub Actions (CI/CD Automatizado)

**`.github/workflows/release.yml`** (exemplo):

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

jobs:
  release:
    strategy:
      matrix:
        include:
          - os: windows-latest
            platform: windows/amd64
          - os: macos-latest
            platform: darwin/universal
          - os: ubuntu-latest
            platform: linux/amd64
    
    runs-on: ${{ matrix.os }}
    
    steps:
      - uses: actions/checkout@v3
      
      - uses: actions/setup-go@v4
        with:
          go-version: '1.24'
      
      - name: Install Wails
        run: go install github.com/wailsapp/wails/v2/cmd/wails@latest
      
      - name: Build
        run: wails build -platform ${{ matrix.platform }}
      
      - name: Calculate Checksum
        id: checksum
        run: |
          # Calcula SHA256 do binário gerado
          # Varia por plataforma
      
      - name: Upload to gh-pages
        # Faz deploy para branch gh-pages
```

## Melhorias Futuras

### Curto Prazo
- [ ] UI no frontend para verificação manual
- [ ] Barra de progresso durante download
- [ ] Notificação desktop quando update estiver pronto

### Médio Prazo
- [ ] Delta updates (apenas diferenças)
- [ ] Assinatura digital dos binários
- [ ] Update silencioso em background
- [ ] Rollback manual para versão anterior

### Longo Prazo
- [ ] Auto-restart após update
- [ ] Canal beta/stable separado
- [ ] Estatísticas de adoção de versões
- [ ] A/B testing de features

## Referências

- [go-update](https://github.com/inconshreveable/go-update)
- [GitHub Pages](https://docs.github.com/en/pages)
- [Wails Build](https://wails.io/docs/guides/building)
- [Semantic Versioning](https://semver.org/)
