# AEP-0041: TTS Proativo (Backend-Driven)

## Status: In Progress — fluxo vigente entregue; regressão E2E planejada permanece pendente

## Motivação (baseline histórico)

Antes desta AEP, o sistema de TTS era **frontend-driven**: o chatStore decidia
localmente quando falar, chamava `ttsService.speakAsRole()` e resolvia
voz/provider no frontend. Isso trazia os problemas abaixo:

1. **Decisão fragmentada** — 5 chamadas a `triggerAutoRead()` espalhadas no chatStore, cada uma com condições ligeiramente diferentes.
2. **Sem suporte a canais** — Telegram, Signal e futuros canais não passam pelo frontend. Não há como gerar áudio para respostas enviadas por canais externos.
3. **Duplicação de resolução de voz** — O frontend resolve perfil → voz → provider localmente, enquanto o backend já tem toda a infraestrutura de resolução via `buildChatSpeakEvent`.
4. **Inconsistência com AEP-0040** — A AEP-0040 (backend-driven messaging) estabeleceu que o backend é a fonte de verdade para eventos de chat. O TTS deveria seguir o mesmo padrão.

## Estado implementado

| Componente | Evidência |
|---|---|
| Dispatch e construção de `chat:speak` | `internal/app/app_speech_events.go` e `app_speech_events_language_test.go` |
| Callback de conclusão do agentic loop | `internal/agent/service.go`, `agentic_loop.go`, `service_speech_test.go` e `agentic_loop_test.go` |
| Listener e arbitragem no frontend | `frontend/src/services/chatEventController.ts` e `chatEventController.test.ts` |
| Broker para canais externos | `internal/messaging/tts_broker.go` e `tts_broker_test.go` |

Os gaps abaixo descreviam o baseline anterior à implementação: ausência de
listener, callback não conectado, broker sem wiring e decisão local duplicada.
Esses caminhos foram substituídos pelos componentes e testes da tabela acima.

## Fluxo de eventos

### Histórico, antes da implementação (frontend-driven)
```
chat:done → frontend chatStore handler
         → ttsService.isAutoReadEnabled()?
         → triggerAutoRead() → ttsService.speakAsRole()
```

### Vigente (backend-driven)
```
chat:stream(done) → OnSpeechRequest síncrono
                  → dispatchSpeechEvent(req)
         → resolve perfil → voz → strategy
         → emitter.Emit("chat:speak", event)
         → frontend listener "chat:speak"
         → handleChatSpeak(event)
         → strategy routing (none/announce/webspeech/backend_audio)
         → retorno ao backend
         → chat:done
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
8. chat:speak            ← TTS proativo disparado antes de chat:done
9. chat:done             ← resposta salva no banco
10. chat:token_stats     ← estatísticas de tokens
11. chat:context_warning ← aviso de limite
12. chat:error           ← erro (em qualquer ponto)
```

## Fases

### Fase 1 — Conectar o fluxo principal (backend → frontend) ✅

#### 1.1 — Backend: injetar callback `OnSpeechRequest` no `agent.Service`

O `agent.Service` (que controla tanto streaming simples via `SimpleStreamHandler` quanto agentic loop) não tem acesso a `*App`. A forma de injetar a capacidade de TTS é via callback no `ServiceConfig`:

**Arquivo:** `internal/agent/service.go`

```go
// ServiceConfig
OnSpeechRequest func(conversationID string, msgID string, role, text, origin, profileSlug string, interrupt bool)
```

**Arquivo:** `app.go` — na criação do service:

```go
a.agentSvc = agent.NewService(agent.ServiceConfig{
    // ... deps existentes
    OnSpeechRequest: func(convID string, msgID string, role, text, origin, profileSlug string, interrupt bool) {
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
    s.onSpeechRequest(conversationID, "", "assistant", result.FullResponse, "segment", params.ProfileSlug, false)
}
```

#### 1.4 — Frontend: listener global em `chatEventController`

