# Backend-Driven Messaging — Desacoplamento Frontend↔Mensagens

## Status: Accepted — contrato vigente

> Tratado como contrato arquitetural em vigor pelo `CLAUDE.md` e pelas instruções
> de agentes (`.github/copilot-instructions.md`). As invariantes desta AEP
> (backend como fonte de verdade, `SendMessage`/`RetryMessage` únicos, eventos
> tipados com `conversationId`, serviços globais arbitrados) são de cumprimento
> obrigatório. Decisões factuais ainda em aberto (ex.: IDs `streaming-*`) são
> rastreadas separadamente e não invalidam o contrato.

## Estratégia de entrega

Este é um **refactor**. Todas as fases são commits em um **único PR**. A ordem dos commits segue a ordem das fases. Cada commit deve deixar os testes passando.

---

## Motivação

## Baseline histórico anterior à implementação

O fluxo abaixo descreve o estado encontrado antes desta AEP, não o runtime
vigente:

### 1. IDs temporários e otimismo sem garantia
O frontend gera IDs com `Date.now()-random` e insere mensagens na UI *antes* de qualquer confirmação do backend. Se a chamada Wails falhar, o usuário vê uma mensagem de user e um placeholder de assistant que **nunca existiram no banco**. O mapeamento `tempId → backendId` acontece em dois momentos distintos (`chat:messages_ready` para user, `chat:stream done=true` para assistant), cada um com lógica própria de substituição — são ~60 linhas de código frágil espalhadas em closures aninhadas.

### 2. Placeholders sem correspondência no banco
`addMessage({role:'assistant', content:'', isStreaming:true})` cria um nó na árvore de mensagens que **não existe no banco de dados**. Até `chat:done` chegar, a UI exibe uma entidade fantasma. Se o backend falhar silenciosamente, esse nó fica pendurado para sempre com `isStreaming: true`.

### 3. Eventos sem padronização
O sistema usa 13+ eventos `chat:*` com payloads diferentes, sem schema validado:

| Evento | Payload contém | Nota |
|--------|---------------|------|
| `chat:messages_ready` | `{userMessageId, conversationId, userContent}` | OK |
| `chat:stream` | `{content, done, error, messageId}` | `messageId` só existe quando `done=true` |
| `chat:done` | `{}` (vazio) | Sem conversationId, sem messageId |
| `chat:tool_start` | `{name, callId, args}` | Sem conversationId |
| `chat:tool_end` | `{callId, name, status, summary}` | Sem conversationId |
| `chat:segment_done` | `{hasMore, content}` | Sem conversationId |

Naquele baseline, nenhum evento carregava `conversationId` de forma
consistente e o frontend dependia de closure capture.

### Contrato vigente

| Família de evento | Identidade vigente |
|---|---|
| `chat:messages_ready`, `chat:stream`, `chat:done` | `conversationId` e IDs persistidos como `string` |
| `chat:tool_start`, `chat:tool_end`, `chat:segment_done` | `conversationId` obrigatório |
| `chat:error` | `conversationId` obrigatório pelo contrato; há lacuna conhecida para erro anterior à resolução da conversa |

Os payloads tipados vivem em `internal/core/ports/chat_events.go`; controllers
frontend filtram por conversa em
`frontend/src/services/chatEventController.ts`.

### 4. Lógica de negócio no frontend
`sendMessageWithParams` (chatStore.ts) tem ~200 linhas que fazem:
- Validação de tamanho de conteúdo e mídia
- **Criação implícita de conversa** (se não existe)
- Geração de IDs
- Conversão de arquivos para base64
- Montagem de params com merge de overrides
- Registro de 7 event listeners com cleanup manual
- Mapeamento de IDs temporários
- Auto-rename de conversa
- Auto-play TTS
- Reload completo de mensagens quando houve tool calls

Isso são **decisões de negócio** vivendo no frontend: quando recarregar mensagens, quando renomear conversa, como lidar com erro de streaming — tudo deveria ser backend-driven.

