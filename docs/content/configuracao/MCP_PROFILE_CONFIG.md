---
title: "MCP — Perfil"
weight: 4
---

# Configurando Modo MCP em Perfis

## 🎯 Visão Geral

Você pode configurar o **modo MCP** diretamente no perfil de conversa. Isso permite que diferentes perfis usem diferentes estratégias de MCP.

---

## 📋 Campo de Configuração

### Localização
```json
{
  "chat": {
    "mcp_mode": "adapter"  // ← AQUI
  }
}
```

### Valores Possíveis

| Valor | Descrição | Quando Usar |
|-------|-----------|-------------|
| `"adapter"` | **Padrão.** Tools MCP via bridge | Qualquer modelo (GPT, Claude, Gemini, etc.) |
| `"native"` | MCP direto (sem bridge) | Claude 3.7+, modelos com suporte MCP |
| `"auto"` | Detecta automaticamente | Deixa o sistema decidir baseado no modelo |

---

## 🔧 Exemplos de Uso

### Exemplo 1: Modo Adapter (Padrão)
**Use para:** Compatibilidade universal

```json
{
  "name": "GPT-4 com MCP",
  "chat": {
    "model": "gpt-4o",
    "mcp_mode": "adapter"
  }
}
```

✅ **Funciona com:** Qualquer modelo  
✅ **Vantagens:** Máxima compatibilidade, controle total  
⚠️ **Desvantagens:** Uma camada extra de abstração

---

### Exemplo 2: Modo Nativo (Claude)
**Use para:** Máxima performance com Claude

```json
{
  "name": "Claude Native MCP",
  "chat": {
    "model": "claude-3-7-sonnet-20250219",
    "mcp_mode": "native"
  }
}
```

✅ **Funciona com:** Claude 3.7+, modelos com suporte MCP  
✅ **Vantagens:** Mais rápido, streaming direto, recursos completos  
⚠️ **Desvantagens:** Requer modelo compatível

---

### Exemplo 3: Modo Auto (Recomendado)
**Use para:** Flexibilidade sem configuração manual

```json
{
  "name": "Smart Profile",
  "chat": {
    "model": "claude-3-7-sonnet-20250219",
    "mcp_mode": "auto"
  }
}
```

✅ **Funciona com:** Qualquer modelo  
✅ **Vantagens:** Otimiza automaticamente  
✅ **Comportamento:**
- Claude 3.7+ → Usa modo nativo
- Outros modelos → Usa adapter

---

## 🧪 Testando a Configuração

### 1. Via Frontend (futuro)
```typescript
// Ao criar/editar perfil
interface ChatConfig {
  model: string;
  mcp_mode?: "adapter" | "native" | "auto";
  // ...
}
```

### 2. Via JSON Manual
```bash
# Editar perfil
notepad ~/.assistente/profiles/meu-perfil.json

# Adicionar/modificar:
{
  "chat": {
    "mcp_mode": "native"
  }
}
```

### 3. Verificar no Código
```go
// O perfil tem métodos helper:
profile := ... // carregar perfil

// Verificar modo efetivo
mode := profile.GetMCPMode() // "adapter", "native", "auto"

// Verificar se deve usar nativo
useNative := profile.ShouldUseMCPNative() // true/false

// Verificar se modelo suporta
supported := profiles.ModelSupportsNativeMCP("claude-3-7-sonnet-20250219") // true
```

---

## 📊 Como Funciona

### Detecção Automática

O sistema verifica o modelo configurado:

```go
func ModelSupportsNativeMCP(modelID string) bool {
    // Claude 3.7+
    if strings.Contains(model, "claude-3-7") ||
       strings.Contains(model, "claude-3.7") {
        return true
    }
    
    // Claude 4+ (futuro)
    if strings.Contains(model, "claude-4") {
        return true
    }
    
    // Adicionar outros modelos conforme ganham suporte
    return false
}
```

### Lógica de Decisão

```
mcp_mode = "adapter"  → Sempre usa bridge
mcp_mode = "native"   → Sempre usa nativo (pode falhar se modelo não suportar)
mcp_mode = "auto"     → Detecta automaticamente baseado no modelo
mcp_mode = ""         → Default: adapter
```

---

## 🎨 Exemplos Práticos

### Perfil Multi-Modelo (Auto)
```json
{
  "name": "Universal",
  "description": "Funciona com qualquer modelo, otimiza automaticamente",
  "chat": {
    "model": "",  // Será escolhido na UI
    "mcp_mode": "auto"  // Adapta ao modelo escolhido
  }
}
```

**Comportamento:**
- Se escolher Claude 3.7 → Modo nativo ⚡
- Se escolher GPT-4o → Modo adapter ✅
- Se escolher Gemini → Modo adapter ✅

---

### Perfil Development (Adapter)
```json
{
  "name": "Development",
  "description": "Para testes e debugging",
  "chat": {
    "model": "gpt-4o",
    "mcp_mode": "adapter"  // Controle total para debugging
  }
}
```

**Vantagens:**
- Logs detalhados de cada chamada MCP
- Pode adicionar breakpoints no bridge
- Facilita auditoria e debugging

