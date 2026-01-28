package speech

// VoskProvider constante do provider Vosk
const STTProviderVosk STTProvider = "vosk"

// VoskConfig configuração do Vosk
type VoskConfig struct {
	// Caminho para o modelo (ex: ~/.assistente/models/vosk-model-small-pt-0.3)
	ModelPath string `json:"modelPath"`

	// Idioma do modelo
	Language string `json:"language"`

	// Taxa de amostragem (geralmente 16000)
	SampleRate float64 `json:"sampleRate"`

	// Se deve usar GPU (se disponível)
	UseGPU bool `json:"useGpu"`
}

// VoskModelInfo informações sobre um modelo Vosk
type VoskModelInfo struct {
	// ID único do modelo
	ID string `json:"id"`

	// Nome de exibição
	Name string `json:"name"`

	// Idioma
	Language string `json:"language"`

	// Tamanho em bytes
	Size int64 `json:"size"`

	// URL de download
	DownloadURL string `json:"downloadUrl"`

	// Se está instalado localmente
	IsInstalled bool `json:"isInstalled"`

	// Caminho local (se instalado)
	LocalPath string `json:"localPath,omitempty"`
}

// VoskWakeWordConfig configuração para wake word
type VoskWakeWordConfig struct {
	// Palavra-chave (ex: "assistente", "computador")
	Keyword string `json:"keyword"`

	// Sensibilidade (0.0 - 1.0)
	Sensitivity float64 `json:"sensitivity"`

	// Callback quando wake word é detectado
	OnWakeWord func() `json:"-"`
}

// VoskStreamingConfig configuração para streaming STT
type VoskStreamingConfig struct {
	// Callback para resultados parciais
	OnPartialResult func(text string) `json:"-"`

	// Callback para resultados finais
	OnFinalResult func(text string) `json:"-"`

	// Callback para erros
	OnError func(err error) `json:"-"`
}

// Modelos Vosk disponíveis para download
var AvailableVoskModels = []VoskModelInfo{
	{
		ID:          "vosk-model-small-pt-0.3",
		Name:        "Português (Pequeno)",
		Language:    "pt-BR",
		Size:        39 * 1024 * 1024, // ~39MB
		DownloadURL: "https://alphacephei.com/vosk/models/vosk-model-small-pt-0.3.zip",
	},
	{
		ID:          "vosk-model-pt-fb-v0.1.1-20220516_2113",
		Name:        "Português (Grande)",
		Language:    "pt-BR",
		Size:        1600 * 1024 * 1024, // ~1.6GB
		DownloadURL: "https://alphacephei.com/vosk/models/vosk-model-pt-fb-v0.1.1-20220516_2113.zip",
	},
	{
		ID:          "vosk-model-small-en-us-0.15",
		Name:        "English (Small)",
		Language:    "en-US",
		Size:        40 * 1024 * 1024, // ~40MB
		DownloadURL: "https://alphacephei.com/vosk/models/vosk-model-small-en-us-0.15.zip",
	},
}

// DefaultVoskConfig configuração padrão
var DefaultVoskConfig = VoskConfig{
	Language:   "pt-BR",
	SampleRate: 16000,
	UseGPU:     false,
}