### 5. Frontend como orquestrador
O frontend registra listeners, dispara a chamada Wails, recebe eventos, decide quando fazer reload, decide quando fazer cleanup. O backend é "burro" — emite eventos e espera que alguém esteja ouvindo. Se o frontend perder um evento (race condition, tab switch, erro JS), o estado fica inconsistente.

### 6. Criação implícita de conversa no envio
O frontend cria conversa automaticamente se `activeConversationId === 0`. Isso acopla criação de conversa ao envio de mensagem — duas responsabilidades distintas no mesmo fluxo. Mensagens devem ser enviadas **apenas** para conversas que já existem. Se a conversa não existe, a superfície do workspace deve garantir e persistir o `conversationId` antes do envio.

---

## Princípios do redesign

1. **Backend é a fonte de verdade** — o frontend nunca cria mensagens locais; só renderiza o que o backend manda.
2. **Sem IDs temporários** — mensagens só aparecem na UI quando o backend confirma com ID real.
3. **Eventos tipados com conversationId** — todo evento carrega identificação da conversa.
4. **Frontend é reativo por superfície** — cada superfície interessada escuta eventos do seu `conversationId` e atualiza o próprio estado visual, sem orquestrar o ciclo de vida de mensagens.
5. **Testável** — cada fase tem critérios de aceitação verificáveis com testes automatizados.
6. **Conversa é pré-requisito** — mensagens só podem ser enviadas para conversas
   que já existem. `SendMessage` com `conversationId=""` retorna erro de ID
   ausente; um UUID informado mas inexistente retorna erro de conversa não
   encontrada. A criação de conversa é responsabilidade separada.
7. **Conversas são independentes** — conversas existem no banco sem vínculo forte com abas, workspace ou qualquer conceito de UI. Abas carregam conversas para exibição, mas conversas sobrevivem sem aba. Canais (Telegram, Signal) criam e mantêm conversas independentemente de haver aba aberta.
8. **Um único contrato para mensagem nova, com retry explícito** — existe UM método `SendMessage` para novas mensagens no backend. O único endpoint adicional permitido é `RetryMessage`, usado exclusivamente para reenviar uma mensagem de usuário já persistida, sem criar nova mensagem `user`. No frontend, todas as superfícies reutilizam o mesmo cliente/pipeline compartilhado para esses contratos explícitos por `conversationId`, sem duplicar lógica divergente de envio.
9. **Controllers por conversa/aba são permitidos** — o frontend pode instanciar controllers autocontidos por `conversationId` ou por aba. Esses controllers podem manter estado próprio de UI, streaming, scroll, histórico carregado e tool calls, desde que filtrem eventos pelo `conversationId` e deleguem envio/retry ao contrato compartilhado.
10. **Contexto de superfície é estruturado** — contexto da aba ativa não deve ser injetado artificialmente no texto do usuário. Ele deve viajar em parâmetros estruturados (`tabType`, `surfaceState`, `surfaceContext`, `surfaceSessionKey`, `surfaceId`, `surfaceType`, `surfaceTabId`) para que profiles, skills, tools e eventos consumam isso de forma consistente.
11. **Eventos contidos e acionáveis** — eventos não são apenas notificações passivas; o backend os usa para tomar decisões de orquestração (quando sintetizar TTS, quando renomear conversa, quando notificar canal externo). O protocolo de eventos é o contrato central do sistema.
12. **Serviços globais são arbitrados, não donos das abas** — recursos globais como anúncios para leitor de tela, TTS e STT não devem ser duplicados por aba. Controllers por aba solicitam esses recursos por uma política central que respeita aba ativa, perfil efetivo e exclusividade de fala/escuta.

---

## Fase 1 — Protocolo de eventos tipado (foundation)

### Objetivo
Padronizar TODOS os eventos de messaging com schema consistente e conversationId obrigatório.

### Implementação

#### 1.1 Definir envelope de evento no backend

