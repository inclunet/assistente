package speech

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"assistente/internal/credentials"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/pagination"
	"github.com/openai/openai-go/packages/param"
)

// ============================================================================
// Constantes do subsistema TTS
// ============================================================================

const (
	// TtsMaxChunkSize é o tamanho máximo (em caracteres) de cada chunk de texto
	// enviado à API de síntese. Margem de segurança abaixo do limite de 4096.
	TtsMaxChunkSize = 4000

	// TtsStreamBufSize é o tamanho do buffer de leitura de streaming (8KB).
	TtsStreamBufSize = 8192

	// TtsTimeoutBase é o timeout base para operações de síntese.
	TtsTimeoutBase = 60 * time.Second

	// TtsTimeoutPerChunk é o timeout adicional por chunk de texto.
	TtsTimeoutPerChunk = 30 * time.Second
)

// CalcTTSTimeout calcula o timeout para uma operação TTS baseado no tamanho do texto.
func CalcTTSTimeout(textLen int) time.Duration {
	chunks := textLen / TtsMaxChunkSize
	return TtsTimeoutBase + time.Duration(chunks)*TtsTimeoutPerChunk
}

// TTSVoice representa uma voz disponível no OpenAI TTS
type TTSVoice string

const (
	VoiceAlloy   TTSVoice = "alloy"   // Neutra, balanceada
	VoiceEcho    TTSVoice = "echo"    // Masculina, profunda
	VoiceFable   TTSVoice = "fable"   // Expressiva, narrativa
	VoiceOnyx    TTSVoice = "onyx"    // Masculina, autoritária
	VoiceNova    TTSVoice = "nova"    // Feminina, jovem
	VoiceShimmer TTSVoice = "shimmer" // Feminina, clara
)

// TTSModel representa o modelo de TTS
type TTSModel string

const (
	ModelTTS1   TTSModel = "tts-1"    // Rápido, menor qualidade
	ModelTTS1HD TTSModel = "tts-1-hd" // Mais lento, melhor qualidade
)

// TTSSelectionMode define se um modelo usa voz separada ou se o próprio
// modelo representa a voz, como em Piper.
type TTSSelectionMode string

const (
	TTSSelectionModelAndVoice TTSSelectionMode = "model_and_voice"
	TTSSelectionModelOnly     TTSSelectionMode = "model_only"
)

// TTSFormat representa o formato de saída de áudio
type TTSFormat string

const (
	FormatMP3  TTSFormat = "mp3"
	FormatOpus TTSFormat = "opus"
	FormatAAC  TTSFormat = "aac"
	FormatFLAC TTSFormat = "flac"
	FormatWAV  TTSFormat = "wav"
	FormatPCM  TTSFormat = "pcm"
)

// TTSConfig configuração para o OpenAI TTS
type TTSConfig struct {
	BaseURL           string           // URL base (vazio = default OpenAI)
	CredentialPattern string           // padrão para credential transport (ex: "api.openai.com")
	Model             TTSModel         // modelo TTS
	Voice             TTSVoice         // alloy, echo, fable, onyx, nova, shimmer
	SelectionMode     TTSSelectionMode // model_and_voice ou model_only
	Format            TTSFormat        // mp3, opus, aac, flac, wav, pcm
	Speed             float64          // 0.25 a 4.0 (1.0 = normal)
}

// TTSClient cliente para síntese de voz via OpenAI SDK
type TTSClient struct {
	client *openai.Client
	config TTSConfig
}

// TTSVoiceInfo informações sobre uma voz
type TTSVoiceInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Gender      string `json:"gender"`
	Provider    string `json:"provider"`
	ModelID     string `json:"model_id,omitempty"`
}

// TTSModelInfo informações sobre um modelo TTS selecionável.
type TTSModelInfo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	Provider      string `json:"provider"`
	SelectionMode string `json:"selection_mode"`
}

