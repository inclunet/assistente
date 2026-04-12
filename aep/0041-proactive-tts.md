# AEP-0041: TTS Proativo (Backend-Driven)

## Status: Em implementação

## Motivação

O sistema de TTS (Text-to-Speech) atual é **frontend-driven**: o chatStore decide localmente quando falar, chama `ttsService.speakAsRole()` e resolve voz/provider no frontend. Isso traz problemas:

1. **Decisão fragmentada** — 5 chamadas a `triggerAutoRead()` espalhadas no chatStore, cada uma com condições ligeiramente diferentes.
2. **Sem suporte a canais** — Telegram, Signal e futuros canais não passam pelo frontend. Não há como gerar áudio para respostas enviadas por canais externos.
3. **Duplicação de resolução de voz** — O frontend resolve perfil → voz → provider localmente, enquanto o backend já tem toda a infraestrutura de resolução via `buildChatSpeakEvent`.
4. **Inconsistência com AEP-0040** — A AEP-0040 (backend-driven messaging) estabeleceu que o backend é a fonte de verdade para eventos de chat. O TTS deveria seguir o mesmo padrão.

## Infraestrutura existente

~80% da implementação já existe, mas as peças não estão conectadas:

| Componente | Estado | Localização |
|---|---|---|
| `DispatchSpeech` (Wails binding) | ✅ Implementado + testado | `app_speech_events.go` L108 |
| `buildChatSpeakEvent` (resolve perfil → voz → strategy) | ✅ Implementado + testado | `app_speech_events.go` L162 |
| Emissão de `chat:speak` | ✅ Implementado | `app_speech_events.go` L142 |
| `handleChatSpeak` (executor de strategies no frontend) | ✅ Implementado + testado | `frontend/src/services/chatSpeak/index.ts` L77 |
| `dispatchChatSpeech` (chamada RPC para backend) | ✅ Implementado | `frontend/src/services/chatSpeak/index.ts` L69 |
| `TTSBroker` (coordenador de áudio para canais) | ✅ Implementado + testado | `internal/messaging/tts_broker.go` |
| `triggerAutoRead` (TTS frontend-driven, legado) | ✅ Em uso | `frontend/src/store/chatStore.ts` L29 |

### O que NÃO está conectado (gaps)

| Gap | Detalhe |
|---|---|
| Nenhum listener `chat:speak` no frontend | Backend emite o evento, mas ninguém ouve |
| Backend não chama `dispatchSpeechEvent` no OnDone | `app_stream_handler.go` não dispara TTS após streaming |
| `TTSBroker` não instanciado | `NewTTSBroker()` existe mas não é chamado em produção |
| Frontend decide TTS localmente | `triggerAutoRead` em 5 pontos do chatStore |

## Fluxo de eventos (atual vs. proposto)

### Atual (frontend-driven)
```
chat:done → frontend chatStore handler
         → ttsService.isAutoReadEnabled()?
         → triggerAutoRead() → ttsService.speakAsRole()
```

### Proposto (backend-driven)
```
chat:done → (backend continua)
         → dispatchSpeechEvent(req)
         → resolve perfil → voz → strategy
         → emitter.Emit("chat:speak", event)
         → frontend listener "chat:speak"
         → handleChatSpeak(event)
         → strategy routing (none/announce/webspeech/sapi5/backend_audio)
```

### Sequência temporal de eventos

```
1. chat:messages_ready   ← msg do usuário salva
2. chat:thinking         ← modelo raciocinando
3. chat:stream           ← chunks de texto
4. chat:tool_start       ← tool call iniciada
5. chat:tool_end         ← tool call concluída
6. chat:segment_done     ← segmento agentic completo
7. chat:stream (done)    ← streaming finalizado
8. chat:done             ← resposta salva no banco
9. chat:speak            ← TTS proativo (NOVO — disparado após chat:done)
10. chat:token_stats     ← estatísticas de tokens
11. chat:context_warning ← aviso de limite
12. chat:error           ← erro (em qualquer ponto)
```

## Fases

### Fase 1 — Conectar o fluxo principal (backend → frontend)

#### 1.1 — Backend: injetar callback `OnSpeechRequest` no `agent.Service`

O `agent.Service` (que controla tanto streaming simples via `SimpleStreamHandler` quanto agentic loop) não tem acesso a `*App`. A forma de injetar a capacidade de TTS é via callback no `ServiceConfig`:

**Arquivo:** `internal/agent/service.go`

```go
// ServiceConfig
OnSpeechRequest func(conversationID uint, msgID uint, role, text, origin string, interrupt bool)
```

**Arquivo:** `app.go` — na criação do service:

```go
a.agentSvc = agent.NewService(agent.ServiceConfig{
    // ... deps existentes
    OnSpeechRequest: func(convID uint, msgID uint, role, text, origin string, interrupt bool) {
        a.dispatchSpeechEvent(ChatSpeakRequest{
            ConversationID: int(convID),
            MessageID:      int(msgID),
            Role:           role,
            Text:           text,
            Origin:         ChatSpeakOrigin(origin),
            Interrupt:       interrupt,
        })
    },
})
```

#### 1.2 — Backend: chamar `OnSpeechRequest` no `SaveAndFinish`

**Arquivo:** `internal/agent/service.go` — em `SaveAndFinish()`, após emitir `chat:done`:

```go
if s.onSpeechRequest != nil {
    go s.onSpeechRequest(conversationID, savedMsgID, "assistant", result.FullResponse, "assistant_message", true)
}
```

#### 1.3 — Backend: chamar `OnSpeechRequest` após `chat:segment_done`