```go
// internal/events/message_events.go

// ChatEventEnvelope é o wrapper padrão para todos os eventos de chat.
// Todo evento de messaging DEVE carregar ConversationID.
type ChatEventEnvelope struct {
    ConversationID string `json:"conversationId"`
    Type           string `json:"type"` // redundante com nome do evento, mas útil para debug
}

type MessagesReadyEvent struct {
    ChatEventEnvelope
    UserMessageID string `json:"userMessageId"`
    UserContent   string `json:"userContent"`
}

type StreamChunkEvent struct {
    ChatEventEnvelope
    Content   string `json:"content"`
    Done      bool   `json:"done"`
    Error     string `json:"error,omitempty"`
    MessageID string `json:"messageId,omitempty"` // presente quando done=true
}

type StreamDoneEvent struct {
    ChatEventEnvelope
    AssistantMessageID string `json:"assistantMessageId"`
    HadToolCalls       bool   `json:"hadToolCalls"`
    FinalContent       string `json:"finalContent,omitempty"`
}

type ThinkingEvent struct {
    ChatEventEnvelope
    Phase   string `json:"phase"` // "started" | "progress" | "done"
    Content string `json:"content,omitempty"`
}

type ToolEvent struct {
    ChatEventEnvelope
    Phase   string `json:"phase"` // "start" | "end"
    Name    string `json:"name"`
    CallID  string `json:"callId"`
    Args    string `json:"args,omitempty"`
    Status  string `json:"status,omitempty"`  // "done" | "error" (apenas end)
    Summary string `json:"summary,omitempty"` // apenas end
}

type SegmentDoneEvent struct {
    ChatEventEnvelope
    HasMore bool   `json:"hasMore"`
    Content string `json:"content,omitempty"`
}
```

#### 1.2 Atualizar emissores no backend

Cada ponto que faz `runtime.EventsEmit(ctx, "chat:*", ...)` passa a usar as structs tipadas. Locais a alterar:
- `internal/chat/interactor.go` — `chat:messages_ready`
- `app_stream_handler.go` — `chat:stream`, `chat:done`, `chat:token_stats`, `chat:context_warning`
- `internal/agent/service.go` — `chat:tool_start`, `chat:tool_end`, `chat:segment_done`, `chat:done`
- `internal/events/base_handler.go` — `chat:thinking`

#### 1.3 Tipar no frontend

```typescript
// frontend/src/types/chatEvents.ts

interface ChatEventEnvelope {
  conversationId: string;
  type: string;
}

interface MessagesReadyEvent extends ChatEventEnvelope {
  userMessageId: string;
  userContent: string;
}

interface StreamChunkEvent extends ChatEventEnvelope {
  content: string;
  done: boolean;
  error?: string;
  messageId?: string;
}

interface StreamDoneEvent extends ChatEventEnvelope {
  assistantMessageId: string;
  hadToolCalls: boolean;
  finalContent?: string;
}
// ... etc
```

#### 1.4 Frontend filtra por conversationId

Cada listener verifica `event.conversationId === activeConversationId` antes de processar. Remove a dependência de closure capture.

### Testes
- [ ] Unit test Go: cada struct serializa corretamente para JSON
- [ ] Unit test Go: emissores incluem conversationId válido
- [ ] Unit test Frontend: listener ignora evento de outra conversa
- [ ] Integration test: fluxo completo emite eventos com schema correto

### Critério de aceitação
- [ ] Zero eventos emitidos sem `conversationId` válido — lacuna de conformidade
  descrita abaixo.
- [x] Frontend em conversa A ignora eventos identificados da conversa B sem
  lógica de closure.

### Lacuna conhecida de conformidade

`internal/chat/interactor.go`, em `PrepareContext`, emite `chat:error` com
`ConversationID: ""` quando o request não possui conversa. Em paralelo,
`frontend/src/services/chatEventController.ts` aceita `chat:error` com
`conversationId === ""` como broadcast. Esse comportamento viola o contrato
vigente de identificação obrigatória e pode anunciar o erro em superfícies não
relacionadas. A correção de implementação permanece pendente: o erro deve ser
associado a uma origem/superfície identificável sem relaxar o requisito de
`conversationId` para eventos de chat.

---

## Fase 2 — Backend-driven message lifecycle (core)

### Objetivo
Eliminar IDs temporários e placeholders. O backend assume controle do ciclo de vida das mensagens.

### Implementação

#### 2.1 Backend emite `chat:user_message_created` com mensagem completa