// NewTTSClient cria um novo cliente TTS usando o SDK openai-go.
// Credenciais são injetadas automaticamente via CredentialTransport,
// usando a mesma estratégia do LLM provider.
func NewTTSClient(config TTSConfig, credMgr *credentials.Manager) *TTSClient {
	if config.Format == "" {
		config.Format = FormatMP3
	}
	if config.Speed == 0 {
		config.Speed = 1.0
	}

	httpClient := credentials.NewHTTPClient(credMgr, config.CredentialPattern, 60*time.Second)

	opts := []option.RequestOption{
		option.WithHTTPClient(httpClient),
		option.WithAPIKey("managed-by-credential-transport"),
	}

	if config.BaseURL != "" {
		baseURL := strings.TrimSuffix(config.BaseURL, "/")
		if !strings.HasSuffix(baseURL, "/v1") {
			baseURL += "/v1"
		}
		opts = append(opts, option.WithBaseURL(baseURL+"/"))
	}

	client := openai.NewClient(opts...)

	return &TTSClient{
		client: &client,
		config: config,
	}
}

// buildParams constrói os parâmetros para Audio.Speech.New
func (c *TTSClient) buildParams(text string, voice TTSVoice) (openai.AudioSpeechNewParams, error) {
	modelID := string(c.config.Model)
	voiceID := string(voice)
	mode := normalizeTTSSelectionMode(modelID, c.config.SelectionMode)
	if err := validateTTSSelection(modelID, voiceID, mode); err != nil {
		return openai.AudioSpeechNewParams{}, err
	}

	params := openai.AudioSpeechNewParams{
		Input: text,
		Model: openai.SpeechModel(c.config.Model),
	}
	if mode == TTSSelectionModelAndVoice {
		params.Voice = openai.AudioSpeechNewParamsVoice(voice)
	}
	if c.config.Speed != 1.0 {
		params.Speed = param.NewOpt(c.config.Speed)
	}
	if c.config.Format != "" {
		params.ResponseFormat = openai.AudioSpeechNewParamsResponseFormat(c.config.Format)
	}
	return params, nil
}

// synthesizeInternal é a implementação central de síntese de texto para áudio.
// Divide textos longos em chunks e sintetiza sequencialmente.
func (c *TTSClient) synthesizeInternal(text string, voice TTSVoice) ([]byte, error) {
	if text == "" {
		return nil, fmt.Errorf("text cannot be empty")
	}
	chunks := splitTextForTTS(text)
	var allData []byte
	for _, chunk := range chunks {
		params, err := c.buildParams(chunk, voice)
		if err != nil {
			return nil, err
		}
		resp, err := c.client.Audio.Speech.New(context.Background(), params)
		if err != nil {
			return nil, fmt.Errorf("TTS synthesis failed: %w", err)
		}
		data, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read TTS response: %w", err)
		}
		allData = append(allData, data...)
	}
	return allData, nil
}

// Synthesize converte texto em áudio usando a voz configurada.
func (c *TTSClient) Synthesize(text string) ([]byte, error) {
	return c.synthesizeInternal(text, c.config.Voice)
}

// SynthesizeWithVoice converte texto em áudio usando uma voz específica.
func (c *TTSClient) SynthesizeWithVoice(text string, voice TTSVoice) ([]byte, error) {
	return c.synthesizeInternal(text, voice)
}

// TTSStreamCallbacks callbacks para streaming de áudio
type TTSStreamCallbacks struct {
	OnChunk func(chunk []byte) // Chamado para cada chunk de áudio recebido
	OnDone  func()             // Chamado quando streaming termina com sucesso
	OnError func(err error)    // Chamado em caso de erro
}

// synthesizeStreamInternal é a implementação central de síntese com streaming.
func (c *TTSClient) synthesizeStreamInternal(ctx context.Context, text string, voice TTSVoice, callbacks TTSStreamCallbacks) error {
	if text == "" {
		return fmt.Errorf("text cannot be empty")
	}

	chunks := splitTextForTTS(text)
	for _, chunk := range chunks {
		params, err := c.buildParams(chunk, voice)
		if err != nil {
			return err
		}
		resp, err := c.client.Audio.Speech.New(ctx, params)
		if err != nil {
			return fmt.Errorf("TTS stream failed: %w", err)
		}
		if err := readStreamChunks(ctx, resp.Body, callbacks); err != nil {
			_ = resp.Body.Close()
			return err
		}
		_ = resp.Body.Close()
	}

	return nil
}

