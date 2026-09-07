---
title: "Build com Versão"
weight: 1
---

# Como Fazer Build com Versão Personalizada

## Versão em Desenvolvimento

Por padrão, builds locais terão versão **"dev"**:

```powershell
wails build
```

Versão exibida no app: **dev**

---

## Versão Personalizada (Teste Local)

Para testar com uma versão específica localmente, use **ldflags**:

```powershell
wails build -ldflags "-X assistente/internal/app.AppVersion=1.0.1"
```

Versão exibida no app: **1.0.1**

---

## Build de Produção (GitHub Actions)

O workflow `.github/workflows/release.yml` injeta automaticamente a versão da tag do GitHub Release:

1. **Criar o release a partir da `main`:**
   ```bash
   gh release create v1.0.1 --target main --title "v1.0.1" --notes "Notas da versão"
   ```

2. **GitHub Actions** irá:
   - Extrair versão da tag (`v1.0.1` → `1.0.1`)
   - Compilar o app desktop com `wails build -ldflags "-X assistente/internal/app.AppVersion=1.0.1"`
   - Criar executáveis Windows/Linux
   - Criar instalador Windows (NSIS)
   - Anexar os artefatos ao release

---

## Como Funciona

### No código (`internal/app/app_updater.go`)

```go
// AppVersion é a versão do aplicativo, injetada via ldflags no build.
// Em dev, permanece como "dev".
var AppVersion = "dev"
```

### No Workflow (release.yml)

```yaml
# Extrai versão da tag
VERSION=${GITHUB_REF#refs/tags/}
VERSION=${VERSION#v}

# Injeta via ldflags
LDFLAGS="-X assistente/internal/app.AppVersion=$VERSION"
wails build -ldflags "$LDFLAGS"
```

O executável da CLI é outro pacote. Seu build continua usando
`-X main.AppVersion=$VERSION`, que aponta para `cmd/asst/main.go`.

### No Runtime

- `GetAppVersion()` retorna o valor de `AppVersion`
- UpdatePage mostra "Versão atual: X.X.X"
- Updater compara com versão do GitHub

---

## Testando Sistema de Atualização Localmente

1. **Build com versão antiga:**
   ```powershell
   wails build -ldflags "-X assistente/internal/app.AppVersion=1.0.0"
   ```

2. **Criar release no GitHub com versão nova:**
   ```bash
   gh release create v1.0.1 --target main --generate-notes
   ```

3. **Executar o build local (v1.0.0)**

4. **Ir em "Sobre" → Verificar Atualizações**

5. **Sistema detectará v1.0.1 disponível**

---

## Importante

- ❌ **NÃO** tente ler `wails.json` em runtime (não é empacotado)
- ✅ **USE** o caminho completo do pacote Go no ldflag do app desktop
- ✅ **NÃO** reutilize `main.AppVersion` do executável da CLI no build Wails
- ✅ Em dev, `AppVersion` sempre será `"dev"`
- ✅ Em produção, GitHub Actions injeta automaticamente
