# AEP-0041: TTS Proativo (Backend-Driven)

## Status: Implementado (Fases 1–8)

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
         → strategy routing (none/announce/webspeech/backend_audio)
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
8. chat:speak            ← TTS proativo (NOVO — disparado ANTES de chat:done)
9. chat:done             ← resposta salva no banco
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
OnSpeechRequest func(conversationID uint, msgID uint, role, text, origin, profileSlug string, interrupt bool)
```

**Arquivo:** `app.go` — na criação do service:

```go
a.agentSvc = agent.NewService(agent.ServiceConfig{
    // ... deps existentes
    OnSpeechRequest: func(convID uint, msgID uint, role, text, origin, profileSlug string, interrupt bool) {
        a.dispatchSpeechEvent(ChatSpeakRequest{
            ConversationID: convID,
            MessageID:      msgID,
            ProfileSlug:    profileSlug,
            Role:           role,
            Text:           text,
            Origin:         ChatSpeakOrigin(origin),
            Interrupt:       &interrupt,
        })
    },
})
```

#### 1.2 — Backend: chamar `OnSpeechRequest` no `SaveAndFinish`

**Arquivo:** `internal/agent/service.go` — em `SaveAndFinish()`, de forma síncrona e **antes** de emitir `chat:done` (para garantir que o frontend ainda tenha listeners ativos quando receber `chat:speak`):

```go
if s.onSpeechRequest != nil {
    s.onSpeechRequest(conversationID, savedMsgID, "assistant", result.FullResponse, "assistant_message", profileSlug, true)
}
```

#### 1.3 — Backend: chamar `OnSpeechRequest` após `chat:segment_done`

**Arquivo:** `internal/agent/service.go` — em `RunAgenticLoop()`, após emitir `chat:segment_done` (L144):

```go
if s.onSpeechRequest != nil && result.FullResponse != "" {
    s.onSpeechRequest(conversationID, 0, "assistant", result.FullResponse, "segment", params.ProfileSlug, false)
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

### Fase 3 — TTSBroker para canais externos

#### 3.1 — TTSBroker integrado ao Gateway

O `TTSBroker` é criado internamente pelo `Gateway` (via `NewGateway`). Coordena a síntese de áudio
com timeout para evitar bloqueio indefinido se a API TTS estiver lenta.

**Arquivo:** `internal/messaging/gateway.go`

- `TTSBroker` é campo privado do Gateway, criado automaticamente
- No callback de resposta, o fluxo usa `Prepare → goroutine(synthesizeTTS → Publish/Cancel) → Wait(5s)`
- Se Wait retorna áudio: envia áudio + salva no DB
- Se timeout ou Cancel: envia texto (fallback)

#### 3.2 — Fix de leak no Wait

**Arquivo:** `internal/messaging/tts_broker.go`

`Wait()` agora limpa o slot do mapa ao dar timeout, prevenindo leak de memória.
Se a goroutine de TTS chamar `Publish` após o timeout, é no-op (slot já removido).

### Fase 4 — Configurabilidade e acessibilidade

#### 4.1 — Verificar resolução de strategy

`buildChatSpeakEvent` já resolve:
- TTS desabilitado → strategy `none`
- TTS habilitado, auto-read desabilitado → strategy `announce`
- TTS habilitado, auto-read habilitado → `webspeech`/`backend_audio` (incluindo SAPI5 via `providerId="sapi5"`)

Validar cada cenário com testes.

#### 4.2 — Comportamento de `announce()` para acessibilidade

Decisão de design: `handleChatSpeak` **não** chama `announce()` para `backend_audio` e `webspeech` quando o TTS é executado com sucesso, para evitar dupla reprodução (TTS + leitor de tela). `announce()` ocorre apenas quando a strategy resolvida é `announce` e como fallback de acessibilidade quando `backend_audio` falha (`played=false`). Qualquer ampliação desse comportamento para outras strategies deve ser tratada como discussão futura fora do escopo desta AEP.

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

### Fase 8 — Cleanup do pacote `internal/speech/`

Auditoria e remoção de dead code, código legacy e práticas questionáveis no pacote speech.

#### 8.1 — Dead code confirmado (já removido)

- `GetTTSModels` binding + controller + service + `FetchTTSModels` + `staticTTSModels` + `StaticTTSModels()`

#### 8.2 — Dead code SpeechManager (remoção nesta fase)

| Função | Motivo |
|---|---|
| `SpeechManager.GetAvailableSTTProviders()` | 0 callers — lógica substituída pelo Service |
| `SpeechManager.GetAvailableTTSProviders()` | 0 callers — lógica substituída pelo Service |
| `SpeechManager.GetAvailableTTSVoices()` | 0 callers — `GetTTSVoices` no Service faz isso diretamente |
| `SpeechManager.GetOpenAIVoices()` | 0 callers — wrapper puro de `GetAvailableVoices()` |
| `stringToUTF16()` em `sapi5_windows.go` | 0 callers — comentário diz "not used" |

#### 8.3 — Refactor de manutenibilidade (executado)

- **Tipo duplicado eliminado**: `TTSVoiceEntry` removido — `GetTTSVoices` agora retorna `[]TTSVoiceInfo` diretamente, sem conversão manual
- **God method split**: `SpeakMessage` (~75 linhas) decomposto em `synthesizeForProvider` → `synthesizeSAPI5` / `synthesizeAPI`
- **Parâmetros excessivos**: `SpeakPreview` 7 params → `SpeakPreviewParams` struct
- **Dead code legacy**: `PreviewVoiceSettings` removido (frontend usa `SpeakPreview`)
- **Naming fix**: `providerId` → `providerID` no binding `SpeakPreview`
- **Logs verbosos hot path**: 6× `log.Printf` removidos de `SpeakMessage` (chamado a cada mensagem)

#### 8.4 — Observações para futuro (fora do escopo deste PR)

- **God method**: `SynthesizeToBytes()` (~70 linhas) em sapi5_windows.go — COM init + voice select + temp file — difícil de testar fora do Windows
- **Erros silenciados**: SAPI5 rate/volume `PutProperty` ignorados; `FetchVoices` mascara HTTP error como nil
- **Dead code provável**: `OpenAIProvider` frontend + 5 bindings legacys (`SynthesizeOpenAIWithVoice`, `SetOpenAITTSVoice`, etc.) — dependem de migração completa para `SpeakMessage`
- Áudio executado no frontend após `handleChatSpeak`

## Riscos e mitigações

| Risco | Mitigação |
|---|---|
| Duplo TTS durante migração | Fase 1.4 (remover legado) só após validar backend-driven |
| Latência: backend resolve perfil a cada mensagem | Perfil já é resolvido no handler; cache por conversa se necessário |
| Race: `chat:speak` chega depois do cleanup | `dispatchSpeechEvent` é chamado **antes** de `chat:done`, de forma síncrona |
| Regressão no play button (on-demand) | Path separado do `chat:speak`; `triggerAutoRead` mantida para on-demand |
| SAPI5 `isSpeaking` stale data | Fix do isSpeaking (AEP-0028 Fase 2B) já aplicado |
| Latência SAPI5 via WAV (Fase 7) | ~50-200ms para escrita WAV + leitura + base64 — imperceptível na prática |

### Fase 7 — Unificação SAPI5 → backend_audio

#### Motivação

SAPI5 usa uma pipeline completamente separada: o frontend orquestra fala direta nos
alto-falantes via COM, com polling de `IsSpeaking()`. Isso impede cache, controle completo
de playback (pause/resume), e mantém uma strategy e provider frontend dedicados.

A API COM SAPI5 suporta redirecionar output para `SpFileStream`, gerando WAV em arquivo.
Com isso, SAPI5 pode usar a mesma pipeline que provedores API (backend_audio):
  backend gera bytes → base64 → frontend reproduz via `HTMLAudioElement`.

#### 7.1 — Backend: `SAPI5Manager.SynthesizeToBytes`

**Arquivo:** `internal/speech/sapi5_windows.go`

Novo método que redireciona `SpVoice.AudioOutputStream` para um `SpFileStream` temporário:

```go
func (m *SAPI5Manager) SynthesizeToBytes(text, voiceName string, rate, volume int) ([]byte, error)
```

Passos:
1. Seleciona voz (se especificada)
2. Configura rate e volume
3. Cria `SAPI.SpFileStream` → abre temp WAV (SSFMCreateForWrite=3)
4. Redireciona `SpVoice.AudioOutputStream` para o SpFileStream
5. `SpVoice.Speak(text, 0)` — síncrono, escreve no arquivo
6. Fecha SpFileStream, restaura output padrão
7. Lê bytes do WAV, remove temp file, retorna `[]byte`

**Arquivo:** `internal/speech/sapi5_other.go` — stub retorna erro (`"SAPI5 indisponível nesta plataforma"`).

#### 7.2 — Backend: rotear SAPI5 em `SpeakMessage`

**Arquivo:** `internal/speech/service.go`

Em `SpeakMessage()`, quando `providerID == "sapi5"`:
- Chama `GetSAPI5Manager().SynthesizeToBytes(content, voiceID, rate, volume)`
- Encoda em base64, salva no DB, retorna `AudioResult{audio, "audio/wav"}`
- **Mesmo path de cache** que provedores API (DB hit → skip síntese)

#### 7.3 — Backend: strategy SAPI5 → backend_audio

**Arquivo:** `app_speech_events.go`

Em `buildChatSpeakEvent`, quando `cfg.Provider == "sapi5"`:
- Emite `strategy: "backend_audio"` (com `fallbackStrategy: "announce"`)
- `ProviderID` = `"sapi5"` (identificador usado em SpeakMessage para rotear)

#### 7.4 — Frontend: sem mudanças

O frontend não precisa de alterações:
- `handleChatSpeak` com `strategy=backend_audio` já chama `messageAudioService.speakMessage()`
- `SpeakMessage` Wails RPC passa `providerId="sapi5"` ao backend, que roteia internamente
- Cache LRU + DB funciona automaticamente
- Playback via `HTMLAudioElement` com controle completo

#### Benefícios

| Aspecto | Antes (strategy=sapi5) | Depois (backend_audio) |
|---|---|---|
| Pipeline | Separada (COM → speakers) | Unificada (COM → WAV → base64 → HTMLAudio) |
| Cache | Nenhum | DB + memória LRU (replay instantâneo) |
| Playback control | Limitado (stop, volume, rate) | Completo (pause, resume, seek, volume) |
| Frontend code | SAPI5Provider + polling | Nenhum código SAPI5 no frontend |
| Latência | ~10ms (COM direto) | ~50-200ms (WAV + IPC) |

## Referências

- AEP-0028: Speech/TTS Refactor (auditoria + correções estruturais)
- AEP-0040: Backend-Driven Messaging (padrão de eventos backend → frontend)
- `app_speech_events.go` — dispatcher e event builder
- `internal/messaging/tts_broker.go` — coordenador de áudio para canais
- `frontend/src/services/chatSpeak/` — handler de strategies no frontend