// SynthesizeStream converte texto em áudio com streaming usando a voz configurada.
func (c *TTSClient) SynthesizeStream(ctx context.Context, text string, callbacks TTSStreamCallbacks) error {
	return c.synthesizeStreamInternal(ctx, text, c.config.Voice, callbacks)
}

// SynthesizeStreamWithVoice converte texto em áudio com streaming usando uma voz específica.
func (c *TTSClient) SynthesizeStreamWithVoice(ctx context.Context, text string, voice TTSVoice, callbacks TTSStreamCallbacks) error {
	return c.synthesizeStreamInternal(ctx, text, voice, callbacks)
}

// splitTextForTTS divide texto longo em chunks de no máximo 4096 caracteres,
// quebrando em limites de frase/parágrafo para manter naturalidade.
func splitTextForTTS(text string) []string {
	if len(text) <= TtsMaxChunkSize {
		return []string{text}
	}

	var chunks []string
	remaining := text

	for len(remaining) > 0 {
		if len(remaining) <= TtsMaxChunkSize {
			chunks = append(chunks, remaining)
			break
		}

		// Procura melhor ponto de quebra dentro do limite
		cutPoint := findBreakPoint(remaining, TtsMaxChunkSize)
		chunks = append(chunks, remaining[:cutPoint])
		remaining = strings.TrimLeft(remaining[cutPoint:], " \n\r")
	}

	return chunks
}

// findBreakPoint encontra o melhor ponto de quebra no texto, priorizando:
// 1. Quebra de parágrafo (\n\n)
// 2. Quebra de linha (\n)
// 3. Fim de frase (. ! ?)
// 4. Vírgula ou ponto-e-vírgula
// 5. Espaço
// 6. Corte bruto no limite
func findBreakPoint(text string, maxLen int) int {
	segment := text[:maxLen]

	// Parágrafo
	if idx := strings.LastIndex(segment, "\n\n"); idx > maxLen/2 {
		return idx + 2
	}
	// Linha
	if idx := strings.LastIndex(segment, "\n"); idx > maxLen/2 {
		return idx + 1
	}
	// Fim de frase
	for _, sep := range []string{". ", "! ", "? "} {
		if idx := strings.LastIndex(segment, sep); idx > maxLen/2 {
			return idx + len(sep)
		}
	}
	// Vírgula/ponto-e-vírgula
	for _, sep := range []string{", ", "; "} {
		if idx := strings.LastIndex(segment, sep); idx > maxLen/2 {
			return idx + len(sep)
		}
	}
	// Espaço
	if idx := strings.LastIndex(segment, " "); idx > maxLen/2 {
		return idx + 1
	}
	// Corte bruto
	return maxLen
}

// readStreamChunks lê o body em chunks de 8KB e chama callbacks
func readStreamChunks(ctx context.Context, body io.ReadCloser, callbacks TTSStreamCallbacks) error {
	buffer := make([]byte, TtsStreamBufSize)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := body.Read(buffer)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buffer[:n])
			if callbacks.OnChunk != nil {
				callbacks.OnChunk(chunk)
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			if callbacks.OnError != nil {
				callbacks.OnError(err)
			}
			return fmt.Errorf("failed to read stream: %w", err)
		}
	}

	if callbacks.OnDone != nil {
		callbacks.OnDone()
	}
	return nil
}

// SetVoice altera a voz padrão
func (c *TTSClient) SetVoice(voice TTSVoice) {
	c.config.Voice = voice
}

// SetSpeed altera a velocidade de fala
func (c *TTSClient) SetSpeed(speed float64) {
	if speed < 0.25 {
		speed = 0.25
	}
	if speed > 4.0 {
		speed = 4.0
	}
	c.config.Speed = speed
}

// SetModel altera o modelo de TTS
func (c *TTSClient) SetModel(model TTSModel) {
	c.config.Model = model
	if c.config.SelectionMode == "" {
		c.config.SelectionMode = selectionModeForTTSModel(string(model))
	}
}

// SetFormat altera o formato de saída
func (c *TTSClient) SetFormat(format TTSFormat) {
	c.config.Format = format
}