**Arquivo:** `internal/agent/service.go` — em `RunAgenticLoop()`, após emitir `chat:segment_done` (L144):

```go
if s.onSpeechRequest != nil && result.FullResponse != "" {
    go s.onSpeechRequest(conversationID, 0, "assistant", result.FullResponse, "segment", false)
}
```

#### 1.4 — Frontend: adicionar listener `chat:speak` no chatStore

Dentro de `sendMessage` e `handleExternalIncoming`, junto com os outros listeners:

```typescript
const unsubSpeak = EventsOn('chat:speak', (event: ChatSpeakEvent) => {
    if (event.conversationId !== conversationId) return;
    if (!activeListeners.has(conversationIdStr)) return;
    handleChatSpeak(event);
});
```

Adicionar `unsubSpeak()` ao cleanup de ambos os fluxos.

O tipo `ChatSpeakEvent` já existe em `frontend/src/services/chatSpeak/index.ts`.

#### 1.5 — Frontend: remover `triggerAutoRead` dos handlers de eventos

Remover as 5 chamadas a `triggerAutoRead()`:
- L553 (user message em `sendMessage`)
- L694 (`chat:done` handler em `sendMessage`)
- L765 (`chat:segment_done` handler em `sendMessage`)
- L1184 (`chat:done` handler em `handleExternalIncoming`)
- L1239 (`chat:segment_done` handler em `handleExternalIncoming`)

Manter a função `triggerAutoRead` para possível uso futuro on-demand.

**Ordem recomendada:** 1.1 → 1.2 → 1.3 → 1.4 → validar → 1.5 (remover legado só após confirmar que backend-driven funciona).

### Fase 2 — Mensagens do usuário

#### 2.1 — Backend: disparar para mensagens do usuário

No `Interactor.RecordUserMessage()` (`internal/chat/interactor.go`), após emitir `chat:messages_ready`:

Opção: Injetar callback `OnSpeechRequest` no `Interactor` ou emitir o evento de TTS direto no `App` layer (onde `a.dispatchSpeechEvent` está disponível).

#### 2.2 — Frontend: remover `triggerAutoRead` da mensagem do usuário

Já coberto pela Fase 1.5 (remoção da chamada L553).

### Fase 3 — TTSBroker para canais externos (futuro)

#### 3.1 — Instanciar TTSBroker no App

Adicionar campo `ttsBroker *messaging.TTSBroker` ao `App` struct e instanciar no `startup()`.

#### 3.2 — Integrar no flow de notificação de canais

No `responseNotifier.Notify()`:
1. `ttsBroker.Prepare(savedMsgID)` antes de salvar
2. Goroutine: gerar TTS → `ttsBroker.Publish(savedMsgID, audio, mimeType)`
3. Gateway callback: `ttsBroker.Wait(savedMsgID, 5s)` para obter o áudio

### Fase 4 — Configurabilidade e acessibilidade

#### 4.1 — Verificar resolução de strategy

`buildChatSpeakEvent` já resolve:
- TTS desabilitado → strategy `none`
- TTS habilitado, auto-read desabilitado → strategy `announce`
- TTS habilitado, auto-read habilitado → `webspeech`/`sapi5`/`backend_audio`

Validar cada cenário com testes.

#### 4.2 — Garantir `announce()` para acessibilidade

`handleChatSpeak` atualmente **NÃO** chama `announce()` para as strategies `backend_audio`, `webspeech` e `sapi5`. Quando TTS funciona, o screen reader não é notificado. Corrigir para chamar `announce()` em **todas** as strategies (não só na strategy `announce`).

### Fase 5 — Limpar código legado

#### 5.1 — Remover `appStreamHandler` (dead code)

O `appStreamHandler` em `app_stream_handler.go` é legado — todo o streaming passa pelo `agent.Service` (via `SimpleStreamHandler` ou `AgenticStreamHandler`). Ele não é mais instanciado. Verificar que não há referências ativas e remover.

### Fase 6 — Testes

#### 6.1 — Go
- `SaveAndFinish` chama `OnSpeechRequest` callback com parâmetros corretos
- `RunAgenticLoop` chama `OnSpeechRequest` após `chat:segment_done`
- Verificar que callback nil não causa panic

#### 6.2 — Frontend (Vitest)
- Listener `chat:speak` no chatStore invoca `handleChatSpeak`
- `triggerAutoRead` não é mais chamado nos handlers de eventos
- Regressão: play button (on-demand) continua funcionando

#### 6.3 — E2E (Playwright)
- `chat:speak` emitido após stream completion
- Áudio executado no frontend após `handleChatSpeak`

## Riscos e mitigações

| Risco | Mitigação |
|---|---|
| Duplo TTS durante migração | Fase 1.4 (remover legado) só após validar backend-driven |
| Latência: backend resolve perfil a cada mensagem | Perfil já é resolvido no handler; cache por conversa se necessário |
| Race: `chat:speak` chega antes de `chat:done` | `dispatchSpeechEvent` é chamado **após** `chat:done`, em goroutine separada |
| Regressão no play button (on-demand) | Path separado do `chat:speak`; `triggerAutoRead` mantida para on-demand |
| SAPI5 `isSpeaking` stale data | Fix do isSpeaking (AEP-0028 Fase 2B) já aplicado |

## Referências

- AEP-0028: Speech/TTS Refactor (auditoria + correções estruturais)
- AEP-0040: Backend-Driven Messaging (padrão de eventos backend → frontend)
- `app_speech_events.go` — dispatcher e event builder
- `internal/messaging/tts_broker.go` — coordenador de áudio para canais
- `frontend/src/services/chatSpeak/` — handler de strategies no frontend
