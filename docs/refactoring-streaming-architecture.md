# Refatoração: Arquitetura de Streaming e Mensagens

## Problema Atual

A arquitetura atual tem responsabilidades mal divididas entre frontend e backend:

### Frontend (Chat.svelte) - Fazendo demais:
- Cria mensagens locais de placeholder para streaming
- Tenta sincronizar estado local com múltiplos eventos do backend
- Organiza hierarquia de threads (deveria vir pronta)
- Monta contexto para enviar ao LLM
- Gerencia múltiplos listeners de eventos

### Backend - Fragmentado:
- Emite múltiplos eventos (`chat:chunk`, `chat:message_saved`, `chat:message_updated`)
- Salva mensagens em momentos diferentes (OnDone)
- Não retorna dados estruturados

### Resultado:
- Sincronização impossível entre estado local e banco
- Mensagens duplicadas/fora de ordem
- Threads não funcionam corretamente ao recarregar

---

## Nova Arquitetura Proposta

### Princípio: Backend como Fonte Única da Verdade

```
┌─────────────────────────────────────────────────────────────┐
│                        FRONTEND                              │
│  - Apenas renderiza o que recebe                            │
│  - NÃO cria mensagens                                       │
│  - NÃO monta contexto                                       │
│  - Atualiza por messageId                                   │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                        BACKEND                               │
│  - Gerencia TODO o estado de mensagens                      │
│  - Monta contexto para LLM                                  │
│  - Organiza threads/hierarquia                              │
│  - Emite eventos simples com messageId                      │
└─────────────────────────────────────────────────────────────┘
```

---

## Mudanças Detalhadas

### 1. Novo Endpoint: GetConversationWithThreads

```go
type MessageNode struct {
    Message  ChatMessage   `json:"message"`
    Children []MessageNode `json:"children,omitempty"`
    Level    int           `json:"level"`
}

type ConversationWithThreads struct {
    ID                   uint          `json:"id"`
    Title                string        `json:"title"`
    Model                string        `json:"model"`
    ShowInternalMessages bool          `json:"show_internal_messages"`
    Threads              []MessageNode `json:"threads"` // Já organizado!
}

func (a *App) GetConversationWithThreads(id uint) (*ConversationWithThreads, error) {
    conv, err := database.GetConversation(id)
    if err != nil {
        return nil, err
    }
    
    threads := buildMessageTree(conv.Messages)
    
    return &ConversationWithThreads{
        ID:          conv.ID,
        Title:       conv.Title,
        Preferences: conv.GetPreferences(), // JSON com model, show_internal_messages, etc.
        Threads:     threads,
    }, nil
}
```

### 2. Novo Sistema de Streaming

#### Eventos Simplificados:

```go
// ÚNICO evento de streaming - sempre inclui messageId
type StreamEvent struct {
    MessageID uint   `json:"messageId"` // ID da mensagem no banco
    Content   string `json:"content"`   // Conteúdo acumulado
    Done      bool   `json:"done"`      // Se terminou
    Error     string `json:"error,omitempty"`
}

// Emitido durante streaming
runtime.EventsEmit(ctx, "chat:stream", StreamEvent{
    MessageID: msgID,
    Content:   accumulatedContent,
    Done:      false,
})

// Emitido quando termina
runtime.EventsEmit(ctx, "chat:stream", StreamEvent{
    MessageID: msgID,
    Content:   fullContent,
    Done:      true,
})
```

#### Fluxo de Envio de Mensagem:

```go
func (a *App) SendMessage(conversationID uint, content string, media string, params ChatParams) error {
    // 1. Cria conversa se necessário
    if conversationID == 0 {
        conv := database.CreateConversation(...)
        conversationID = conv.ID
        runtime.EventsEmit(ctx, "chat:conversation_created", conv)
    }
    
    // 2. Salva mensagem do usuário no banco
    userMsg := database.AddMessage(conversationID, "user", content)
    
    // 3. Cria mensagem vazia do assistant (placeholder no banco)
    assistantMsg := database.AddMessage(conversationID, "assistant", "")
    
    // 4. Emite evento informando que mensagens foram criadas
    runtime.EventsEmit(ctx, "chat:messages_added", []uint{userMsg.ID, assistantMsg.ID})
    
    // 5. Inicia streaming com o messageId do assistant
    go a.streamResponse(conversationID, assistantMsg.ID, params)
    
    return nil
}
```

