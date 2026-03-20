# Modo MCP Nativo

## O que é?

O Assistente agora suporta **dois modos** de usar servidores MCP (Model Context Protocol):

### 1. **Modo Adapter (Atual - Padrão)**
- O modelo usa function calling tradicional
- O Assistente faz bridge das chamadas para servidores MCP
- **Compatível com qualquer modelo** (GPT, Claude, Gemini, LLaMA, etc.)

### 2. **Modo Nativo (Novo)**
- O modelo acessa servidores MCP **diretamente**
- Requer que o modelo tenha suporte MCP nativo
- **Mais eficiente** - remove camada intermediária

---

## Modelos com Suporte MCP Nativo

### Claude (Anthropic API)
```typescript
// Claude suporta MCP via prompt do sistema
const response = await anthropic.messages.create({
  model: "claude-3-7-sonnet-20250219",
  mcp_servers: getAssistenteNativeServers(), // ← nossa função
  messages: [...]
});
```

### Outros modelos
- Atualmente, apenas Claude tem suporte oficial
- Outros vendors podem adicionar no futuro

---

## Como Usar Modo Nativo

### 1. No código Go (Backend)

```go
// Obter informações dos servidores MCP para passar ao modelo
nativeServers := app.GetNativeMCPServers()

// Resultado: []map[string]any{
//   {
//     "slug": "filesystem",
//     "name": "Filesystem MCP Server",
//     "description": "Acesso a arquivos",
//     "transport": "stdio",
//     "sessionId": "abc123",
//     "capabilities": {
//       "tools": true,
//       "resources": true,
//       "prompts": false
//     }
//   },
//   ...
// }
```

### 2. Passando para Claude

```go
import anthropic "github.com/anthropics/anthropic-sdk-go"

// Converter formato do Assistente para formato Claude
mcpServers := convertToClaudeMCPFormat(nativeServers)

client := anthropic.NewClient(...)
response, err := client.Messages.New(ctx, anthropic.MessageNewParams{
    Model: "claude-3-7-sonnet-20250219",
    MCPServers: mcpServers, // ← servidores MCP diretos
    Messages: []anthropic.MessageParam{...},
})
```

### 3. Claude usa MCP diretamente

O modelo agora pode:
- Listar tools/resources dos servidores
- Chamar tools diretamente
- Ler resources
- Executar prompts

**Sem passar pelo nosso bridge!**

---

## Vantagens do Modo Nativo

### ✅ **Performance**
- Remove hop intermediário
- Latência menor
- Menos serialização/deserialização

### ✅ **Recursos Completos**
- Acesso a resources MCP (arquivos, dados)
- Execução de prompts
- Subscriptions em tempo real

### ✅ **Streaming**
- Claude pode fazer streaming direto do servidor MCP
- Melhor experiência do usuário

---

## Quando Usar Cada Modo

### Use **Modo Adapter** quando:
- Modelo não suporta MCP nativamente (maioria dos casos)
- Quer controle total sobre chamadas MCP
- Precisa adicionar autorização/auditoria

### Use **Modo Nativo** quando:
- Modelo suporta MCP (ex: Claude via API)
- Quer máxima performance
- Quer usar recursos avançados (streaming, subscriptions)

---

## Implementação Atual

### Estrutura

```
┌─────────────────────────────────────────────────────┐
│                    Assistente                       │
│                                                     │
│  ┌──────────────┐         ┌──────────────┐        │
│  │   Modo       │         │    Modo      │        │
│  │   Adapter    │         │   Nativo     │        │
│  │              │         │              │        │
│  │  Model       │         │  Model       │        │
│  │    ↓         │         │    ↓         │        │
│  │  Bridge      │         │  Direto ao   │        │
│  │    ↓         │         │  MCP Server  │        │
│  │  MCP Server  │         │              │        │
│  └──────────────┘         └──────────────┘        │
└─────────────────────────────────────────────────────┘
```

### Métodos Disponíveis

```go
// Backend (app.go)
func (a *App) GetNativeMCPServers() []map[string]any

// Frontend (TypeScript)
const servers = await GetNativeMCPServers();
```

---

## Exemplo Completo

### Scenario: Chat com Claude usando MCP Nativo

```go
// 1. Obter servidores MCP ativos
nativeServers := app.GetNativeMCPServers()

// 2. Converter para formato Claude
claudeMCP := make([]anthropic.MCPServer, len(nativeServers))
for i, srv := range nativeServers {
    claudeMCP[i] = anthropic.MCPServer{
        Name:        srv["name"].(string),
        Endpoint:    srv["endpoint"].(string), // se SSE
        SessionID:   srv["sessionId"].(string),
        Capabilities: srv["capabilities"].(map[string]any),
    }
}

// 3. Criar mensagem com MCP nativo
response, err := anthropicClient.Messages.New(ctx, 
    anthropic.MessageNewParams{
        Model: "claude-3-7-sonnet-20250219",
        MCPServers: claudeMCP, // ← MODO NATIVO
        Messages: []anthropic.MessageParam{
            {
                Role: "user",
                Content: "Liste os arquivos do projeto",
            },
        },
    },
)

// 4. Claude acessa filesystem MCP server diretamente!
// Sem passar pelo nosso bridge
```

---

## Configuração

### Habilitar para um servidor específico

No arquivo `.assistente/mcp/filesystem.json`:

```json
{
  "name": "Filesystem Server",
  "transport": "sse",
  "url": "http://localhost:3000/mcp",
  "enabled": true,
  "auto_connect": true,
  "expose_native": true  ← NOVA OPÇÃO (futuro)
}
```

---

## Roadmap

### ✅ Implementado
- [x] Detecção de servidores conectados
- [x] Exposição de metadata para modelos nativos
- [x] Suporte a capabilities (tools/resources/prompts)

### 🚧 Em Desenvolvimento
- [ ] UI para toggle Modo Nativo vs Adapter
- [ ] Métricas de uso nativo vs adapter
- [ ] Suporte a mais modelos além de Claude

### 📋 Planejado
- [ ] Proxy MCP para adicionar auth mesmo em modo nativo
- [ ] Streaming direto do MCP para UI
- [ ] Resource subscriptions em tempo real

---

## FAQ

### P: Posso usar ambos os modos ao mesmo tempo?
**R:** Sim! Você pode usar modo adapter para alguns modelos e nativo para outros.

### P: Como sei se meu modelo suporta MCP nativo?
**R:** Verifique a documentação da API. Atualmente apenas Claude 3.7+ tem suporte oficial.

### P: Modo nativo é mais seguro?
**R:** Não necessariamente. Modo adapter permite adicionar validação/auditoria. Planejamos adicionar proxy para segurança em modo nativo.

### P: Qual é mais rápido?
**R:** Modo nativo é teoricamente mais rápido (menos hops), mas a diferença é mínima para a maioria dos casos.

---

## Conclusão

O suporte a **MCP Nativo** é um **diferencial importante** para quando você usar modelos de última geração como Claude. Mantemos o modo adapter para **compatibilidade universal**, mas oferecemos modo nativo para **máxima performance** quando disponível.

**Ambos os modos coexistem perfeitamente!** 🚀
