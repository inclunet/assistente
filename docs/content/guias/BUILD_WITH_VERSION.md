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
wails build -ldflags "-X main.AppVersion=1.0.1"
```

Versão exibida no app: **1.0.1**

---

## Build de Produção (GitHub Actions)

O workflow `.github/workflows/release.yml` injeta automaticamente a versão da tag:

1. **Criar tag:**
   ```bash
   git tag v1.0.1
   git push origin v1.0.1
   ```

2. **Criar release:**
   ```bash
   gh release create v1.0.1 --title "v1.0.1" --notes "Release notes aqui"
   ```

3. **GitHub Actions** irá:
   - Extrair versão da tag (`v1.0.1` → `1.0.1`)
   - Compilar com `wails build -ldflags "-X main.AppVersion=1.0.1"`
   - Criar executáveis Windows/Linux
   - Criar instalador Windows (NSIS)
   - Fazer upload dos artifacts

---

## Como Funciona

### No Código (app.go)

```go
var (
    // AppVersion é a versão do aplicativo
    // Em dev: permanece como "dev"
    // Em produção: injetada via ldflags durante build
    AppVersion = "dev"
)
```

### No Workflow (release.yml)

```yaml
# Extrai versão da tag
VERSION=${GITHUB_REF#refs/tags/v}

# Injeta via ldflags
wails build -ldflags "-X main.AppVersion=$VERSION"
```

### No Runtime

- `GetAppVersion()` retorna o valor de `AppVersion`
- UpdatePage mostra "Versão atual: X.X.X"
- Updater compara com versão do GitHub

---

## Testando Sistema de Atualização Localmente

1. **Build com versão antiga:**
   ```powershell
   wails build -ldflags "-X main.AppVersion=1.0.0"
   ```

2. **Criar release no GitHub com versão nova:**
   ```bash
   gh release create v1.0.1
   ```

3. **Executar o build local (v1.0.0)**

4. **Ir em "Sobre" → Verificar Atualizações**

5. **Sistema detectará v1.0.1 disponível**

---

## Variáveis Disponíveis para Injeção

Você pode injetar múltiplas variáveis:

```powershell
wails build -ldflags "-X main.AppVersion=1.0.1 -X main.BuildTime=$(date -u +%Y-%m-%d_%H:%M:%S)"
```

Exemplo no código:

```go
var (
    AppVersion = "dev"
    BuildTime  = "unknown"
)
```

---

## Importante

- ❌ **NÃO** tente ler `wails.json` em runtime (não é empacotado)
- ✅ **USE** ldflags para injetar variáveis em tempo de compilação
- ✅ Em dev, `AppVersion` sempre será `"dev"`
- ✅ Em produção, GitHub Actions injeta automaticamente
