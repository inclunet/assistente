# Refatoracao do Modelo de Voz ÔÇö Separacao por Role

## Status: Proposto

---

## Motivacao

O modelo atual de configuracao de voz (`VoiceConfig`) acumula campos flat para assistente, usuario e sistema na mesma struct, misturados com campos legacy. Resultado:

- 20+ campos em `VoiceConfig` com prefixos (`AssistantVoiceID`, `UserVoiceID`, `EnabledForAgent`, `EnabledForUser`, etc.)
- Campos legacy convivem com campos novos (`Provider` vs `AssistantProviderID`, `VoiceID` vs `AssistantVoiceID`)
- Frontend usa campos antigos (`voice_id`, `enabled_for_agent`) enquanto backend tem campos novos nao consumidos
- `InteractionConfig` (STT) e `channel_response_mode` (canais) estao embutidos em `voice`, poluindo a separacao de responsabilidades
- `InitSpeechManagerFromProfile()` tem 40+ linhas de fallback/migracao
- Frontend `useInteractionProfile` sincroniza TTS via campos legacy, ignorando campos novos

O sistema funciona parcialmente mas e fragil: editar perfil nao reinicializa speech, trocar provider por role nao tem efeito, e novos campos Go nao existem nos bindings TypeScript.

---

## Decisao: Modelo Limpo por Role

Redesenhar `VoiceConfig` como struct hierarquica com 3 sub-configs independentes (assistant, user, system), extrair STT para `InputConfig` e canais para `ChannelsConfig`. Sem campos legacy, sem fallback.

### Atualizacao 2026-05-05: contrato definitivo de modelo e voz TTS

A implementacao inicial separou os campos `model` e `voice_id`, mas a UI e parte do backend continuaram tratando alguns modelos TTS dinamicos como se fossem vozes. Isso criou um contrato ambiguo:

- OpenAI oficial usava IDs compostos de picker (`voiceId::model`), misturando modelo e voz em uma unica selecao.
- Piper/LocalAI expunha modelos `voice-*` como `TTSVoiceInfo`, porque cada modelo Piper costuma representar uma voz.
- Qwen/Kokoro/OpenAI-compatible eram empurrados para o mesmo caminho dinamico, embora normalmente tenham modelo TTS e nome de voz separados.
- Backend aceitava `model` vazio e preenchia com `voice_id`, mascarando configuracao invalida.

Essa ambiguidade fica removida. A partir desta AEP, TTS HTTP tem contrato explicito:

```
voice.<role>:
  provider: "openai"        # familia HTTP OpenAI-compatible
  llm_provider_id: string   # provider registrado para credenciais/base URL
  model: string             # obrigatorio para TTS HTTP
  voice_id: string          # obrigatorio somente quando o provider/modelo exige voz separada
  selection_mode: string    # "model_and_voice" ou "model_only"
```

Nomenclatura:

- `provider` e a familia de TTS no perfil. Para APIs HTTP OpenAI-compatible, o valor e `openai`, mesmo quando o backend real e Kokoro, Qwen, LocalAI ou outro endpoint compativel.
- `llm_provider_id` e o ID do provider registrado que carrega credenciais, base URL e formato da API.
- Parametros `providerID` nas APIs abaixo recebem esse mesmo ID registrado (`llm_provider_id`), nao a familia `provider`.

Regras:

- `model` nunca e inferido a partir de `voice_id`.
- `voice_id` nunca carrega modelo embutido.
- Providers/modelos como OpenAI, Kokoro e Qwen usam `selection_mode = "model_and_voice"`: o usuario escolhe modelo e voz separadamente.
- Providers/modelos como Piper usam `selection_mode = "model_only"`: o usuario escolhe apenas o modelo; `voice_id` permanece vazio.
- IDs compostos (`voiceId::model`) ficam proibidos.
- Listagem de `/v1/models` alimenta seletor de modelos, nao seletor de vozes.
- Listagem de vozes recebe `providerID` e `modelID` como entrada.
- Sem migracao automatica, fallback, heuristica de compatibilidade ou degradacao silenciosa para perfis antigos.

APIs obrigatorias:

