# AEP-0028: Speech/TTS Refactor — Auditoria Completa

## Contexto

Auditoria profunda do subsistema Speech/TTS identificou 61 issues:
- 8 CRITICAL, 20 HIGH, 17 MEDIUM, 16 LOW
- Cobertura de testes: ~10-15% do subsistema

## Plano de Fases

### Fase 1A — Backend: Constantes + Deduplicação openai_tts.go
- [ ] Extrair constantes: `TtsMaxChunkSize`, `TtsStreamBufSize`, `TtsTimeoutBase`, `TtsTimeoutPerChunk`
- [ ] Consolidar `Synthesize` + `SynthesizeWithVoice` → `synthesizeInternal(text, voice)`
- [ ] Consolidar `SynthesizeStream` + `SynthesizeStreamWithVoice` → `synthesizeStreamInternal(ctx, text, voice, callbacks)`
- [ ] Manter métodos públicos como thin wrappers (API compatível)
- [ ] `GetAvailableVoices` → var estática (evitar rebuild do slice)

### Fase 1B — Backend: calcTTSTimeout + Error Handling + profile_factory
- [ ] Extrair `CalcTTSTimeout(textLen int) time.Duration`
- [ ] Usar em `app_speech.go` (2 locais) e `index.ts` frontend (2 locais)
- [ ] `profile_factory.go`: retornar erro estruturado quando credenciais falham
- [ ] Refatorar `SpeakPreview` em sub-funções (`previewSAPI5`, `previewLLM`)
- [ ] Wrap errors com contexto em `app_speech.go` (pattern `fmt.Errorf("op: %w", err)`)

### Fase 2A — Frontend: TTSConfig unificado + base64 util
- [ ] Remover `TTSConfig` de `index.ts`, re-exportar de `types.ts`
- [ ] Adicionar `enabledForUser` a `types.ts`
- [ ] Extrair `base64ToBlob(base64: string, mimeType: string): Blob` em `lib/audioUtils.ts`
- [ ] Usar em `openai.ts`, `streamPlayer.ts`, `messageAudio/index.ts`

### Fase 2B — Frontend: Race Conditions
- [ ] `speakWithOverride`: não mutar config global para webspeech/sapi5 — criar provider temporário
- [ ] `openai.ts` pendingSynthesis: usar AbortController + rejeitar Promise no stop()
- [ ] `sapi5.ts` isSpeaking: usar campo `_isSpeaking` gerenciado pelo polling

### Fase 2C — Frontend: StreamPlayer + Auto-read
- [ ] `streamPlayer.ts`: cancelar timeout pendente no cleanup/dispose
- [ ] `streamPlayer.ts`: invalidar singleton no dispose (`globalStreamPlayer = null`)
- [ ] Remover latência calculada e descartada (`void latency`)
- [ ] Extrair `triggerAutoRead(text, role)` helper no `chatStore.ts`
- [ ] Garantir `stripMarkdown` consistente em todos os paths
- [ ] `messageAudioService.saveAudioToDB`: logar warning em vez de engolir

### Fase 3 — Acoplamento: Event Constants + Pitch Pipeline
- [ ] Criar `frontend/src/lib/speechEvents.ts` com constantes de eventos
- [ ] Criar `internal/speech/events.go` com constantes correspondentes
- [ ] Adicionar `pitch` a `RoleVoiceConfig` (frontend + backend `speech.RoleVoiceConfig`)
- [ ] Propagar pitch em `profile_factory.go` `buildRoleConfig`
- [ ] Propagar pitch em `useInteractionProfile.ts` `makeRoleConfig`
- [ ] `TTSControls.tsx`: fix tabIndex para acessibilidade

### Fase 4 — Testes Prioritários
- [ ] `internal/speech/openai_tts_test.go`: testar `buildParams`, constantes, synthesize internal
- [ ] `frontend/src/services/tts/index.test.ts`: speakAsRole, hasVoiceConfig, isAutoReadEnabled
- [ ] `frontend/src/services/tts/providers/openai.test.ts`: speak, stop, pendingSynthesis
- [ ] `frontend/src/lib/audioUtils.test.ts`: base64ToBlob

## Critério de Sucesso
- Todos os testes existentes (973 frontend + Go) continuam passando
- Novos testes cobrem funções críticas
- Zero duplicação nos métodos Synthesize
- TTSConfig com definição única
- Race conditions eliminadas
