# AEP-0028: Speech/TTS Refactor — Auditoria Completa

## Contexto

Auditoria profunda do subsistema Speech/TTS identificou 61 issues:
- 8 CRITICAL, 20 HIGH, 17 MEDIUM, 16 LOW
- Cobertura de testes: ~10-15% do subsistema

## Plano de Fases

### Fase 1A — Backend: Constantes + Deduplicação openai_tts.go ✅
- [x] Extrair constantes: `TtsMaxChunkSize`, `TtsStreamBufSize`, `TtsTimeoutBase`, `TtsTimeoutPerChunk`
- [x] Consolidar `Synthesize` + `SynthesizeWithVoice` → `synthesizeInternal(text, voice)`
- [x] Consolidar `SynthesizeStream` + `SynthesizeStreamWithVoice` → `synthesizeStreamInternal(ctx, text, voice, callbacks)`
- [x] Manter métodos públicos como thin wrappers (API compatível)
- [x] `GetAvailableVoices` → var estática (evitar rebuild do slice)

### Fase 1B — Backend: calcTTSTimeout + Error Handling + profile_factory ✅
- [x] Extrair `CalcTTSTimeout(textLen int) time.Duration`
- [x] Usar em `app_speech.go` (2 locais) e `index.ts` frontend (2 locais)
- [x] `profile_factory.go`: retornar erro estruturado quando credenciais falham
- [x] Refatorar `SpeakPreview` em sub-funções (`previewSAPI5`, `previewLLM`)
- [x] Wrap errors com contexto em `app_speech.go` (pattern `fmt.Errorf("op: %w", err)`)

### Fase 2A — Frontend: TTSConfig unificado + base64 util ✅
- [x] Remover `TTSConfig` de `index.ts`, re-exportar de `types.ts`
- [x] Adicionar `enabledForUser` a `types.ts`
- [x] Extrair `base64ToBlob(base64: string, mimeType: string): Blob` em `lib/audioUtils.ts`
- [x] Usar em `openai.ts`, `streamPlayer.ts`, `messageAudio/index.ts`

### Fase 2B — Frontend: Race Conditions ✅
- [x] `speakWithOverride`: mutex `overrideLock` para evitar mutação concorrente
- [x] `openai.ts` pendingSynthesis: `rejectPendingSynthesis` pattern no stop()
- [x] `sapi5.ts` isSpeaking: campo `_isSpeaking` gerenciado pelo polling

### Fase 2C — Frontend: StreamPlayer + Auto-read ✅
- [x] `streamPlayer.ts`: cancelar timeout pendente no cleanup/dispose
- [x] `streamPlayer.ts`: invalidar singleton no dispose (`globalStreamPlayer = null`)
- [x] Remover latência calculada e descartada (`void latency`)
- [x] Extrair `triggerAutoRead(text, role)` helper no `chatStore.ts`
- [x] Garantir `stripMarkdown` consistente em todos os paths
- [x] `messageAudioService.saveAudioToDB`: logar warning em vez de engolir

### Fase 3 — Acoplamento: Event Constants + Pitch Pipeline ✅
- [x] Criar `frontend/src/lib/speechEvents.ts` com constantes de eventos
- [x] Criar `internal/speech/events.go` com constantes correspondentes
- [x] Adicionar `pitch` a `RoleVoiceConfig` (frontend + backend `speech.RoleVoiceConfig`)
- [x] Propagar pitch em `profile_factory.go` `buildRoleConfig`
- [x] Propagar pitch em `useInteractionProfile.ts` `makeRoleConfig`
- [x] `TTSControls.tsx`: fix tabIndex para acessibilidade

### Fase 4 — Testes Prioritários ✅
- [x] `internal/speech/openai_tts_test.go`: constantes, CalcTTSTimeout, buildParams, GetAvailableVoices, pitch
- [x] `frontend/src/lib/audioUtils.test.ts`: base64ToBlob, base64ToBytes, calcTTSTimeoutMs
- [x] `frontend/src/lib/speechEvents.test.ts`: constantes de eventos TTS

## Critério de Sucesso
- ✅ Todos os testes existentes (973 frontend + Go) continuam passando
- ✅ Novos testes cobrem funções críticas (18 novos testes)
- ✅ Zero duplicação nos métodos Synthesize
- ✅ TTSConfig com definição única
- ✅ Race conditions eliminadas

## PR: https://github.com/inclunet/assistente/pull/60