```
GetTTSModels(providerID) []TTSModelInfo
GetTTSVoices(providerID, modelID) []TTSVoiceInfo
SpeakPreview(providerID, modelID, voiceID, rate, volume, text, sessionID)
SpeakMessage(messageID, providerID, modelID, voiceID, rate)
```

Em `selection_mode = "model_only"`, o cliente chama `SpeakPreview` e `SpeakMessage` com `voiceID = ""`; a requisicao HTTP envia apenas o modelo.

Validacao obrigatoria:

- Para qualquer TTS HTTP OpenAI-compatible representado no perfil por `provider = "openai"`, `model` vazio e erro de configuracao.
- Para `selection_mode = "model_and_voice"`, `voice_id` vazio e erro de configuracao.
- Para `selection_mode = "model_only"`, `voice_id` deve ficar vazio e a sintese envia apenas o modelo.
- Nenhum caminho pode tentar outro provider automaticamente.

### Antes (modelo atual)

```
voice:
  disabled: bool
  provider: string                    # legacy
  assistant_provider_id: string       # novo (nao usado no TS)
  user_provider_id: string            # novo (nao usado no TS)
  system_provider_id: string          # novo (nao usado no TS)
  llm_provider_id: string             # legacy
  voice_id: string                    # legacy
  assistant_voice_id: string          # novo
  user_voice_id: string               # novo
  system_voice_id: string             # novo
  tts_model: string                   # novo (nao existe no TS)
  rate: float
  pitch: float
  volume: float
  enabled_for_agent: bool             # legacy
  enabled_for_user: bool              # legacy
  enabled_for_assistant: bool         # novo
  enabled_for_user_voice: bool        # novo
  enabled_for_system: bool            # novo
  channel_response_mode: string       # misturado aqui
interaction:
  stt_provider: string
  llm_provider_id: string
  language: string
  feedback_sounds: bool
  triggers: [...]
```

### Depois (modelo novo)

```
voice:
  assistant:
    enabled: bool
    provider: string          # "disabled", "webspeech", "sapi5", "openai"
    llm_provider_id: string   # ID do provedor LLM (para credenciais API)
    voice_id: string          # "nova", "alloy", "echo", etc.
    model: string             # "tts-1", "tts-1-hd"
    rate: float               # 0.5-2.0
    pitch: float              # 0.5-2.0
    volume: float             # 0.0-1.0
  user:
    enabled: bool
    provider: string
    llm_provider_id: string
    voice_id: string
    model: string
    rate: float
    pitch: float
    volume: float
  system:
    enabled: bool
    provider: string
    llm_provider_id: string
    voice_id: string
    model: string
    rate: float
    pitch: float
    volume: float
input:
  enabled: bool
  stt_provider: string        # "webspeech", "whisper_api"
  llm_provider_id: string     # Para Whisper API (independente do chat)
  language: string            # "pt-BR", "en", "es"
  feedback_sounds: bool
  triggers: [...]
channels:
  response_mode: string       # "mirror", "always_text", "always_audio"
```

---

## Roles de Voz

| Role | Escopo | Exemplo de uso |
|------|--------|----------------|
| `assistant` | Respostas do LLM | Auto-read de mensagens do assistente |
| `user` | Mensagens do proprio usuario | Acessibilidade: confirmar o que foi enviado |
| `system` | Notificacoes, tool calling, MCP, alertas | Mensagens que existem mas normalmente nao sao lidas |

Cada role tem provider independente. Exemplos:

- Assistente usa OpenAI TTS (voz "nova"), usuario usa WebSpeech, sistema desabilitado
- Todas usam SAPI5 local (offline)
- Assistente e usuario usam OpenAI com vozes diferentes, sistema desabilitado

---

## Structs Go