// listModelsSafe chama client.Models.List com proteção contra panic do SDK.
func (c *TTSClient) listModelsSafe(ctx context.Context) (page *pagination.Page[openai.Model], retErr error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[TTSClient] PANIC no SDK Models.List: %v", r)
			retErr = fmt.Errorf("panic no SDK: %v", r)
		}
	}()
	return c.client.Models.List(ctx)
}

// FetchTTSModels retorna modelos disponíveis para TTS.
func (c *TTSClient) FetchTTSModels() ([]TTSModelInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	page, err := c.listModelsSafe(ctx)
	if err != nil {
		return nil, err
	}
	if page == nil {
		return []TTSModelInfo{}, nil
	}

	var models []TTSModelInfo
	for _, m := range page.Data {
		if isTTSModel(m.ID) {
			models = append(models, TTSModelInfo{
				ID:            m.ID,
				Name:          m.ID,
				Provider:      "openai",
				SelectionMode: string(selectionModeForTTSModel(m.ID)),
			})
		}
	}
	return models, nil
}

// FetchVoices retorna vozes disponíveis para um modelo TTS específico.
func (c *TTSClient) FetchVoices(modelID string) ([]TTSVoiceInfo, error) {
	if modelID == "" {
		return nil, fmt.Errorf("model is required to list TTS voices")
	}
	if selectionModeForTTSModel(modelID) == TTSSelectionModelOnly {
		return []TTSVoiceInfo{}, nil
	}
	voices := voicesForTTSModel(modelID)
	for i := range voices {
		voices[i].ModelID = modelID
	}
	return voices, nil
}

// SpeechModelInfo informações sobre um modelo de speech (TTS ou STT).
type SpeechModelInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// staticSTTModels é o fallback quando /v1/models não está disponível.
var staticSTTModels = []SpeechModelInfo{
	{ID: "whisper-1", Name: "whisper-1"},
	{ID: "gpt-4o-transcribe", Name: "gpt-4o-transcribe"},
	{ID: "gpt-4o-mini-transcribe", Name: "gpt-4o-mini-transcribe"},
}

// StaticSTTModels retorna a lista estática de modelos STT (para uso quando não há client).
func StaticSTTModels() []SpeechModelInfo { return staticSTTModels }

// knownTTSModels são IDs exatos de modelos TTS reconhecidos.
var knownTTSModels = map[string]bool{
	"tts-1": true, "tts-1-hd": true,
	"tts-1-1106": true, "tts-1-hd-1106": true,
	"gpt-4o-mini-tts": true,
}

var staticTTSModels = []TTSModelInfo{
	{ID: "tts-1", Name: "tts-1", Provider: "openai", SelectionMode: string(TTSSelectionModelAndVoice)},
	{ID: "tts-1-hd", Name: "tts-1-hd", Provider: "openai", SelectionMode: string(TTSSelectionModelAndVoice)},
	{ID: "gpt-4o-mini-tts", Name: "gpt-4o-mini-tts", Provider: "openai", SelectionMode: string(TTSSelectionModelAndVoice)},
}

// StaticTTSModels retorna a lista de modelos TTS conhecidos da OpenAI.
func StaticTTSModels() []TTSModelInfo {
	result := make([]TTSModelInfo, len(staticTTSModels))
	copy(result, staticTTSModels)
	return result
}

// knownSTTModels são IDs exatos de modelos STT reconhecidos.
var knownSTTModels = map[string]bool{
	"whisper-1": true, "whisper-large-v3": true,
	"gpt-4o-transcribe": true, "gpt-4o-mini-transcribe": true,
}

// ttsPrefixes são prefixos heurísticos para modelos TTS não catalogados.
// - "tts-": OpenAI (tts-1, tts-1-hd)
// - "voice-": Piper/LocalAI (voice-pt_BR-cadu-medium, voice-en_US-amy-medium)
var ttsPrefixes = []string{"tts-", "voice-"}

// ttsInfixes são substrings que indicam modelo TTS quando aparecem no meio do ID.
// Ex: qwen3-tts-0.6b-custom-voice, vllm-omni-qwen3-tts-custom-voice,
// qwen3-0.6b-custom-voice, kokoro, kokoros.
var ttsInfixes = []string{"-tts-", "-tts", "custom-voice", "kokoro"}

