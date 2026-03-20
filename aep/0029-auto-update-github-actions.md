# Auto-Update com GitHub Actions + Pages

Sistema completo de release e distribuição automatizado.

## 🎯 Arquitetura

```
git tag v1.0.1
git push --tags
       ↓
┌──────────────────────┐
│  GitHub Actions      │
│  (CI/CD Pipeline)    │
│                      │
│  • Build Windows     │
│  • Build macOS       │
│  • Build Linux       │
│  • Gera instaladores │
│  • Calcula checksums │
└──────────────────────┘
       ↓
    ┌──────────────────────────┐
    │                          │
    ↓                          ↓
┌─────────────────┐   ┌─────────────────────┐
│ GitHub Releases │   │   GitHub Pages      │
│  (Executáveis)  │   │   (/docs folder)    │
│                 │   │                     │
│ • Binários      │   │ • Site download     │
│ • Para update   │   │ • Instaladores      │
│                 │   │ • Manifest JSON     │
└─────────────────┘   └─────────────────────┘
       ↓                          ↓
┌─────────────────┐   ┌─────────────────────┐
│  Auto-Update    │   │  Download Manual    │
│  (App detecta)  │   │  (Usuários visitam) │
└─────────────────┘   └─────────────────────┘
```

## 📦 Tipos de Arquivos

### GitHub Releases (API - Auto-update)
- `assistente-windows-amd64.exe` - Executável Windows
- `assistente-darwin-amd64` - Executável macOS Intel
- `assistente-darwin-arm64` - Executável macOS Apple Silicon
- `assistente-linux-amd64` - Executável Linux

### GitHub Pages (Web - Download manual)
- `docs/index.html` - Site de download
- `docs/update-manifest.json` - Manifest de versões
- `docs/releases/X.Y.Z/` - Instaladores:
  - `*-installer.exe` - Instalador Windows
  - `*.dmg` - Instalador macOS
  - `*.deb` - Pacote Debian/Ubuntu

## 🚀 Como Fazer Release

### Método 1: Via Tag (Recomendado)

```bash
# 1. Atualizar versão em app.go
# const AppVersion = "1.0.1"

# 2. Commit
git add app.go
git commit -m "chore: bump version to 1.0.1"

# 3. Criar tag
git tag v1.0.1

# 4. Push tag (trigger Actions)
git push origin main
git push origin v1.0.1
```

Actions vai automaticamente:
1. Buildar para todas as plataformas
2. Criar GitHub Release
3. Atualizar site no GitHub Pages
4. Atualizar manifest de auto-update

### Método 2: Via GitHub UI

1. Vá em **Releases** → **Draft a new release**
2. Crie tag `v1.0.1`
3. Clique em **Publish release**
4. Actions roda automaticamente

## 🔧 Configuração Inicial (Uma Vez)

### 1. Habilitar GitHub Pages

1. **Settings** → **Pages**
2. **Source**: Deploy from a branch
3. **Branch**: `main` → `/docs` → **Save**
4. Aguarde 2-3 minutos

### 2. Verificar Permissões

1. **Settings** → **Actions** → **General**
2. **Workflow permissions**: ✅ Read and write permissions
3. ✅ Allow GitHub Actions to create and approve pull requests
4. **Save**

### 3. Teste Inicial

```bash
# Criar manifest vazio
mkdir -p docs
echo '{"version":"1.0.0","released":"2026-02-16T00:00:00Z","notes":"","builds":{}}' > docs/update-manifest.json

# Commit
git add docs/
git commit -m "docs: init GitHub Pages"
git push

# Verificar
curl https://inclunet.github.io/assistente/update-manifest.json
```

## 📝 Workflow do Actions

### Triggers
- Push de tags `v*` (v1.0.0, v1.0.1, etc.)

### Jobs

#### 1. `build` (matriz de 4 jobs paralelos)
- Windows (amd64)
- macOS (amd64 + arm64)
- Linux (amd64)

Para cada plataforma:
1. Checkout código
2. Setup Go + Node.js
3. Instala Wails
4. Builda aplicativo
5. Prepara artefatos (executável + instaladores)
6. Calcula checksums SHA256
7. Upload como artefato

#### 2. `release` (após builds)
1. Download todos os artefatos
2. Gera `update-manifest.json` com:
   - Versão
   - Data
   - URLs dos binários
   - Checksums
   - Tamanhos
3. Cria GitHub Release com executáveis
4. Atualiza GitHub Pages:
   - Copia instaladores para `/docs/releases/X.Y.Z/`
   - Atualiza `update-manifest.json`
   - Commit e push para main

