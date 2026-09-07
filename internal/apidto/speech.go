package apidto

// SynthesisResultInfo projeta SynthesisResult sem AudioData (bytes) para o frontend.
type SynthesisResultInfo struct {
	AudioBase64 string `json:"audioBase64"`
	Format      string `json:"format"`
	Provider    string `json:"provider"`
}

// ChatSpeakStrategy é a estratégia de fala resolvida para um evento chat:speak.
type ChatSpeakStrategy string

const (
	ChatSpeakStrategyNone         ChatSpeakStrategy = "none"
	ChatSpeakStrategyAnnounce     ChatSpeakStrategy = "announce"
	ChatSpeakStrategyWebSpeech    ChatSpeakStrategy = "webspeech"
	ChatSpeakStrategyBackendAudio ChatSpeakStrategy = "backend_audio"
)

// ChatSpeakOrigin identifica a origem do pedido de fala no chat.
type ChatSpeakOrigin string

const (
	ChatSpeakOriginAssistantMessage ChatSpeakOrigin = "assistant_message"
	ChatSpeakOriginUserMessage      ChatSpeakOrigin = "user_message"
	ChatSpeakOriginSystemMessage    ChatSpeakOrigin = "system_message"
	ChatSpeakOriginThinking         ChatSpeakOrigin = "thinking"
	ChatSpeakOriginToolStatus       ChatSpeakOrigin = "tool_status"
	ChatSpeakOriginSegment          ChatSpeakOrigin = "segment"
)

// ChatSpeakRequest é o payload da borda Wails para DispatchSpeech (AEP-0088).
type ChatSpeakRequest struct {
	ConversationID string          `json:"conversationId"`
	MessageID      string          `json:"messageId,omitempty"`
	ProfileSlug    string          `json:"profileSlug,omitempty"`
	Role           string          `json:"role"`
	Text           string          `json:"text"`
	Origin         ChatSpeakOrigin `json:"origin"`
	Interrupt      *bool           `json:"interrupt,omitempty"`
}

// ChatSpeakEvent é o evento emitido em chat:speak após resolver perfil/estratégia.
type ChatSpeakEvent struct {
	MessageID        string            `json:"messageId,omitempty"`
	ConversationID   string            `json:"conversationId"`
	Role             string            `json:"role"`
	Text             string            `json:"text"`
	Strategy         ChatSpeakStrategy `json:"strategy"`
	FallbackStrategy ChatSpeakStrategy `json:"fallbackStrategy,omitempty"`
	AutoRead         bool              `json:"autoRead"`
	ProviderID       string            `json:"providerId,omitempty"`
	VoiceID          string            `json:"voiceId,omitempty"`
	Model            string            `json:"model,omitempty"`
	Rate             float64           `json:"rate,omitempty"`
	Pitch            float64           `json:"pitch,omitempty"`
	Volume           float64           `json:"volume,omitempty"`
	Origin           ChatSpeakOrigin   `json:"origin"`
	Interrupt        bool              `json:"interrupt"`
	// SpeechLanguage é o idioma do perfil que resolveu este evento. A strategy
	// backend_audio regenera o áudio a partir da mensagem persistida, então
	// precisa do mesmo idioma para não falar rótulos de outro perfil.
	SpeechLanguage string `json:"speechLanguage,omitempty"`
}