Quando o backend salva a mensagem do user no banco, emite:

```go
type UserMessageCreatedEvent struct {
    ChatEventEnvelope
    Message EnrichedMessage `json:"message"` // mensagem completa, com ID do banco
}
```

O frontend recebe e insere no estado. **O frontend não cria mais a mensagem do user localmente.**

#### 2.2 Backend emite `chat:assistant_message_started` com ID real

Antes de iniciar streaming, o backend cria o registro da mensagem do assistant no banco (com `content=""`) e emite:

```go
type AssistantMessageStartedEvent struct {
    ChatEventEnvelope
    Message EnrichedMessage `json:"message"` // ID real do banco, content vazio, isStreaming=true
}
```

O frontend recebe e insere no estado. **O frontend não cria mais o placeholder do assistant localmente.**

#### 2.3 Streaming atualiza mensagem existente (sem ID temporário)

`chat:stream` agora carrega o `messageId` real em **todo** chunk, não só no final:

```go
type StreamChunkEvent struct {
    ChatEventEnvelope
    MessageID string `json:"messageId"` // SEMPRE presente
    Content   string `json:"content"`
    Done      bool   `json:"done"`
    Error     string `json:"error,omitempty"`
}
```

O frontend faz `updateMessage(event.messageId, event.content)` — sem mapeamentos.

#### 2.4 `chat:done` carrega estado final completo

```go
type ChatDoneEvent struct {
    ChatEventEnvelope
    AssistantMessageID string `json:"assistantMessageId"`
    HadToolCalls       bool `json:"hadToolCalls"`
    // Se hadToolCalls, o backend já recarregou e emite a árvore atualizada:
    UpdatedMessages []MessageNode `json:"updatedMessages,omitempty"`
}
```

Quando houve tool calls, o backend inclui a árvore de mensagens atualizada no próprio evento `chat:done`. **O frontend não precisa fazer `GetMessages()` manualmente.**

#### 2.5 Mudanças no frontend no contrato compartilhado de envio

A camada compartilhada de envio encolhe para ~20 linhas. **Não existe mais `sendMessageWithParams`** e superfícies diferentes não duplicam lógica de envio; todas delegam para o mesmo cliente por `conversationId`. Controllers por aba podem manter seu próprio estado de loading/streaming, mas não reimplementam validação, serialização de mídia ou chamada ao backend:

```typescript
sendMessageToConversation: async (conversationId, content, mediaFiles, paramsOverride, options) => {
  if (!conversationId) {
    // Conversa DEVE existir antes de enviar mensagem.
    // Se não existe, é erro. Criação de conversa é responsabilidade separada.
    announce(t('chat.errors.noActiveConversation'));
    return;
  }

  const mediaJson = mediaFiles ? await serializeMedia(mediaFiles) : '';
  const params = buildParams({
    ...paramsOverride,
    surfaceSessionKey: options?.origin?.sessionKey,
    surfaceId: options?.origin?.surfaceId,
    surfaceType: options?.origin?.surfaceType,
    surfaceTabId: options?.origin?.tabId,
  });

  try {
    await SendMessage(conversationId, content, mediaJson, params);
  } catch (error) {
    reportSurfaceError(options?.origin?.sessionKey, getErrorMessage(error));
  }
  // Não faz mais NADA aqui. O backend emite eventos e o frontend reage.
}
```

**Removido**: criação implícita de conversa (`if (conversationId === 0) { createConversation() }`). O caller é responsável por garantir que a conversa existe.

#### 2.6 Event listeners por controller de conversa (não por chamada)

Os listeners não são registrados dentro de cada envio. Cada controller de conversa/aba registra seus listeners enquanto estiver vivo, filtra eventos pelo seu `conversationId` e atualiza apenas o próprio estado visual:

