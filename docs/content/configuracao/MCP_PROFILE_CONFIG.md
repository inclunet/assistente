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

## Política MCP no perfil

O perfil controla quais tools MCP ficam bloqueadas, disponíveis sob demanda ou
pré-carregadas por meio de `tool_policy`:

```json
{
  "chat": {
    "llm_provider": "openai-default",
    "disable_tools": false,
    "tool_policy_default": "disabled",
    "tool_policy": {
      "mcp/*": "on_demand",
      "mcp/atlassian/*": "preloaded",
      "mcp_atlassian__create_issue": "disabled"
    }
  }
}
```

- `llm_provider` — determina indiretamente o caminho MCP via `api_format` do provider
- `disable_tools` — desabilita todo tool calling (incluindo MCP)
- `tool_policy_default` — estado de tools não cobertas por outra regra
- `tool_policy` — mapa de nomes/seletores para `disabled`, `on_demand` ou `preloaded`

### Seletores aceitos

- `mcp/*` seleciona todas as tools MCP.
- `mcp/<slug>/*` seleciona um servidor, por exemplo `mcp/atlassian/*`.
- `mcp:<slug>/*` é alias da forma anterior.
- `package/<pacote>/*` seleciona builtins pelo pacote do catálogo, por exemplo
  `package/history/*`; `<pacote>/*` é a forma curta.
- `*` seleciona todas as tools nativas.
- `mcp_<slug>__*` é a forma correspondente ao namespace interno.

A precedência é: nome literal, wildcard específico, wildcard geral e default.
Em empate vence o estado mais restritivo. Portanto, no exemplo,
`mcp_atlassian__create_issue=disabled` vence o preload do servidor. Defaults e
wildcards também cobrem tools conectadas depois de o perfil ser carregado.
Tools opt-in nunca são liberadas por wildcard; capacidades internas de
control-plane ainda podem ser autorizadas explicitamente pelo runtime.

O editor de perfis mostra o estado efetivo de cada tool conectada. Os wildcards
continuam preservados no JSON; não é necessário manter manualmente uma lista
para cada nova tool de um servidor.

Os perfis builtin **Padrão** e **Programação** já incluem
`"mcp/*": "on_demand"`. Assim, MCPs atuais e futuras funcionam sem configuração
manual, mas permanecem fora do payload inicial. Essa disponibilidade não
ignora allowlists, risco, confiança de rede ou confirmações de execução.

---

## Referências

- [`internal/llm/chat_provider.go`](../../../internal/llm/chat_provider.go) — interface ChatProvider + SupportsNativeMCP()
- [`internal/llm/provider.go`](../../../internal/llm/provider.go) — APIFormat e GetAPIFormat()
- [`internal/mcp/manager.go`](../../../internal/mcp/manager.go) — GetEligibleNativeMCPServers()
- [PROVIDER_CONFIGURATION.md](PROVIDER_CONFIGURATION.md) — configuração de api_format
- [AEP-0021: MCP Modo Nativo](../../../aep/0021-mcp-native-mode.md)
- [AEP-0037: SDK Migration + ChatProvider](../../../aep/0037-sdk-migration-chat-provider.md)