## 🌐 Site de Downloads

### Estrutura
```
docs/
├── index.html              # Página principal
├── update-manifest.json    # Manifest para auto-update
└── releases/
    ├── 1.0.0/
    │   ├── assistente-windows-amd64-installer.exe
    │   ├── assistente-darwin-universal.dmg
    │   └── assistente-linux-amd64.deb
    └── 1.0.1/
        └── ...
```

### Features do Site
- 🎨 Design moderno e responsivo
- 📥 Botões de download por plataforma
- 📝 Release notes automáticas
- 🔄 Carrega versão do manifest automaticamente
- 📱 Mobile-friendly

### URLs
- **Site**: https://inclunet.github.io/assistente/
- **Manifest**: https://inclunet.github.io/assistente/update-manifest.json
- **Releases**: https://inclunet.github.io/assistente/releases/X.Y.Z/

## 🔄 Como o Auto-Update Funciona

### No Startup
```go
// app.go - startup()
a.initUpdater()
go a.checkForUpdatesOnStartup()
```

1. Aguarda 5s após app iniciar
2. Faz GET em `update-manifest.json`
3. Compara `manifest.version` vs `AppVersion`
4. Se diferente → Mostra questionário
5. Usuário aceita → Download do GitHub Release
6. Verifica checksum SHA256
7. Aplica com `go-update` (rollback automático se falhar)
8. Notifica usuário para reiniciar

### Periodicamente
- Verifica a cada 6 horas (configurável)
- Mesma lógica do startup

### Manual (Frontend)
```javascript
// Frontend pode chamar:
await CheckForUpdates()  // Retorna UpdateInfo
await ApplyUpdate()      // Aplica update
await GetAppVersion()    // Versão atual
```

## 🔒 Segurança

### Checksums SHA256
- Todos os binários têm hash SHA256
- Verificado antes de aplicar
- Previne arquivos corrompidos/modificados

### HTTPS
- GitHub Pages/Releases usa HTTPS
- Previne man-in-the-middle

### Rollback Automático
- `go-update` salva binário antigo
- Se falhar, restaura automaticamente

### Token de Acesso (Opcional)
Se o repo for privado:
```go
// Em app.go
updater.SetGitHubToken(os.Getenv("GITHUB_TOKEN"))
```

## 🐛 Troubleshooting

### Actions falha no build

**Erro: Wails not found**
```yaml
# Adicionar no workflow:
- name: Install Wails
  run: go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

**Erro: Permission denied**
```yaml
# Verificar permissões em Settings → Actions
permissions:
  contents: write
```

### Site não atualiza

**Aguarde propagação**: GitHub Pages demora 2-5 minutos

**Cache do navegador**: Ctrl+Shift+R para forçar reload

**Verifique branch**: Settings → Pages → Deve ser `main` / `/docs`

### Auto-update não funciona

**Verifique manifest**:
```bash
curl https://inclunet.github.io/assistente/update-manifest.json
```

**Veja logs do app**: Procure por `[Updater]` nos logs

**Checksum inválido**: Re-builde e atualize manifest

## 📊 Customizações

### Mudar Intervalo de Verificação
```go
// internal/updater/updater.go
const CheckInterval = 12 * time.Hour  // De 6h para 12h
```

### Adicionar Release Notes Customizadas
```yaml
# .github/workflows/release.yml
- name: Generate manifest
  run: |
    # Adicionar notes do commit ou arquivo CHANGELOG
    NOTES=$(git log -1 --pretty=%B)
    # Usar $NOTES no JSON
```

### Build Apenas uma Plataforma
```yaml
# Comentar plataformas não desejadas na matriz
matrix:
  include:
    - os: windows-latest  # Só Windows
```

## 🎯 Melhorias Futuras

- [ ] Assinatura digital de binários (code signing)
- [ ] Delta updates (apenas diferenças)
- [ ] Canais beta/stable separados
- [ ] Estatísticas de downloads
- [ ] Notificação desktop quando update disponível
- [ ] Barra de progresso no download
- [ ] Auto-restart após update

## 📚 Referências

- [GitHub Actions](https://docs.github.com/en/actions)
- [GitHub Pages](https://docs.github.com/en/pages)
- [GitHub Releases](https://docs.github.com/en/repositories/releasing-projects-on-github)
- [Wails Build](https://wails.io/docs/guides/building)
- [go-update](https://github.com/inconshreveable/go-update)
