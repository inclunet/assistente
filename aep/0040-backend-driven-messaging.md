# Backend-Driven Messaging — Desacoplamento Frontend↔Mensagens

## Status: Proposto

## Estratégia de entrega

Este é um **refactor**. Todas as fases são commits em um **único PR**. A ordem dos commits segue a ordem das fases. Cada commit deve deixar os testes passando.

---

## Motivação

O fluxo atual de envio e recebimento de mensagens sofre de problemas estruturais:

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

Nenhum evento carrega `conversationId` de forma consistente. O frontend depende de closure capture para saber a qual conversa o evento pertence — funciona por acaso, não por design.

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
4. **Frontend é reativo** — escuta eventos e atualiza estado; não orquestra fluxo.
5. **Testável** — cada fase tem critérios de aceitação verificáveis com testes automatizados.
6. **Conversa é pré-requisito** — mensagens só podem ser enviadas para conversas que já existem. `SendMessage` para `conversationId=0` ou inexistente retorna erro. A criação de conversa é responsabilidade separada.
7. **Conversas são independentes** — conversas existem no banco sem vínculo forte com abas, workspace ou qualquer conceito de UI. Abas carregam conversas para exibição, mas conversas sobrevivem sem aba. Canais (Telegram, Signal) criam e mantêm conversas independentemente de haver aba aberta.
8. **Um único pipeline para mensagem nova, com retry explícito** — existe UM método `SendMessage` para novas mensagens no backend. O único endpoint adicional permitido é `RetryMessage`, usado exclusivamente para reenviar uma mensagem de usuário já persistida, sem criar nova mensagem `user`. No frontend, todas as superfícies convergem para esses contratos explícitos por `conversationId`, sem fluxos paralelos de envio.
9. **Contexto de superfície é estruturado** — contexto da aba ativa não deve ser injetado artificialmente no texto do usuário. Ele deve viajar em parâmetros estruturados (`tabType`, `surfaceState`, `surfaceContext`) para que profiles, skills e tools consumam isso de forma consistente.
10. **Eventos contidos e acionáveis** — eventos não são apenas notificações passivas; o backend os usa para tomar decisões de orquestração (quando sintetizar TTS, quando renomear conversa, quando notificar canal externo). O protocolo de eventos é o contrato central do sistema.

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
    ConversationID uint   `json:"conversationId"`
    Type           string `json:"type"` // redundante com nome do evento, mas útil para debug
}

type MessagesReadyEvent struct {
    ChatEventEnvelope
    UserMessageID uint   `json:"userMessageId"`
    UserContent   string `json:"userContent"`
}

type StreamChunkEvent struct {
    ChatEventEnvelope
    Content   string `json:"content"`
    Done      bool   `json:"done"`
    Error     string `json:"error,omitempty"`
    MessageID uint   `json:"messageId,omitempty"` // presente quando done=true
}

type StreamDoneEvent struct {
    ChatEventEnvelope
    AssistantMessageID uint   `json:"assistantMessageId"`
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
  conversationId: number;
  type: string;
}

interface MessagesReadyEvent extends ChatEventEnvelope {
  userMessageId: number;
  userContent: string;
}

interface StreamChunkEvent extends ChatEventEnvelope {
  content: string;
  done: boolean;
  error?: string;
  messageId?: number;
}