### 3. Frontend Simplificado

```javascript
// Estado
let conversation = null;  // ConversationWithThreads
let streamingMessageId = null;
let streamingContent = '';

// Carrega conversa (já vem estruturada)
async function loadConversation(id) {
    conversation = await GetConversationWithThreads(id);
}

// Listener único de streaming
EventsOn('chat:stream', (event) => {
    streamingMessageId = event.messageId;
    streamingContent = event.content;
    
    if (event.done) {
        // Recarrega do banco para ter dados finais
        loadConversation(conversation.id);
        streamingMessageId = null;
        streamingContent = '';
    }
});

// Listener para novas mensagens
EventsOn('chat:messages_added', (messageIds) => {
    // Recarrega conversa para incluir novas mensagens
    loadConversation(conversation.id);
});

// Renderização: usa conversation.threads diretamente
// Mensagem em streaming: sobrescreve content da mensagem com streamingMessageId
```

---

## Plano de Implementação

### Fase 1: Backend - Estrutura de Dados (Estimativa: 1h) ✅ CONCLUÍDA
- [x] Criar tipos `MessageNode` e `ConversationWithThreads` (`app.go`)
- [x] Implementar função `buildMessageTree(messages []ChatMessage) []MessageNode` (`db.go`)
- [x] Criar endpoint `GetConversationWithThreads` (`db.go`)
- [x] Testar com dados existentes

### Fase 2: Backend - Streaming Simplificado (Estimativa: 2h) ✅ CONCLUÍDA
- [x] Criar tipo `StreamEvent` (`app.go`)
- [x] Modificar `SendMessage` para criar mensagem do assistant antes de streamar (`llm.go`)
- [x] Modificar `appStreamHandler` para emitir eventos com `messageId` (`llm.go`)
- [x] Manter `chat:stream` como evento principal de streaming

### Fase 3: Frontend - Simplificação (Estimativa: 2h) ✅ CONCLUÍDA
- [x] Adicionar import de `GetConversationWithThreads` (`Chat.svelte`)
- [x] Criar `reloadConversation()` e `extractMessagesFromThreads()` (`Chat.svelte`)
- [x] Criar `updateMessageContent()` para streaming (`Chat.svelte`)
- [x] Simplificar `handleSubmit` para não criar placeholders locais
- [x] Atualizar listeners para `chat:messages_ready` e `chat:stream`
- [x] Atualizar `loadConversation` para usar `GetConversationWithThreads`

### Fase 4: Limpeza (Estimativa: 30min) 🔄 EM ANDAMENTO
- [x] Deprecar função `reloadMessagesFromDB`
- [ ] Testar fluxo completo
- [ ] Remover código morto após testes

---

## Benefícios

1. **Previsibilidade**: Frontend só renderiza, não gerencia estado
2. **Simplicidade**: Um único evento de streaming
3. **Consistência**: Dados sempre vêm do banco, não de estado local
4. **Threads funcionais**: Hierarquia calculada no backend, sempre correta
5. **Manutenibilidade**: Menos código, menos bugs

---

## Riscos e Mitigações

| Risco | Mitigação |
|-------|-----------|
| Performance ao recarregar conversa | Cache no frontend, recarrega só quando necessário |
| Latência no início do streaming | Mensagem já existe no banco, frontend só atualiza |
| Compatibilidade com conversas antigas | buildMessageTree funciona com parent_id=null |

---

## Arquivos Afetados

### Backend (Go):
- `app.go` - Novo endpoint GetConversationWithThreads
- `llm.go` - Refatorar streaming
- `internal/database/database.go` - Nova função buildMessageTree (opcional, pode ficar em app.go)

### Frontend (Svelte):
- `frontend/src/components/Chat.svelte` - Simplificação massiva

---

## Próximos Passos

1. Aprovar este plano
2. Implementar Fase 1 (estrutura de dados)
3. Testar isoladamente
4. Continuar com fases seguintes

