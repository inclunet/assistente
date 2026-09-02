---
title: "MCP — Configuração"
weight: 4
---

# MCP no Assistente

## Visão Geral

O Assistente usa MCP (Model Context Protocol) para integrar tools de servidores
externos. A política vigente combina a capacidade física do provider com o
override tri-state do perfil.

---

## Como Funciona

### Dois caminhos de consumo

| Caminho | Quando | Como |
|---------|--------|------|
| **Adapter (bridge)** | Algum gate nativo falha ou a tool foi carregada sob demanda | Assistente registra a tool como function calling e executa localmente |
| **Nativo (server-side)** | Todos os gates abaixo passam para uma tool MCP preloaded | Provider LLM conecta diretamente ao servidor e executa server-side |

### Decisão em runtime

A decisão aplica todos estes gates:

```
provider.NativeMCPCapable() == true
  E Profile.Chat.NativeMCP != false
  E servidor está conectado, tem transporte HTTP e URL elegível
  E servidor.prefer_bridge == false
  E ao menos uma tool MCP do servidor está preloaded pela política efetiva
  → tenta MCP nativo (server-side)

Caso contrário
  → Adapter/bridge (execução local)
```

`Profile.Chat.NativeMCP` é um `*bool`:

- ausente/`nil`: automático otimista; tenta nativo quando o provider é
  fisicamente capaz;
- `true`: força a tentativa nativa, ainda limitada pela capacidade física;
- `false`: força adapter/bridge.

Se o modo automático encontrar modelo ou endpoint incompatível com
`type:"mcp"`, o mesmo turno é refeito em adapter, com as bridge tools
disponíveis. Depois, a transição `nil` → `false` é persistida no perfil para
evitar repetir a falha. Overrides explícitos nunca são sobrescritos
automaticamente.

`native_mcp: true` não ultrapassa os demais gates: não torna STDIO nativo, não
ignora `prefer_bridge` e não promove uma tool `on_demand` para `preloaded`.
Tools MCP carregadas sob demanda permanecem bridge/function no turno.

---

## Suporte por Provider

| Provider | `api_format` | MCP Nativo |
|----------|-------------|------------|
| OpenAI Responses-compatible | `openai_responses` | Sim |
| Anthropic (Claude) | `anthropic` | Sim |
| Google (Gemini) | `google` | Não |
| OpenAI Chat Completions-compatible | `openai` | Não |

Para detalhes sobre `api_format`, veja [Configuração de Providers](PROVIDER_CONFIGURATION.md).

---

## Candidatos do manager e elegibilidade final

`Manager.GetEligibleNativeMCPServers()` monta candidatos com os critérios 1–4
abaixo. O nome do método não significa que qualquer tool disponível será
enviada nativamente: a camada de chat ainda aplica capacidade do provider,
tri-state do perfil e o critério 5 por turno.

Apenas servidores que atendem **todos** estes critérios vão pelo caminho nativo:

1. Conectados e com tools disponíveis
2. Transporte HTTP (SSE ou Streamable HTTP)
3. URL elegível: `https://` sempre, `http://` apenas para localhost/loopback (127.0.0.1, ::1)
4. `prefer_bridge` desativado
5. Ao menos uma tool do servidor está `preloaded` pela política efetiva do
   perfil

URLs `http://` com host remoto são excluídas — o provider LLM faz a conexão server-side e auth tokens seriam transmitidos sem encriptação.

Servidores STDIO/locais **sempre** usam adapter, independente do provider.
Um servidor candidato cuja única tool esteja `on_demand` permanece no caminho
bridge; carregá-la durante o turno não a promove retroativamente a MCP nativo.

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

O perfil de conversa possui o override tri-state `native_mcp`:

```json
{
  "chat": {
    "llm_provider": "openai-default",
    "native_mcp": null,
    "disable_tools": false,
    "enabled_tools": null
  }
}
```

- `llm_provider` — seleciona o provider cuja capacidade física é consultada
- `native_mcp` — `null`/ausente = automático, `true` = forçar nativo,
  `false` = forçar adapter