interface StreamDoneEvent extends ChatEventEnvelope {
  assistantMessageId: number;
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
- Zero eventos emitidos sem `conversationId`
- Frontend pode estar em conversa A e ignorar eventos de conversa B sem lógica de closure

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
    MessageID uint   `json:"messageId"` // SEMPRE presente
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
    AssistantMessageID uint `json:"assistantMessageId"`
    HadToolCalls       bool `json:"hadToolCalls"`
    // Se hadToolCalls, o backend já recarregou e emite a árvore atualizada:
    UpdatedMessages []MessageNode `json:"updatedMessages,omitempty"`
}
```

Quando houve tool calls, o backend inclui a árvore de mensagens atualizada no próprio evento `chat:done`. **O frontend não precisa fazer `GetMessages()` manualmente.**

#### 2.5 Mudanças no frontend no pipeline de envio

A camada de envio encolhe para ~20 linhas. **Não existe mais `sendMessageWithParams`** e superfícies diferentes não implementam envios paralelos; todas convergem para um mesmo envio por `conversationId`:

```typescript
sendMessageToConversation: async (conversationId, content, mediaFiles, paramsOverride) => {
  if (!conversationId) {
    // Conversa DEVE existir antes de enviar mensagem.
    // Se não existe, é erro. Criação de conversa é responsabilidade separada.
    announce(t('chat.errors.noActiveConversation'));
    return;
  }

  set({ isLoading: true });
  
  const mediaJson = mediaFiles ? await serializeMedia(mediaFiles) : '';
  const params = buildParams(paramsOverride, get().contextProfileSlug);

  try {
    await SendMessage(conversationId, content, mediaJson, params);
  } catch (error) {
    set({ isLoading: false });
    announce(getErrorMessage(error));
  }
  // Não faz mais NADA aqui. O backend emite eventos e o frontend reage.
}
```

**Removido**: criação implícita de conversa (`if (conversationId === 0) { createConversation() }`). O caller é responsável por garantir que a conversa existe.

#### 2.6 Event listener centralizado (não por chamada)

Os listeners são registrados UMA VEZ no mount da app (ou no hook `useChatEvents`), não dentro do envio:

```typescript
// frontend/src/hooks/useChatEvents.ts
export function useChatEvents() {
  const activeConversationId = useChatStore(s => s.activeConversationId);
  
  useEffect(() => {
    const unsubs = [
      EventsOn('chat:user_message_created', (e: UserMessageCreatedEvent) => {
        if (e.conversationId !== activeConversationId) return;
        useChatStore.getState().insertBackendMessage(e.message);
        playSendSound();
      }),
      EventsOn('chat:assistant_message_started', (e: AssistantMessageStartedEvent) => {
        if (e.conversationId !== activeConversationId) return;
        useChatStore.getState().insertBackendMessage(e.message);
        set({ streamingMessageId: e.message.id });
      }),
      EventsOn('chat:stream', (e: StreamChunkEvent) => {
        if (e.conversationId !== activeConversationId) return;
        useChatStore.getState().updateMessage(e.messageId, e.content);
      }),
      EventsOn('chat:done', (e: ChatDoneEvent) => {
        if (e.conversationId !== activeConversationId) return;
        if (e.updatedMessages) {
          useChatStore.getState().replaceMessages(e.updatedMessages);
        }
        set({ isLoading: false, streamingMessageId: null });
      }),
      // ... thinking, tool_start, tool_end, segment_done
    ];
    return () => unsubs.forEach(fn => fn());
  }, [activeConversationId]);
}
```

### Testes
- [ ] Unit test Go: `SendMessage` com `conversationID=0` retorna erro
- [ ] Unit test Go: `SendMessage` com conversationID inexistente retorna erro
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
- **Mensagens só podem ser enviadas para conversas que existem.** `SendMessage` com `conversationID=0` ou ID inexistente retorna erro. A criação de conversa é responsabilidade separada e explícita.

### Envio de mensagens
- **Existe UMA única função `SendMessage` para mensagens novas no backend** (`app_chat.go` → `ChatController` → `SendMessageUseCase`). Toda mensagem nova — vinda do frontend, de canais, de deep links — passa por essa função.
- **`RetryMessage` é a única exceção permitida ao contrato acima.** Ele existe apenas para reexecutar a resposta a partir de uma mensagem de usuário já persistida, sem inserir nova linha `user` no banco. É proibido criar qualquer outro endpoint público de envio.
- **Existe UM único pipeline de envio no frontend** (chatStore). Componentes podem ter helpers para resolver contexto ou `conversationId`, mas nenhum componente, hook ou store pode criar um fluxo alternativo de envio fora de `SendMessage` para mensagem nova e `RetryMessage` para retry explícito.
- **O backend é a fonte de verdade.** O frontend não cria mensagens locais, não gera IDs temporários, não decide quando recarregar mensagens. Renderiza o que o backend emite via eventos.

### Eventos
- **Todo evento de chat carrega `conversationId`.** Sem exceções.
- **Eventos são tipados com structs/interfaces.** Nada de `map[string]interface{}` ou objetos genéricos.
- **O protocolo de eventos é o contrato central.** O backend usa eventos para orquestrar TTS, rename, notificação de canais. Alterar o schema de um evento exige atualização de todos os consumidores.

---

## Referências
- AEP-0010: Streaming Architecture
- AEP-0006: Chat Architecture Fix
- AEP-0039: Tool Calling Revamp (Fase 1 complementar)
