# Speech System Status & Analysis

**Date:** March 6, 2026  
**Status:** ⚠️ Partially Broken - Needs Provider Integration

---

## 1. Current State

### What Works
- ✅ Speech Manager initialized in app.go
- ✅ TTS (OpenAI) can be called directly
- ✅ STT (Whisper) can be called directly
- ✅ Profile has Voice and Interaction config (already per-profile!)
- ✅ ResponseTimeout exists in ChatConfig

### What's NOT Working (Fixable)
- ❌ `InitSpeechManager()` hardcoded to read `config.json` APIKey/APIBaseURL
  - Should read from **profile** instead
  - Profile.Voice.Provider already exists
  - Profile.Interaction.STTProvider already exists
- ❌ **ResponseTimeout not used from profile**
  - `profile.Chat.ResponseTimeout` exists but ignored
  - Uses global `config.json` timeout instead
- ❌ **Credentials passed as strings**
  - Should use credentials.Manager (encrypted)
  - Will be fixed by provider integration

---

## 2. Architecture Issue: Global Speech Config

### Problem Flow (Current)
```
config.json
  ├─ APIKey: "sk-xxxx"      ← Global, used by EVERYONE
  └─ APIBaseURL: "https://..."

app.go line 2162:
  InitSpeechManager(cfg.APIKey, cfg.APIBaseURL, ...)
  
speech_manager.go:
  SpeechConfig{
    OpenAIAPIKey: "sk-xxxx",      ← SAME for all profiles
    OpenAIAPIBaseURL: "https://..."
  }

Result:
  - Profile A wants to use Claude for chat, but is forced to use OpenAI TTS
  - Profile B wants different APIKey, but can't change it
  - Profile C has no TTS because config is missing
```

### Target Flow (After Provider Integration)
```
Provider Registry
  ├─ "openai-default"
  │  ├─ BaseURL: "https://api.openai.com/v1"
  │  └─ CredentialPattern: "*.openai.com"
  └─ "openai-prod"
     ├─ BaseURL: "https://api.openai.com/v1"
     └─ CredentialPattern: "*.openai.com"

Credentials Manager
  ├─ Pattern "*.openai.com" → Token "sk-xxxx" (Profile A)
  └─ Pattern "*.openai.com" → Token "sk-yyyy" (Profile B)

Profile A
  ├─ voice.provider_id: "openai-default"
  └─ interaction.provider_id: "openai-default"

Profile B
  ├─ voice.provider_id: "openai-prod"
  └─ interaction.provider_id: "openai-prod"

Result:
  - Each profile can have different TTS/STT credentials
  - Automatic token injection via credentials.Manager
```

---

## 3. Code Analysis

### 3.1 InitSpeechManager (app.go:2129-2145)
```go
func (a *App) InitSpeechManager(apiKey, apiBaseURL, whisperLanguage, ttsVoice, ttsModel string) error {
	config := speech.SpeechConfig{
		STTProvider:      speech.STTProviderWhisper,
		TTSProvider:      speech.TTSProviderOpenAI,
		OpenAIAPIKey:     apiKey,          // ← HARDCODED per-call
		OpenAIAPIBaseURL: apiBaseURL,      // ← HARDCODED per-call
		WhisperModel:     "whisper-1",
		WhisperLanguage:  whisperLanguage,
		TTSModel:         ttsModel,
		TTSVoice:         ttsVoice,
	}

	a.speechManager = speech.NewSpeechManager(config, a.credMgr)
	return nil
}
```

**Issues:**
- Takes explicit `apiKey` and `apiBaseURL` parameters
- Called 8 times in app.go with same credentials each time
- No profile awareness
- No way to override per-profile

### 3.2 ensureSpeechManager (app.go:2148-2165)
```go
func (a *App) ensureSpeechManager() bool {
	if a.speechManager != nil {
		return true
	}
	cfg, err := config.Load()
	if err != nil {
		log.Printf("[Speech] Erro ao carregar config...")
		return false
	}
	if cfg.APIKey == "" {
		log.Printf("[Speech] API key não configurada...")
		return false
	}
	a.InitSpeechManager(cfg.APIKey, cfg.APIBaseURL, "pt", "nova", "tts-1")
	return a.speechManager != nil
}
```

**Issues:**
- Reads from global `config.json` only
- Falls back to hardcoded values when missing
- No profile context

### 3.3 Places Called (8 times!)
1. **Startup** (line 2162): `a.InitSpeechManager(cfg.APIKey, cfg.APIBaseURL, "pt", "nova", "tts-1")`
2. **Profile switch** (line 2258): `a.InitSpeechManager(cfg.APIKey, cfg.APIBaseURL, "pt", "nova", "tts-1")`
3. **Voice change** (line 2284, 2309, 2351, 2572)
4. **Settings update** (line 2572)

**Pattern:** Every time voice settings change, calls `InitSpeechManager()` with same `cfg.APIKey/APIBaseURL`

---

## 4. Profile Structure (Already Good!)