```typescript
// frontend/src/hooks/useChatController.ts
export function useChatController(conversationId: string) {
  const controller = useMemo(() => createChatController(conversationId), [conversationId]);
  
  useEffect(() => {
    const unsubs = [
      EventsOn('chat:user_message_created', (e: UserMessageCreatedEvent) => {
        if (e.conversationId !== conversationId) return;
        controller.insertBackendMessage(e.message);
        playSendSound();
      }),
      EventsOn('chat:assistant_message_started', (e: AssistantMessageStartedEvent) => {
        if (e.conversationId !== conversationId) return;
        controller.insertBackendMessage(e.message);
        controller.setStreamingMessageId(e.message.id);
      }),
      EventsOn('chat:stream', (e: StreamChunkEvent) => {
        if (e.conversationId !== conversationId) return;
        controller.updateMessage(e.messageId, e.content);
      }),
      EventsOn('chat:done', (e: ChatDoneEvent) => {
        if (e.conversationId !== conversationId) return;
        if (e.updatedMessages) {
          controller.replaceMessages(e.updatedMessages);
        }
        controller.finishStreaming();
      }),
      // ... thinking, tool_start, tool_end, segment_done
    ];
    return () => unsubs.forEach(fn => fn());
  }, [conversationId, controller]);
}
```

### Testes
- [ ] Unit test Go: `SendMessage` com `conversationID=""` retorna erro de ID ausente
- [ ] Unit test Go: `SendMessage` com UUID inexistente retorna erro de conversa não encontrada
- [ ] Unit test Go: `SendMessage` cria registro no banco ANTES de emitir `chat:user_message_created`
- [ ] Unit test Go: `chat:assistant_message_started` emitido com ID real
- [ ] Unit test Go: `chat:done` inclui `updatedMessages` quando `hadToolCalls=true`
- [ ] Unit test Frontend: `insertBackendMessage` adiciona mensagem ao estado
- [ ] Unit test Frontend: o pipeline de envio NÃO cria mensagens locais
- [ ] Unit test Frontend: enviar sem `conversationId` mostra erro, não cria conversa
- [ ] Unit test Frontend: listeners ignoram eventos de outra conversa
- [ ] Integration test: mensagem enviada → UI mostra mensagem com ID do banco, sem temp ID

### Critério de aceitação
- `generateId()` removido do chatStore
- `addMessage()` removido do fluxo de envio (mantido apenas para uso interno se necessário)
- Zero mensagens na UI sem correspondência no banco
- O pipeline de envio do frontend tem < 30 linhas
- `sendMessageWithParams` não existe mais
- Enviar mensagem sem `conversationId` gera erro claro, sem criação implícita

---

## Fase 3 — Auto-rename e pós-processamento backend-driven

### Objetivo
Mover toda lógica de pós-processamento (auto-rename, TTS auto-read, reload com tools) para o backend.

### Implementação

#### 3.1 Auto-rename no backend

```go
// Dentro de OnDone() no app_stream_handler.go, após salvar:
if shouldAutoRename(conv) {
    firstUserContent := getFirstUserContent(conversationID)
    title := truncate(firstUserContent, 50)
    convRepo.RenameConversation(conversationID, title)
    runtime.EventsEmit(ctx, "conversation:renamed", ConversationRenamedEvent{
        ConversationID: conversationID,
        Title:          title,
    })
}
```

O frontend não decide mais quando renomear. Escuta `conversation:renamed` e atualiza o título na tab/sidebar.

#### 3.2 TTS decision no backend

O backend já sabe o perfil, as configs de TTS, e o conteúdo final. Ele decide se deve sintetizar e emite:

```go
// Se auto-read habilitado:
runtime.EventsEmit(ctx, "chat:speak", ChatSpeakEvent{
    ConversationID: conversationID,
    Content:        finalContent,
    Source:         "assistant",
    MessageID:      savedMsgID,
})
```

O frontend já tem listener para `chat:speak` — basta garantir que a decisão é do backend.

#### 3.3 Tool reload no backend

Já resolvido na Fase 2 — `chat:done` carrega `updatedMessages` quando houve tool calls.

### Testes
- [ ] Unit test Go: conversa com título padrão é renomeada após primeira resposta
- [ ] Unit test Go: conversa com título customizado NÃO é renomeada
- [ ] Unit test Frontend: `conversation:renamed` atualiza título no estado
- [ ] Integration test: fluxo completo → conversa renomeada automaticamente