O listener vigente é registrado uma vez em
`frontend/src/services/chatEventController.ts`, filtra/arbitra a origem da
superfície e delega para `handleChatSpeak`. Não há listener por envio nem
ownership de TTS no `chatStore`.

```typescript
const unsubSpeak = EventsOn('chat:speak', (event: ChatSpeakEvent) => {
    if (event.conversationId !== conversationId) return;
    if (!activeListeners.has(conversationIdStr)) return;
    handleChatSpeak(event);
});
```

O tipo `ChatSpeakEvent` já existe em `frontend/src/services/chatSpeak/index.ts`.

#### 1.5 — Frontend: fluxo legado removido

As decisões automáticas locais do antigo `chatStore` foram removidas. Fala
manual/on-demand usa o serviço próprio (`useTTS`/`SpeakMessage`) e não reutiliza
um `triggerAutoRead` de evento.

### Fase 2 — Mensagens do usuário ✅

#### 2.1 — Backend: disparar para mensagens do usuário

O caminho vigente está em `internal/core/usecases/send_message.go`: após
persistir a mensagem do usuário, o use case chama `OnSpeechRequest` com
`conversationID`, `userMsg.ID`, perfil e origem `user_message`.

#### 2.2 — Frontend reativo

O frontend apenas recebe `chat:speak` pelo `chatEventController`; não decide
localmente se a mensagem do usuário deve ser falada.

### Fase 3 — TTSBroker para canais externos ✅

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

### Fase 4 — Configurabilidade e acessibilidade ✅

#### 4.1 — Verificar resolução de strategy

`buildChatSpeakEvent` já resolve:
- TTS desabilitado (ou `provider: "disabled"`) → strategy `announce`. O leitor de
  telas continua lendo a resposta pela live region; desligar a voz sintética não
  pode silenciar a acessibilidade.
- TTS habilitado, auto-read desabilitado → strategy `announce`
- TTS habilitado, auto-read habilitado → `webspeech`/`backend_audio` (incluindo SAPI5 via `providerId="sapi5"`)

Validar cada cenário com testes.

#### 4.2 — Comportamento de `announce()` para acessibilidade

Decisão de design: `handleChatSpeak` **não** chama `announce()` para `backend_audio` e `webspeech` quando o TTS é executado com sucesso, para evitar dupla reprodução (TTS + leitor de tela). `announce()` ocorre apenas quando a strategy resolvida é `announce` e como fallback de acessibilidade quando `backend_audio` falha (`played=false`). Qualquer ampliação desse comportamento para outras strategies deve ser tratada como discussão futura fora do escopo desta AEP.

#### 4.3 — Fonte única da fala do conteúdo do assistente

**Decisão:** o `chat:speak` emitido pelo backend é a **única** fonte da fala do
conteúdo do assistente, tanto para a síntese de voz quanto para a live region do
leitor de telas. O backend fala o texto **daquele** segmento a cada
`chat:segment_done` (`agentic_loop.go`) e apenas o **último** segmento ao fim do
turno (`agent/service.go`).

Proibições que decorrem da decisão:

- Componentes de mensagem (`ChatMessage`) **não** anunciam conteúdo em live
  region. Eles apenas renderizam e montam `aria-label`.