// sttPrefixes são prefixos heurísticos para modelos STT não catalogados.
var sttPrefixes = []string{"whisper"}

// sttSuffixes são sufixos heurísticos para modelos STT não catalogados.
var sttSuffixes = []string{"-transcribe", "-asr"}

// isTTSModel retorna true se o ID é um modelo TTS conhecido ou corresponde
// a padrões heurísticos:
//   - prefixo "tts-" (OpenAI), "voice-" (Piper/LocalAI)
//   - infixo "-tts-" ou sufixo "-tts" (qwen3-tts-*, vllm-omni-*-tts-*)
func isTTSModel(id string) bool {
	lower := strings.ToLower(id)
	if knownTTSModels[lower] {
		return true
	}
	for _, p := range ttsPrefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	for _, infix := range ttsInfixes {
		if strings.Contains(lower, infix) {
			return true
		}
	}
	return false
}

func selectionModeForTTSModel(id string) TTSSelectionMode {
	lower := strings.ToLower(id)
	if strings.HasPrefix(lower, "voice-") {
		return TTSSelectionModelOnly
	}
	return TTSSelectionModelAndVoice
}

func voicesForTTSModel(modelID string) []TTSVoiceInfo {
	lower := strings.ToLower(modelID)
	switch {
	case strings.Contains(lower, "kokoro"):
		return KokoroTTSVoices()
	case strings.Contains(lower, "custom-voice"):
		return QwenCustomVoiceTTSVoices()
	default:
		return GetAvailableVoices()
	}
}

func normalizeTTSSelectionMode(modelID string, mode TTSSelectionMode) TTSSelectionMode {
	switch mode {
	case TTSSelectionModelAndVoice, TTSSelectionModelOnly:
		return mode
	default:
		return selectionModeForTTSModel(modelID)
	}
}

func validateTTSSelection(modelID, voiceID string, mode TTSSelectionMode) error {
	if modelID == "" {
		return fmt.Errorf("TTS model is required")
	}
	switch normalizeTTSSelectionMode(modelID, mode) {
	case TTSSelectionModelOnly:
		if voiceID != "" {
			return fmt.Errorf("voice_id must be empty for model-only TTS model %q", modelID)
		}
	case TTSSelectionModelAndVoice:
		if voiceID == "" {
			return fmt.Errorf("voice_id is required for TTS model %q", modelID)
		}
	}
	return nil
}

// isSTTModel retorna true se o ID é um modelo STT conhecido ou corresponde
// a padrões heurísticos (prefixo "whisper", sufixo "-transcribe"/"-asr").
func isSTTModel(id string) bool {
	lower := strings.ToLower(id)
	if knownSTTModels[lower] {
		return true
	}
	for _, p := range sttPrefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	for _, s := range sttSuffixes {
		if strings.HasSuffix(lower, s) {
			return true
		}
	}
	return false
}

// FetchSTTModels retorna modelos STT disponíveis no provider.
// Busca via /v1/models e filtra por prefixo "whisper" ou sufixo "-transcribe".
// Em caso de falha, retorna a lista estática.
func (c *TTSClient) FetchSTTModels() []SpeechModelInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	page, err := c.listModelsSafe(ctx)
	if err != nil {
		log.Printf("[FetchSTTModels] erro ao listar modelos: %v", err)
		return staticSTTModels
	}
	if page == nil {
		return staticSTTModels
	}

	var models []SpeechModelInfo
	for _, m := range page.Data {
		if isSTTModel(m.ID) {
			models = append(models, SpeechModelInfo{ID: m.ID, Name: m.ID})
		}
	}
	if len(models) == 0 {
		return staticSTTModels
	}
	return models
}