### Critério de aceitação
- Zero lógica de rename no frontend
- Zero lógica de "quando fazer TTS" no frontend (apenas player/plumbing)
- `chat:done` handler no frontend tem < 15 linhas

---

## Fase 4 — Validação no backend, frontend thin

### Objetivo
Mover validações (tamanho de conteúdo, tamanho de mídia) para o backend. O frontend faz validação apenas para UX imediata (feedback rápido), mas o backend é o guardião.

### Implementação

#### 4.1 Validação no use case

```go
func (uc *SendMessageUseCase) validate(req SendMessageRequest) error {
    if len(req.UserContent) > MaxContentSize {
        return ErrContentTooLarge
    }
    if len(req.UserMedia) > MaxMediaSize {
        return ErrMediaTooLarge  
    }
    return nil
}
```

Se a validação falha, `SendMessage` retorna erro via Wails e o frontend mostra.

#### 4.2 Frontend mantém validação como hint

O frontend pode manter checks de tamanho para evitar chamada Wails desnecessária, mas o backend é a autoridade. Se o frontend falhar em validar (bug), o backend rejeita.

### Testes
- [ ] Unit test Go: conteúdo > 500KB retorna `ErrContentTooLarge`
- [ ] Unit test Go: media > 10MB retorna `ErrMediaTooLarge`
- [ ] Unit test Frontend: erro de validação do backend é exibido ao usuário

### Critério de aceitação
- Backend rejeita mensagens inválidas antes de criar registro no banco
- Frontend exibe erros do backend para o usuário

---

## Fase 5 — Cleanup e dívida técnica

### Objetivo
Remover código morto e simplificar.

### Itens
- [x] Remover `generateId()` do chatStore
- [x] Remover `addMessage()` do fluxo de envio (e a função inteira — sem consumidores)
- [x] ~~Remover lógica de mapeamento `tempId → backendId`~~ — mantido: necessário para placeholder de streaming (`streaming-${conversationId}`)
- [x] ~~Remover `unsubscribe*` manuais~~ — mantido: essencial para cleanup de event listeners
- [x] ~~Remover `activeListeners` Map~~ — mantido: previne memory leaks de listeners
- [x] Remover `DEFAULT_TITLE_PATTERNS` e lógica de rename do frontend (feito na Fase 2)
- [x] Unificar o pipeline de envio do frontend e remover `sendMessageWithParams` (feito entre Fase 2-3)
- [x] Remover criação implícita de conversa do fluxo de envio
- [x] ~~Atualizar tipos `id` para `number`~~ — mantido como `string`: streaming placeholder usa string determinística
- [x] ~~Remover `debouncedUpdateMessage` e `flushPendingUpdate`~~ — mantido: crítico para performance (60fps throttle)
- [x] Atualizar todos os 3 locales com novas strings de erro (feito na Fase 4)
- [x] Remover `hadToolCalls` local state — agora usa apenas `event.hadToolCalls` do backend

### Testes
- [x] Todos os testes existentes passam (168 files, 1007 tests)
- [ ] Cobertura de `chatStore.ts` ≥ 80%
- [x] Zero `console.log` remanescentes no código tocado
- [x] Lint + build passam

---

## Ordem de execução

```
Commit 1: Fase 1 (eventos tipados)
Commit 2: Fase 2 (backend-driven lifecycle)  ← MAIOR IMPACTO
Commit 3: Fase 3 (pós-processamento)
Commit 4: Fase 4 (validação backend)
Commit 5: Fase 5 (cleanup)
```

Tudo em um único PR. Cada commit deve deixar os testes passando.

---

## Impacto estimado

| Métrica | Antes | Depois |
|---------|-------|--------|
| Linhas da camada principal de envio | ~200 | ~30 |
| Event listeners registrados por chamada | 7 | 0 (globais) |
| IDs temporários na UI | 2 por mensagem | 0 |
| Mensagens fantasma (sem banco) | Possível | Impossível |
| Pontos de mapeamento tempId→backendId | 2 | 0 |
| Lógica de negócio no frontend | rename, reload, TTS decision | Nenhuma |
| Eventos sem conversationId | ~50% | 0% |

---

## Riscos e mitigações