- `disable_tools` — desabilita todo tool calling (incluindo MCP)
- `enabled_tools` — compatibilidade da allowlist legada quando `tool_policy`
  está ausente/vazia e `tool_policy_default` está ausente/vazio:
  - ausente/`null`: perfil legado aberto; quando `tool_catalog` está disponível,
    começa apenas com o catálogo e permite carga sob demanda; nesse estado,
    tools MCP não entram no caminho nativo inicialmente e, quando carregadas,
    permanecem bridge; sem catálogo, preserva o fallback legado de preload das
    tools não opt-in;
  - `[]`: seleção explícita vazia, desliga as tools iniciais e não adiciona
    tools de runtime;
  - lista: allowlist explícita; os nomes permitidos entram como preload e os
    demais ficam bloqueados por esse contrato legado.

### `tool_policy` e precedência da política efetiva

`internal/chat.ToolSelectionPolicy.ResolveEffectiveToolPolicy()` é a fonte de
verdade da seleção:

1. `disable_tools: true` vence todas as demais opções.
2. Um mapa `tool_policy` não vazio ativa a política nova da AEP-0081 e tem
   precedência sobre `enabled_tools`.
3. `tool_policy_default` define o estado das tools ausentes do mapa e aceita
   `disabled` ou `on_demand`; com `enabled_tools: null`, ele sozinho já ativa a
   política nova. Vazio falha fechado como `disabled`.
4. Se houver somente `tool_policy_default`, mas `enabled_tools` for uma lista
   legada não nula, a allowlist legada continua soberana para não ampliar
   capabilities implicitamente.
5. Sem mapa/default novos, aplicam-se as três formas legadas de
   `enabled_tools` descritas acima.

Estados por tool:

- `preloaded`: entra no conjunto inicial do turno. Para uma tool MCP de servidor
  HTTP elegível, é o único estado que pode satisfazer o gate final de MCP
  nativo.
- `on_demand`: aparece no catálogo e pode ser carregada durante o turno, mas
  continua adapter/bridge nesse turno. Se `tool_catalog` não estiver disponível,
  a resolução promove estados `on_demand` para preload como fallback, pois não
  haveria mecanismo de carga posterior.
- `disabled`: não entra inicialmente, não aparece como carregável e não pode ser
  promovida pelo runtime.

Tools opt-in permanecem `disabled` pelo default e precisam de entrada explícita.
Quando existe alguma tool `on_demand`, a resolução mantém `tool_catalog`
preloaded, salvo negação explícita. Exemplos:

```json
{
  "chat": {
    "enabled_tools": null,
    "tool_policy_default": "on_demand",
    "tool_policy": {
      "mcp_jira__create_issue": "preloaded",
      "mcp_jira__delete_issue": "disabled"
    }
  }
}
```

Nesse exemplo, `enabled_tools: null` não força o comportamento legado: o mapa
explícito prevalece, a tool de criação pode participar do caminho MCP nativo e
a de exclusão permanece bloqueada.

---

## Referências

- [`internal/llm/chat_provider.go`](../../../internal/llm/chat_provider.go) — interface ChatProvider + `NativeMCPCapable()`
- [`internal/profiles/types.go`](../../../internal/profiles/types.go) — `Profile.Chat.NativeMCP`
- [`internal/chat/tool_defs.go`](../../../internal/chat/tool_defs.go) — resolução tri-state e montagem native/adapter
- [`internal/llm/provider.go`](../../../internal/llm/provider.go) — APIFormat e GetAPIFormat()
- [`internal/mcp/manager.go`](../../../internal/mcp/manager.go) — GetEligibleNativeMCPServers()
- [PROVIDER_CONFIGURATION.md](PROVIDER_CONFIGURATION.md) — configuração de api_format
- [AEP-0021: MCP Modo Nativo](../../../aep/0021-mcp-native-mode.md)
- [AEP-0037: SDK Migration + ChatProvider](../../../aep/0037-sdk-migration-chat-provider.md)