// staticVoices é a lista estática de vozes OpenAI padrão (evita reconstruir a cada chamada).
var staticVoices = []TTSVoiceInfo{
	{
		ID:          "alloy",
		Name:        "Alloy",
		Description: "Voz neutra e balanceada",
		Gender:      "neutral",
		Provider:    "openai",
	},
	{
		ID:          "ash",
		Name:        "Ash",
		Description: "Voz masculina conversacional",
		Gender:      "male",
		Provider:    "openai",
	},
	{
		ID:          "ballad",
		Name:        "Ballad",
		Description: "Voz suave e melódica",
		Gender:      "neutral",
		Provider:    "openai",
	},
	{
		ID:          "coral",
		Name:        "Coral",
		Description: "Voz feminina clara",
		Gender:      "female",
		Provider:    "openai",
	},
	{
		ID:          "echo",
		Name:        "Echo",
		Description: "Voz masculina e profunda",
		Gender:      "male",
		Provider:    "openai",
	},
	{
		ID:          "fable",
		Name:        "Fable",
		Description: "Voz expressiva, ideal para narrativas",
		Gender:      "neutral",
		Provider:    "openai",
	},
	{
		ID:          "nova",
		Name:        "Nova",
		Description: "Voz feminina jovem e energética",
		Gender:      "female",
		Provider:    "openai",
	},
	{
		ID:          "onyx",
		Name:        "Onyx",
		Description: "Voz masculina e autoritária",
		Gender:      "male",
		Provider:    "openai",
	},
	{
		ID:          "sage",
		Name:        "Sage",
		Description: "Voz calma e sábia",
		Gender:      "neutral",
		Provider:    "openai",
	},
	{
		ID:          "shimmer",
		Name:        "Shimmer",
		Description: "Voz feminina clara e expressiva",
		Gender:      "female",
		Provider:    "openai",
	},
	{
		ID:          "verse",
		Name:        "Verse",
		Description: "Voz versátil e dinâmica",
		Gender:      "neutral",
		Provider:    "openai",
	},
}

var qwenCustomVoiceVoices = []TTSVoiceInfo{
	{ID: "Vivian", Name: "Vivian", Description: "Qwen CustomVoice - feminina jovem, chinês", Gender: "female", Provider: "qwen"},
	{ID: "Serena", Name: "Serena", Description: "Qwen CustomVoice - feminina suave, chinês", Gender: "female", Provider: "qwen"},
	{ID: "Uncle_Fu", Name: "Uncle_Fu", Description: "Qwen CustomVoice - masculina grave, chinês", Gender: "male", Provider: "qwen"},
	{ID: "Dylan", Name: "Dylan", Description: "Qwen CustomVoice - masculina jovem, dialeto de Pequim", Gender: "male", Provider: "qwen"},
	{ID: "Eric", Name: "Eric", Description: "Qwen CustomVoice - masculina expressiva, dialeto de Chengdu", Gender: "male", Provider: "qwen"},
	{ID: "Ryan", Name: "Ryan", Description: "Qwen CustomVoice - masculina dinâmica, inglês", Gender: "male", Provider: "qwen"},
	{ID: "Aiden", Name: "Aiden", Description: "Qwen CustomVoice - masculina americana, inglês", Gender: "male", Provider: "qwen"},
	{ID: "Ono_Anna", Name: "Ono_Anna", Description: "Qwen CustomVoice - feminina, japonês", Gender: "female", Provider: "qwen"},
	{ID: "Sohee", Name: "Sohee", Description: "Qwen CustomVoice - feminina, coreano", Gender: "female", Provider: "qwen"},
}