```go
// VoiceRoleConfig configura TTS para uma role especifica
type VoiceRoleConfig struct {
    Enabled       bool    `json:"enabled"`
    Provider      string  `json:"provider"`                  // "disabled", "webspeech", "sapi5", "openai"
    LLMProviderID string  `json:"llm_provider_id,omitempty"` // ID do provedor LLM (para credenciais)
    VoiceID       string  `json:"voice_id,omitempty"`        // ID da voz
    Model         string  `json:"model,omitempty"`           // Modelo TTS
    Rate          float64 `json:"rate"`                      // Velocidade
    Pitch         float64 `json:"pitch"`                     // Tom
    Volume        float64 `json:"volume"`                    // Volume
}

// VoiceConfig configura TTS ÔÇö uma sub-config por role
type VoiceConfig struct {
    Assistant VoiceRoleConfig `json:"assistant"`
    User      VoiceRoleConfig `json:"user"`
    System    VoiceRoleConfig `json:"system"`
}

// InputConfig configura STT e triggers de interacao por voz
// Substitui InteractionConfig
type InputConfig struct {
    Enabled        bool            `json:"enabled"`
    STTProvider    string          `json:"stt_provider"`
    LLMProviderID  string          `json:"llm_provider_id,omitempty"`
    Language       string          `json:"language"`
    FeedbackSounds bool            `json:"feedback_sounds"`
    Triggers       []TriggerConfig `json:"triggers,omitempty"`
}

// ChannelsConfig configura comportamento para canais externos
type ChannelsConfig struct {
    ResponseMode string `json:"response_mode,omitempty"` // "mirror", "always_text", "always_audio"
}
```

TriggerConfig permanece inalterada.

---

## Exemplo: Profile Padrao (novo formato)

```json
{
  "_builtin_version": "4.0.0",
  "name": "Padrao",
  "description": "Configuracao padrao.",
  "icon": "chatbox",
  "chat": {
    "llm_provider": "$default",
    "model": "$default",
    "temperature": 0.7,
    "max_tokens": 4096,
    "context_window": 120000,
    "max_context_messages": 60,
    "min_context_messages": 20,
    "top_p": 1,
    "response_timeout": 180,
    "enabled_tools": null,
    "enabled_skills": ["workspace", "memory"]
  },
  "voice": {
    "assistant": {
      "enabled": false,
      "provider": "disabled",
      "llm_provider_id": "$default",
      "voice_id": "",
      "model": "tts-1",
      "rate": 1.0,
      "pitch": 1.0,
      "volume": 1.0
    },
    "user": {
      "enabled": false,
      "provider": "disabled",
      "rate": 1.0,
      "pitch": 1.0,
      "volume": 1.0
    },
    "system": {
      "enabled": false,
      "provider": "disabled",
      "rate": 1.0,
      "pitch": 1.0,
      "volume": 1.0
    }
  },
  "input": {
    "enabled": true,
    "stt_provider": "webspeech",
    "llm_provider_id": "$default",
    "language": "pt-BR",
    "feedback_sounds": true,
    "triggers": []
  },
  "channels": {
    "response_mode": "mirror"
  }
}
```

---

## Integracao Backend

### InitSpeechManagerFromProfile()

Fluxo simplificado sem fallbacks:

```
profile = GetActiveProfile()

Para cada role em [assistant, user, system]:
  roleConfig = profile.Voice.<Role>
  if roleConfig.Provider == "openai" && roleConfig.LLMProviderID != "":
    providerCfg = llmRegistry.Get(roleConfig.LLMProviderID)
    cred = credMgr.GetByPattern(providerCfg.CredentialPattern)
    -> apiKey, baseURL

sttConfig = profile.Input
if sttConfig.STTProvider == "whisper_api" && sttConfig.LLMProviderID != "":
  sttProviderCfg = llmRegistry.Get(sttConfig.LLMProviderID)
  -> whisper creds

speechManager = NewSpeechManager(config, credMgr)
```

### Pontos de chamada obrigatorios

| Local | Quando | Acao |
|-------|--------|------|
| `startup()` | App inicia | `InitSpeechManagerFromProfile()` |
| `SetActiveProfile()` | Troca de perfil | `InitSpeechManagerFromProfile()` |
| `UpdateProfile()` | Edita perfil ativo | `InitSpeechManagerFromProfile()` |

---

## Integracao Frontend

### useInteractionProfile ÔÇö sync de TTS

```typescript
// Antes (legacy)
ttsService.setAutoRead(voiceConfig.enabled_for_agent);
ttsService.setEnabledForUser(voiceConfig.enabled_for_user);
await ttsService.setVoice(voiceConfig.voice_id);

// Depois (por role)
const assistantVoice = profile.voice.assistant;
ttsService.setAutoRead(assistantVoice.enabled);
ttsService.setEnabledForUser(profile.voice.user.enabled);
const provider = mapTTSProvider(assistantVoice.provider);
await ttsService.setProvider(provider);
await ttsService.setVoice(assistantVoice.voice_id);
await ttsService.setRate(assistantVoice.rate);
ttsService.setPitch(assistantVoice.pitch);
await ttsService.setVolume(assistantVoice.volume);
```

