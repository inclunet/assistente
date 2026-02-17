# Auto-Update - Guia Rápido

## Setup Inicial (Uma Vez)

### 1. Configurar GitHub Pages

```bash
# Criar branch gh-pages
git checkout --orphan gh-pages
git rm -rf .

# Estrutura inicial
mkdir -p releases
echo '{"version":"1.0.0","released":"2026-02-16T00:00:00Z","notes":"Inicial","builds":{}}' > update-manifest.json

git add .
git commit -m "Init GitHub Pages"
git push origin gh-pages

git checkout main
```

Vá em **Settings** → **Pages** → **Branch: gh-pages** → **Save**

### 2. Verificar

Aguarde 2-3 minutos, depois teste:
```bash
curl https://inclunet.github.io/assistente/update-manifest.json
```

## Release Nova Versão

### Windows (Local)

```powershell
# 1. Atualizar versão em app.go
# const AppVersion = "1.0.1"

# 2. Build
wails build

# 3. Gerar release
.\build-release.ps1 1.0.1

# 4. Deploy para gh-pages
git checkout gh-pages
git add releases/1.0.1 update-manifest.json
git commit -m "Release v1.0.1"
git push origin gh-pages
git checkout main
```

### macOS (Se tiver Mac)

```bash
# Build para Mac
wails build -platform darwin/amd64
wails build -platform darwin/arm64

# Copiar para releases/1.0.1/
# Atualizar checksums no manifest
```

### Linux (Se tiver Linux)

```bash
# Build para Linux
wails build -platform linux/amd64

# Copiar para releases/1.0.1/
# Atualizar checksums no manifest
```

## Como Funciona

1. **App inicia** → Aguarda 5s → Verifica atualizações
2. **Nova versão?** → Pergunta via questionário
3. **Usuário aceita?** → Download + Verifica checksum + Aplica
4. **Sucesso** → Notifica usuário para reiniciar

## Arquivos Importantes

- `internal/updater/updater.go` - Lógica de update
- `app.go` - Integração com app (métodos CheckForUpdates, ApplyUpdate)
- `build-release.ps1` - Script de build e release
- `docs/AUTO_UPDATE.md` - Documentação completa

## URLs

- **Manifest**: https://inclunet.github.io/assistente/update-manifest.json
- **Releases**: https://inclunet.github.io/assistente/releases/X.Y.Z/

## Troubleshooting

**Build falha?**
- Verifique se Wails está instalado: `wails doctor`

**Update não funciona?**
- Verifique manifest no navegador (URL acima)
- Veja logs do app: "Updater"

**Checksum inválido?**
- Recalcule: `Get-FileHash -Algorithm SHA256 arquivo.exe`
- Atualize o manifest com hash correto

## Próximos Passos

1. Configure GitHub Actions para build automático
2. Adicione UI no frontend para check manual
3. Implemente barra de progresso no download

Ver `docs/AUTO_UPDATE.md` para detalhes completos.