var kokoroVoices = []TTSVoiceInfo{
	{ID: "af_heart", Name: "AF Heart", Description: "Kokoro - English US female", Gender: "female", Provider: "kokoro"},
	{ID: "af_alloy", Name: "AF Alloy", Description: "Kokoro - English US female", Gender: "female", Provider: "kokoro"},
	{ID: "af_aoede", Name: "AF Aoede", Description: "Kokoro - English US female", Gender: "female", Provider: "kokoro"},
	{ID: "af_bella", Name: "AF Bella", Description: "Kokoro - English US female", Gender: "female", Provider: "kokoro"},
	{ID: "af_jessica", Name: "AF Jessica", Description: "Kokoro - English US female", Gender: "female", Provider: "kokoro"},
	{ID: "af_kore", Name: "AF Kore", Description: "Kokoro - English US female", Gender: "female", Provider: "kokoro"},
	{ID: "af_nicole", Name: "AF Nicole", Description: "Kokoro - English US female", Gender: "female", Provider: "kokoro"},
	{ID: "af_nova", Name: "AF Nova", Description: "Kokoro - English US female", Gender: "female", Provider: "kokoro"},
	{ID: "af_river", Name: "AF River", Description: "Kokoro - English US female", Gender: "female", Provider: "kokoro"},
	{ID: "af_sarah", Name: "AF Sarah", Description: "Kokoro - English US female", Gender: "female", Provider: "kokoro"},
	{ID: "af_sky", Name: "AF Sky", Description: "Kokoro - English US female", Gender: "female", Provider: "kokoro"},
	{ID: "am_adam", Name: "AM Adam", Description: "Kokoro - English US male", Gender: "male", Provider: "kokoro"},
	{ID: "am_echo", Name: "AM Echo", Description: "Kokoro - English US male", Gender: "male", Provider: "kokoro"},
	{ID: "am_eric", Name: "AM Eric", Description: "Kokoro - English US male", Gender: "male", Provider: "kokoro"},
	{ID: "am_fenrir", Name: "AM Fenrir", Description: "Kokoro - English US male", Gender: "male", Provider: "kokoro"},
	{ID: "am_liam", Name: "AM Liam", Description: "Kokoro - English US male", Gender: "male", Provider: "kokoro"},
	{ID: "am_michael", Name: "AM Michael", Description: "Kokoro - English US male", Gender: "male", Provider: "kokoro"},
	{ID: "am_onyx", Name: "AM Onyx", Description: "Kokoro - English US male", Gender: "male", Provider: "kokoro"},
	{ID: "am_puck", Name: "AM Puck", Description: "Kokoro - English US male", Gender: "male", Provider: "kokoro"},
	{ID: "am_santa", Name: "AM Santa", Description: "Kokoro - English US male", Gender: "male", Provider: "kokoro"},
	{ID: "bf_alice", Name: "BF Alice", Description: "Kokoro - English UK female", Gender: "female", Provider: "kokoro"},
	{ID: "bf_emma", Name: "BF Emma", Description: "Kokoro - English UK female", Gender: "female", Provider: "kokoro"},
	{ID: "bf_isabella", Name: "BF Isabella", Description: "Kokoro - English UK female", Gender: "female", Provider: "kokoro"},
	{ID: "bf_lily", Name: "BF Lily", Description: "Kokoro - English UK female", Gender: "female", Provider: "kokoro"},
	{ID: "bm_daniel", Name: "BM Daniel", Description: "Kokoro - English UK male", Gender: "male", Provider: "kokoro"},
	{ID: "bm_fable", Name: "BM Fable", Description: "Kokoro - English UK male", Gender: "male", Provider: "kokoro"},
	{ID: "bm_george", Name: "BM George", Description: "Kokoro - English UK male", Gender: "male", Provider: "kokoro"},
	{ID: "bm_lewis", Name: "BM Lewis", Description: "Kokoro - English UK male", Gender: "male", Provider: "kokoro"},
	{ID: "pf_dora", Name: "PF Dora", Description: "Kokoro - português feminino", Gender: "female", Provider: "kokoro"},
	{ID: "pm_alex", Name: "PM Alex", Description: "Kokoro - português masculino", Gender: "male", Provider: "kokoro"},
	{ID: "pm_santa", Name: "PM Santa", Description: "Kokoro - português masculino", Gender: "male", Provider: "kokoro"},
}

func copyTTSVoices(voices []TTSVoiceInfo) []TTSVoiceInfo {
	result := make([]TTSVoiceInfo, len(voices))
	copy(result, voices)
	return result
}

// GetAvailableVoices retorna a lista de vozes disponíveis (cópia da lista estática).
func GetAvailableVoices() []TTSVoiceInfo {
	return copyTTSVoices(staticVoices)
}

func QwenCustomVoiceTTSVoices() []TTSVoiceInfo {
	return copyTTSVoices(qwenCustomVoiceVoices)
}

func KokoroTTSVoices() []TTSVoiceInfo {
	return copyTTSVoices(kokoroVoices)
}