| Risco | Mitigação |
|-------|-----------|
| Latência percebida: mensagem user demora ~50ms para aparecer | Backend emite `chat:user_message_created` imediatamente após o insert, antes de iniciar LLM. A latência real é a do Wails IPC (~5ms) + insert SQLite (~2ms). Imperceptível. Se necessário, o frontend pode mostrar um estado "enviando..." sem criar mensagem fake. |
| Canais externos perdem eventos | Canais não usam Wails events — usam callback direto. Sem impacto. |
| Testes existentes quebram | Cada commit atualiza testes antes de mudar código. |

---

## Invariantes arquiteturais (conversas e mensagens)

Estas regras são permanentes e devem ser respeitadas por qualquer mudança futura no codebase. Elas estão documentadas aqui e replicadas nos arquivos de instrução para agentes de codificação (`.github/copilot-instructions.md`, `CLAUDE.md`).

### Conversas
- **Conversas são entidades independentes.** Não têm vínculo forte com abas, workspace ou qualquer conceito de UI. Abas carregam conversas para exibição, mas conversas existem no banco independentemente de haver aba aberta.
- **Canais criam e mantêm conversas sem abas.** Telegram, Signal e outros canais criam conversas diretamente no backend. A existência de uma aba é irrelevante para o ciclo de vida da conversa.
- **Mensagens só podem ser enviadas para conversas que existem.** `SendMessage`
  com `conversationID=""` falha por ausência de ID; UUID inexistente falha como
  conversa não encontrada. Não há criação implícita.

### Envio de mensagens
- **Existe UMA única função `SendMessage` para mensagens novas no backend** (`app_chat.go` → `ChatController` → `SendMessageUseCase`). Toda mensagem nova — vinda do frontend, de canais, de deep links — passa por essa função.
- **`RetryMessage` é a única exceção permitida ao contrato acima.** Ele existe apenas para reexecutar a resposta a partir de uma mensagem de usuário já persistida, sem inserir nova linha `user` no banco. É proibido criar qualquer outro endpoint público de envio.
- **Existe UM único contrato compartilhado de envio no frontend.** Componentes e controllers podem ser instanciados por aba/conversa, mas todos delegam mensagem nova para `SendMessage` e retry explícito para `RetryMessage`. Nenhum componente, hook ou store pode duplicar a lógica de validação, serialização, parâmetros ou chamada ao backend em um fluxo alternativo.
- **O backend é a fonte de verdade.** O frontend não cria mensagens locais, não gera IDs temporários, não decide quando recarregar mensagens. Renderiza o que o backend emite via eventos.

### Eventos
- **Todo evento de chat carrega `conversationId`.** Sem exceções.
- **Eventos são tipados com structs/interfaces.** Nada de `map[string]interface{}` ou objetos genéricos.
- **O protocolo de eventos é o contrato central.** O backend usa eventos para orquestrar TTS, rename, notificação de canais. Alterar o schema de um evento exige atualização de todos os consumidores.
- **Controllers filtram eventos por conversa.** Um controller de aba/conversa só processa eventos do seu `conversationId`; isso permite respostas simultâneas em abas diferentes sem uma conversa bloquear a outra.

### Serviços globais da interface
- **Announcer é global e único.** Não há múltiplas live regions por aba. Controllers solicitam anúncios a uma política central, que anuncia progresso normal apenas para a aba ativa e eventos relevantes de abas inativas com contexto de aba/conversa.
- **TTS é globalmente exclusivo.** Duas abas podem responder em paralelo, mas não podem falar ao mesmo tempo. A arbitragem usa a configuração/perfil efetivo da aba que originou a fala, ou da aba ativa quando a ação for iniciada manualmente.
- **STT local só funciona na aba ativa.** Abas inativas em keep-alive não podem ouvir microfone, transcrever nem enviar mensagens por captura local. Entradas de canais externos, como Telegram, Slack ou Signal, seguem o fluxo backend-driven de canais e independem da aba ativa da interface.

---

## Referências
- AEP-0010: Streaming Architecture
- AEP-0006: Chat Architecture Fix
- AEP-0039: Tool Calling Revamp (Fase 1 complementar)