### VoiceConfig (types.go:68-82)
```go
type VoiceConfig struct {
	Disabled        bool    `json:"disabled,omitempty"`
	Provider        string  `json:"provider"`            // ✅ Already has provider field
	VoiceID         string  `json:"voice_id,omitempty"`
	Rate            float64 `json:"rate"`
	Pitch           float64 `json:"pitch"`
	Volume          float64 `json:"volume"`
	EnabledForAgent bool    `json:"enabled_for_agent"`
	EnabledForUser  bool    `json:"enabled_for_user"`
	ChannelResponseMode string `json:"channel_response_mode,omitempty"`
	
	// MISSING: Provider selection ID
	// ProviderID string `json:"provider_id,omitempty"` // NEW: for TTS credentials
}
```

### InteractionConfig (types.go:103-130)
```go
type InteractionConfig struct {
	Disabled       bool            `json:"disabled,omitempty"`
	STTProvider    string          `json:"stt_provider"`      // ✅ Already has provider field
	Language       string          `json:"language"`
	FeedbackSounds bool            `json:"feedback_sounds"`
	Triggers       []TriggerConfig `json:"triggers,omitempty"`
	
	// MISSING: Provider selection ID
	// ProviderID string `json:"provider_id,omitempty"` // NEW: for STT credentials
}
```

### ChatConfig (types.go:34-65)
```go
type ChatConfig struct {
	Model                string   `json:"model,omitempty"`
	Temperature          float64  `json:"temperature"`
	MaxTokens            int      `json:"max_tokens"`
	ContextWindow        int      `json:"context_window,omitempty"`
	MaxContextMessages   int      `json:"max_context_messages,omitempty"`
	MinContextMessages   int      `json:"min_context_messages,omitempty"`
	TopP                 float64  `json:"top_p"`
	ResponseTimeout      int      `json:"response_timeout"`  // ✅ EXISTS but unused!
	ReasoningEffort      string   `json:"reasoning_effort,omitempty"`
	SystemPrompt         string   `json:"system_prompt,omitempty"`
	// ... more fields
}
```

**Good news:** Structure is already there, just needs:
1. Add `ProviderID` fields to VoiceConfig + InteractionConfig
2. Use `ResponseTimeout` from profile instead of global config

---

## 5. Integration Tasks

### Task 1: Update InitSpeechManager Signature
```go
// OLD (config.json-based):
func (a *App) InitSpeechManager(apiKey, apiBaseURL, whisperLanguage, ttsVoice, ttsModel string) error

// NEW (profile-based):
func (a *App) InitSpeechManager(profile *profiles.Profile) error {
    // Use profile.Voice.Provider for TTS
    // Use profile.Interaction.STTProvider for STT
    // Use profile.Chat.ResponseTimeout for timeout
    // Resolve credentials via credMgr (encrypted)
}
```

### Task 2: Use ResponseTimeout from Profile
```go
// Instead of:
timeout := cfg.ResponseTimeout

// Do:
timeout := profile.Chat.ResponseTimeout
if timeout == 0 {
    timeout = 180  // default
}
```

### Task 3: Update all 8 Call Sites
Find and replace all calls to `InitSpeechManager()`:
```go
// Lines: 2162, 2258, 2284, 2309, 2351, 2572, etc.
// Change from:
a.InitSpeechManager(cfg.APIKey, cfg.APIBaseURL, "pt", "nova", "tts-1")

// To:
profile, _ := profiles.Get(cfg.ActiveProfile)
if profile != nil {
    a.InitSpeechManager(profile)
}
```

### Task 4: Update ensureSpeechManager()
- Pass profile to `InitSpeechManager()` instead of just config

---

## 6. Implementation Order

1. **Phase 1 (Provider Infrastructure):** Create provider registry + types
2. **Phase 2 (LLM Client):** Refactor LLM client to use providers
3. **Phase 3 (Profile Selection):** Profile-based LLM provider selection
4. **Phase 4 (Speech Manager Fix):** Update InitSpeechManager to use profile
   - ✅ No new profile fields needed (Voice + Interaction + Chat.ResponseTimeout already exist)
   - ✅ Just rewire the code to read from profile instead of config.json
5. **Phase 5 (Cleanup):** Remove APIKey, APIBaseURL, ResponseTimeout, STTParams from config.json

---

## 7. Backward Compatibility

If old profiles don't have `provider_id`:
```go
func (vc *VoiceConfig) GetProviderID() string {
    if vc.ProviderID != "" {
        return vc.ProviderID
    }
    // Default: use provider name as ID
    if vc.Provider == "openai" {
        return "openai-default"
    }
    return "default"
}
```

---

## 8. Success Criteria

✅ Profile can specify different TTS provider per profile  
✅ Profile can specify different STT provider per profile  
✅ Profile's ResponseTimeout is used (not global config)  
✅ Credentials resolved from credentials.Manager (encrypted)  
✅ Profile switch automatically updates speech manager  
✅ Multiple providers with different API keys work  
✅ Old profiles without provider_id still work (fallback)  

---

## 9. Files to Modify

- `internal/profiles/types.go` — Add ProviderID fields
- `app.go` — Refactor InitSpeechManager + 8 call sites
- `internal/speech/speech_manager.go` — Accept providers + registry
- `builtin_profiles.go` — Add provider_id to default profiles
- `docs/PROFILES.md` — Document new provider_id fields