- O frontend **não** difere texto acumulado de streaming contra o que já foi
  falado. Como `stripMarkdown` reescreve o prefixo retroativamente sempre que uma
  marcação fecha (`**x**`, `` `x` ``, ` ``` `, `[x](y)`), qualquer diff local
  degenera em releitura do texto inteiro numa live region `aria-atomic="true"`.
- Anúncios de **estado** (respondendo, raciocinando, ferramenta em execução,
  turno agêntico concluído sem texto) continuam no frontend, no
  `chatEventController`, porque não são conteúdo de mensagem.

#### 4.4 — Rótulo de bloco de código localizado

O texto falado substitui fences de código por um marcador curto. Esse marcador é
localizado a partir de `profile.Input.Language` via
`textutil.CodeBlockSpeechLabel` (pt/es/en, inglês como fallback), e aplicado por
`textutil.StripMarkdownForSpeechLabeled`. Como o strip depende do perfil,
`dispatchSpeechEvent` resolve o perfil **antes** de limpar o Markdown.

O idioma resolvido viaja no evento em `speechLanguage`. A strategy
`backend_audio` não reaproveita o texto já limpo: ela regenera o áudio pelo
`SpeakMessage`, que relê a mensagem persistida. Sem o idioma no evento, esse
caminho cairia no perfil ativo e falaria o marcador no idioma errado quando a
conversa usa outro perfil. Pela mesma razão o gateway de mensageria resolve o
idioma **por canal**, casando com o perfil que sintetiza o áudio do canal.

### Fase 5 — Limpar código legado ✅

#### 5.1 — Remover `appStreamHandler` (dead code)

O `appStreamHandler` em `app_stream_handler.go` é legado — todo o streaming passa pelo `agent.Service` (via `SimpleStreamHandler` ou `AgenticStreamHandler`). Ele não é mais instanciado. Verificar que não há referências ativas e remover.

### Fase 6 — Testes 🚧

#### 6.1 — Go
- `SaveAndFinish` chama `OnSpeechRequest` callback com parâmetros corretos
- `RunAgenticLoop` chama `OnSpeechRequest` após `chat:segment_done`
- Verificar que callback nil não causa panic

#### 6.2 — Frontend (Vitest)
- `chatEventController.test.ts` cobre listener, origem e `handleChatSpeak`
- handlers não executam decisão local de auto-read
- Regressão: play button (on-demand) continua funcionando

#### 6.3 — E2E (Playwright)
- `chat:speak` emitido após stream completion

### Fase 8 — Cleanup do pacote `internal/speech/` ✅

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
| Regressão no play button (on-demand) | Path separado do `chat:speak`, coberto por `useTTS`/`SpeakMessage` |
| SAPI5 `isSpeaking` stale data | Fix do isSpeaking (AEP-0024 apêndice Speech/TTS Refactor, Fase 2B) já aplicado |
| Latência SAPI5 via WAV (Fase 7) | ~50-200ms para escrita WAV + leitura + base64 — imperceptível na prática |

### Fase 7 — Unificação SAPI5 → backend_audio ✅

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

## Critérios de aceitação e evidências

- [x] `OnSpeechRequest` é síncrono e precede `chat:done`:
  `internal/agent/service_speech_test.go` e `agentic_loop_test.go`.
- [x] Dispatcher resolve perfil, idioma e emite `chat:speak`:
  `internal/app/app_speech_events_language_test.go`.
- [x] Frontend roteia `chat:speak` pelas strategies vigentes:
  `frontend/src/services/chatEventController.test.ts` e
  `frontend/src/services/chatSpeak/index.test.ts`.
- [x] Canais externos usam broker compartilhado:
  `internal/messaging/tts_broker_test.go`.
- [x] Configuração, fala manual e playback permanecem cobertos por
  `frontend/src/hooks/useTTS.test.tsx` e `ChatMessage.test.tsx`.
- [x] Cleanup e pipeline de speech possuem regressões em
  `internal/speech/*_test.go` e `internal/app/app_speech_provider_test.go`.
- [ ] Adicionar regressão Playwright para a sequência completa
  `chat:stream(done) → chat:speak → chat:done`; não há teste E2E focado
  encontrado no HEAD.

## Referências

- AEP-0024 (apêndice): Speech/TTS Refactor (auditoria + correções estruturais)
- AEP-0040: Backend-Driven Messaging (padrão de eventos backend → frontend)
- `app_speech_events.go` — dispatcher e event builder
- `internal/messaging/tts_broker.go` — coordenador de áudio para canais
- `frontend/src/services/chatSpeak/` — handler de strategies no frontend
