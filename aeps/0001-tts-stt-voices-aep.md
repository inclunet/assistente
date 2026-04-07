# AEP 0001 ÔÇö Evolu├º├úo TTS/STT: Vozes (Assistant + User)

Estado: Draft
Autor: Leonardo
Owner: Leonardo
Idioma: en-US

## Resumo
Evoluir o sistema de configura├º├úo de vozes no perfil para suportar vozes separadas para o assistant e para o user, mantendo a possibilidade de usar a mesma voz para ambos. Revisar e validar a pipeline de STT para garantir que n├úo haja regress├Áes ap├│s mudan├ºas recentes.

## Objetivos
- Permitir configurar voice_assistant e voice_user no profile.
- Permitir usar uma ├║nica voz para ambos (op├º├úo "use same voice").
- Garantir migra├º├úo autom├ítica dos perfis existentes durante o deploy.
- Garantir compatibilidade com providers priorit├írios: Azure, ElevenLabs, Google, Amazon Polly, OpenAI.
- Revisar STT pipeline e corrigir regress├Áes.

## Deliverables
- Documento AEP detalhado (este arquivo).
- Migration script para perfis existentes.
- Backend APIs (GET/PUT profile.tts).
- Frontend UI components no profile.
- Integration tests (mocked providers + staging end-to-end).
- PR template e checklist.

## Data model proposal
Option A (preferred): JSON column `tts` on profiles table
{
  "tts": {
    "provider_id": "string",
    "assistant_voice_id": "string | null",
    "user_voice_id": "string | null",
    "voice_settings": {
      "rate": number,
      "pitch": number
    },
    "metadata": {
      "assistant_voice_name": "string",
      "user_voice_name": "string"
    }
  }
}

Fallbacks:
- If assistant_voice_id is null ÔåÆ fallback to user_voice_id or legacy `voice_id`.
- If both null ÔåÆ use system default.

Migration strategy (chosen): automatic on deploy (silent)
- Script populates assistant_voice_id and user_voice_id from legacy `voice_id`.

Alternative less-disruptive: two new columns `tts_assistant_voice_id`, `tts_user_voice_id` and keep legacy until later removal.

## API changes
- GET /profiles/:id ÔåÆ include `tts` object with voice metadata.
- PUT /profiles/:id ÔåÆ accept both legacy `voice_id` and new `tts` structure for back-compat.
- Validate provider_id + voice_id on save (ensure voice exists in provider metadata).

## Frontend UX
- Profile > Voices section
  - Checkbox: "Use same voice for assistant and user" (default: true)
  - If checked: single voice selector with play button
  - If unchecked: two selectors (Assistant voice / User voice) each with play
  - Show provider, locale, sample rate and cost hints
  - Button: "Play sample" and "Test microphone" (for STT)
  - Accessibility: labels, aria-live for playback start/stop, keyboard focus

## Migration script (pseudo)
- For each profile:
  - Read legacy voice_id
  - Set assistant_voice_id = voice_id
  - Set user_voice_id = voice_id
  - Save `tts` JSON

## Security
- Do not store provider secrets in plain text. Use existing key-protection system.
- Validate permission: only profile owner or admin can change provider/keys.

## STT review checklist
- Validate provider auth & endpoints after recent changes
- Verify recording pipeline: sample rate, encoding, chunking
- Smoke tests for BR Portuguese and English
- Fallback provider path and timeout handling
- Add CI integration test: short audio -> expected decent transcription

## Feature flags & rollout
- Add feature flag `tts_separate_voices` (default off). Enable for beta users then globally.
- Rollout: opt-in beta ÔåÆ 10% ÔåÆ 50% ÔåÆ 100%

## Tests
- Unit tests for migration and API validations
- Integration tests with mocked provider responses
- E2E test with staging keys for at least one provider
- Accessibility manual checks

## Estimatives (rough)
- Design & AEP: 1-2d
- Backend: 2-4d
- Frontend: 2-4d
- Migration & scripts: 1d
- Tests & QA: 2d
- Rollout & monitoring: 1d
Total: 9-14d

## PR template & checklist
- Include description and link to AEP
- Migration script included (if DB changes)
- Tests added/updated
- Accessibility checks
- Feature flag gating implemented

## Acceptance criteria
- User can configure same or different voices and play samples
- Old profiles keep behavior via migration/fallback
- STT sanity checks pass (no regressions in main languages)
- All tests pass and PR checklist completed

## Next steps
1. Review AEP content and confirm file naming/numbering
2. Create feature-flag and trunk branch
3. Implement migration script + backend
4. Implement frontend profile UI
5. Tests, QA and rollout


---
Generated automatically per request. Owner: Leonardo