---

### Perfil Production (Native)
```json
{
  "name": "Production - Claude",
  "description": "Máxima performance para produção",
  "chat": {
    "model": "claude-3-7-sonnet-20250219",
    "mcp_mode": "native"  // Performance otimizada
  }
}
```

**Vantagens:**
- Latência mínima
- Streaming direto do MCP para o modelo
- Usa resources/prompts nativamente

---

## 🔄 Migração de Perfis Existentes

### Opção 1: Adicionar campo manualmente
```bash
# Editar cada perfil
nano ~/.assistente/profiles/*.json

# Adicionar:
"mcp_mode": "auto"  # Recomendado
```

### Opção 2: Script de migração
```bash
# PowerShell
Get-ChildItem ~/.assistente/profiles/*.json | ForEach-Object {
    $content = Get-Content $_ | ConvertFrom-Json
    if (-not $content.chat.mcp_mode) {
        $content.chat | Add-Member -NotePropertyName "mcp_mode" -NotePropertyValue "auto"
        $content | ConvertTo-Json -Depth 10 | Set-Content $_
    }
}
```

### Opção 3: Deixar vazio (default = adapter)
**Não fazer nada** - o sistema usa "adapter" como padrão se o campo não existir.

---

## 📈 Métricas e Observação

### Frontend (futuro)
Adicionar indicador visual do modo ativo:

```typescript
<div className="mcp-mode-indicator">
  {profile.mcp_mode === "native" && <Badge color="green">MCP Nativo ⚡</Badge>}
  {profile.mcp_mode === "adapter" && <Badge color="blue">MCP Adapter ✓</Badge>}
  {profile.mcp_mode === "auto" && <Badge color="purple">MCP Auto 🎯</Badge>}
</div>
```

### Logs
```
[MCP] Perfil 'Claude Native MCP': modo=native, modelo=claude-3-7-sonnet
[MCP] Usando MCP nativo: 3 servidores disponíveis
[MCP]   - filesystem (tools: 12, resources: 5)
[MCP]   - github (tools: 8, resources: 3, prompts: 2)
[MCP]   - database (tools: 6)
```

---

## ⚙️ Configuração Avançada

### Por Workspace
Você pode ter perfis diferentes por projeto:

```
~/projects/webapp/.assistente/profiles/
  ├── frontend-dev.json    (mcp_mode: adapter, model: gpt-4o)
  └── backend-dev.json     (mcp_mode: native, model: claude-3-7)

~/projects/api/.assistente/profiles/
  └── api-assist.json      (mcp_mode: auto)
```

### Por Ambiente
```json
// development.json
{
  "chat": {
    "model": "gpt-4o-mini",
    "mcp_mode": "adapter"  // Debugging
  }
}

// production.json
{
  "chat": {
    "model": "claude-3-7-sonnet-20250219",
    "mcp_mode": "native"  // Performance
  }
}
```

---

## 🐛 Troubleshooting

### Erro: "modelo não suporta MCP nativo"
```json
// Mudar de:
"mcp_mode": "native"

// Para:
"mcp_mode": "auto"  // ou "adapter"
```

### Modo nativo não está funcionando
1. Verificar se modelo é compatível (Claude 3.7+)
2. Verificar se MCP servers estão conectados
3. Verificar logs para mensagens de erro
4. Testar com modo "adapter" primeiro

### Performance não melhorou
- Modo nativo só faz diferença com múltiplas chamadas MCP
- Para uso básico, adapter é suficiente
- Benefício maior é em streaming e subscriptions

---

## 📚 Referências

- [`internal/profiles/types.go`](../internal/profiles/types.go) - Definição do campo
- [`docs/MCP_NATIVE_MODE.md`](./MCP_NATIVE_MODE.md) - Documentação completa do modo nativo
- [`docs/example-profile-claude-native.json`](./example-profile-claude-native.json) - Exemplo de perfil

---

## ✅ Checklist de Implementação

Backend:
- [x] Campo `mcp_mode` em ChatConfig
- [x] Constantes MCPModeAdapter, MCPModeNative, MCPModeAuto
- [x] Validação do campo
- [x] Método `GetMCPMode()`
- [x] Método `ShouldUseMCPNative()`
- [x] Função `ModelSupportsNativeMCP()`

Frontend (TODO):
- [ ] Dropdown/Radio para selecionar modo MCP
- [ ] Badge visual do modo ativo
- [ ] Tooltip explicativo
- [ ] Warning se modo native com modelo incompatível

Documentação:
- [x] Este arquivo
- [x] Exemplo de perfil
- [x] Atualização do MCP_NATIVE_MODE.md

---

## 🎉 Conclusão

Agora você pode configurar o modo MCP **diretamente no perfil**! 

**Recomendações:**
- 🆕 **Novos perfis:** Use `"mcp_mode": "auto"` 
- 🔧 **Development:** Use `"adapter"` para debugging
- ⚡ **Production (Claude):** Use `"native"` para performance

**O sistema escolhe automaticamente a melhor estratégia baseado no modelo!** 🚀