### useInteractionProfile ÔÇö sync de STT

```typescript
// Antes
setProvider(mapSTTProvider(profile.interaction.stt_provider));
setLanguage(profile.interaction.language);

// Depois
setProvider(mapSTTProvider(profile.input.stt_provider));
setLanguage(profile.input.language);
```

### ProfileAudioTab ÔÇö layout com 3 roles

```
ProfileAudioTab
+-- ProfileVoiceSection (role="assistant", config=profile.voice.assistant)
+-- ProfileVoiceSection (role="user", config=profile.voice.user)
+-- ProfileVoiceSection (role="system", config=profile.voice.system)
+-- ProfileInputSection (config=profile.input)
+-- ChannelsSection (config=profile.channels)
```

`ProfileVoiceSection` e reutilizado 3x com props diferentes.

---

## Migracao

- **Sem migracao automatica** de perfis antigos
- Perfis builtin: bump `_builtin_version` para `4.0.0` forca reinstalacao
- Perfis custom do usuario: campos antigos ignorados na desserializacao (Go ignora campos desconhecidos por padrao); voice fica com zero-values (disabled)
- Campos removidos: `enabled_for_agent`, `enabled_for_user`, `voice_id` (flat), `provider` (flat), `llm_provider_id` (flat no voice), `assistant_provider_id`/`user_provider_id`/`system_provider_id` (flat), `tts_model` (flat), `channel_response_mode`
- Campos renomeados: `interaction` -> `input`

---

## Arquivos Impactados

### Backend (Go)
- `internal/profiles/types.go` ÔÇö Reescrever VoiceConfig, criar VoiceRoleConfig, InputConfig, ChannelsConfig
- `internal/speech/speech_manager.go` ÔÇö Simplificar SpeechConfig, remover campos legacy
- `app.go` ÔÇö InitSpeechManagerFromProfile, startup, UpdateProfile, SetActiveProfile, Synthesize*, Transcribe*, GetTTSVoices, PreviewVoiceSettings
- `builtin_profiles.go` ÔÇö Se referencia tipos voice/interaction
- `builtin/profiles/*.json` ÔÇö Novo formato, version bump
- `*_test.go` ÔÇö Adaptar testes que criam Profile

### Frontend (TypeScript)
- `frontend/wailsjs/go/models.ts` ÔÇö Regenerar bindings
- `frontend/src/hooks/useInteractionProfile.ts` ÔÇö Adaptar voice.assistant.*, input.*
- `frontend/src/hooks/useTTS.ts`, `useSTT.ts` ÔÇö Adaptar paths
- `frontend/src/services/tts/types.ts` ÔÇö Adicionar model
- `frontend/src/components/profiles/ProfileAudioTab.tsx` ÔÇö 3 sections por role
- `frontend/src/components/profiles/ProfileVoiceSection.tsx` ÔÇö Receber VoiceRoleConfig
- `frontend/src/components/profiles/ProfileInteractionSection.tsx` ÔÇö Renomear para InputSection
- `frontend/src/components/chat/VoiceButton.tsx` ÔÇö profile.input
- `frontend/src/components/chat/TTSControls.tsx` ÔÇö profile.voice.assistant
- `frontend/src/pages/ChatPage.tsx` ÔÇö voiceEnabled do perfil
- `frontend/src/store/chatStore.ts` ÔÇö Verificar auto-read
- `frontend/src/locales/{pt-BR,en,es}.ts` ÔÇö Strings por role

---

## Verificacao

1. `go test ./...` ÔÇö Todos passam
2. `npm run lint` ÔÇö Sem erros
3. `npm run test` ÔÇö Todos passam
4. `npm run build` ÔÇö Compila
5. Teste manual: editar perfil -> habilitar voz por role -> chat funciona
6. Teste manual: trocar perfil -> voz reconfigura
7. Teste manual: STT -> transcricao -> envio no chat
8. Teste manual: TTS auto-read por role funciona
