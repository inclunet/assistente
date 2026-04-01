---
title: "MCP — Configuração"
weight: 4
---

# MCP no Assistente

## Visão Geral

O Assistente usa MCP (Model Context Protocol) para integrar tools de servidores externos. A decisão de como MCP servers são consumidos é **capability-driven** — determinada pelo runtime com base no provider LLM configurado, não por campos manuais no perfil.

---

## Como Funciona

### Dois caminhos de consumo

| Caminho | Quando | Como |
|---------|--------|------|
| **Adapter (bridge)** | Provider não suporta MCP nativo, ou servidor é STDIO local | Assistente descobre tools via MCP, registra como function calling normal, executa localmente |
| **Nativo (server-side)** | Provider suporta MCP nativo E servidor HTTP com URL elegível | Provider LLM se conecta diretamente ao MCP server, resolve tool calls server-side |

### Decisão em runtime

A decisão é automática e baseada em capacidade real:

```
provider.SupportsNativeMCP() == true
  E servidor tem transporte HTTP (SSE/Streamable)
  → MCP nativo (server-side)

Caso contrário
  → Adapter/bridge (execução local)
```

Não existe campo de perfil que controle isso. A fonte de verdade é o `api_format` do provider LLM.

---

## Suporte por Provider

| Provider | `api_format` | MCP Nativo |
|----------|-------------|------------|
| OpenAI (api.openai.com) | `openai_responses` | Sim |
| Anthropic (Claude) | `anthropic` | Sim |
| Google (Gemini) | `google` | Não |
| OpenAI-compatible (OpenRouter, Ollama, Groq, etc.) | `openai` | Não |

Para detalhes sobre `api_format`, veja [Configuração de Providers](PROVIDER_CONFIGURATION.md).

---

## Servidores Elegíveis para MCP Nativo

Apenas servidores que atendem **todos** estes critérios vão pelo caminho nativo:

1. Conectados e com tools disponíveis
2. Transporte HTTP (SSE ou Streamable HTTP)
3. URL elegível: `https://` sempre, `http://` apenas para localhost/loopback (127.0.0.1, ::1)

URLs `http://` com host remoto são excluídas — o provider LLM faz a conexão server-side e auth tokens seriam transmitidos sem encriptação.

Servidores STDIO/locais **sempre** usam adapter, independente do provider.

---

## Coexistência na mesma request

Quando MCP nativo está ativo, a mesma request ao LLM pode conter:

- **Tools internas** (task, task_list, etc.) — function calling normal
- **MCP servers HTTP** — nativo server-side
- **MCP servers STDIO** — bridge/adapter no function calling

O modelo usa todas simultaneamente. Tools nativas são resolvidas server-side; tools internas e STDIO bridges são executadas localmente pelo agentic loop.

---

## Impacto nos Jobs

**Nenhum.** Jobs usam `toolRegistry.Get(name).Execute()` diretamente, sem LLM. MCP tools continuam registradas como `MCPToolBridge` no registry e funcionam identicamente, independente do caminho nativo.

---

## Configuração do Perfil

O perfil de conversa **não** tem campos para controlar MCP nativo. Os campos relevantes são:

```json
{
  "chat": {
    "llm_provider": "openai-default",
    "disable_tools": false,
    "enabled_tools": null
  }
}
```

- `llm_provider` — determina indiretamente o caminho MCP via `api_format` do provider
- `disable_tools` — desabilita todo tool calling (incluindo MCP)
- `enabled_tools` — filtra quais tools estão disponíveis (null = todas)

---

## Referências

- [`internal/llm/chat_provider.go`](../../../internal/llm/chat_provider.go) — interface ChatProvider + SupportsNativeMCP()
- [`internal/llm/provider.go`](../../../internal/llm/provider.go) — APIFormat e GetAPIFormat()
- [`internal/mcp/manager.go`](../../../internal/mcp/manager.go) — GetEligibleNativeMCPServers()
- [PROVIDER_CONFIGURATION.md](PROVIDER_CONFIGURATION.md) — configuração de api_format
- [AEP-0021: MCP Modo Nativo](../../../aep/0021-mcp-native-mode.md)
- [AEP-0037: SDK Migration + ChatProvider](../../../aep/0037-sdk-migration-chat-provider.md)
