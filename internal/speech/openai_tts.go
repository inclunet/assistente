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
	BaseURL           string    // URL base (vazio = default OpenAI)
	CredentialPattern string    // padrão para credential transport (ex: "api.openai.com")
	Model             TTSModel  // "tts-1" ou "tts-1-hd"
	Voice             TTSVoice  // alloy, echo, fable, onyx, nova, shimmer
	Format            TTSFormat // mp3, opus, aac, flac, wav, pcm
	Speed             float64   // 0.25 a 4.0 (1.0 = normal)
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
}

// NewTTSClient cria um novo cliente TTS usando o SDK openai-go.
// Credenciais são injetadas automaticamente via CredentialTransport,
// usando a mesma estratégia do LLM provider.
func NewTTSClient(config TTSConfig, credMgr *credentials.Manager) *TTSClient {
	if config.Model == "" {
		config.Model = ModelTTS1
	}
	if config.Voice == "" {
		config.Voice = VoiceNova
	}
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
func (c *TTSClient) buildParams(text string, voice TTSVoice) openai.AudioSpeechNewParams {
	params := openai.AudioSpeechNewParams{
		Input: text,
		Model: openai.SpeechModel(c.config.Model),
		Voice: openai.AudioSpeechNewParamsVoice(voice),
	}
	if c.config.Speed != 1.0 {
		params.Speed = param.NewOpt(c.config.Speed)
	}
	if c.config.Format != "" {
		params.ResponseFormat = openai.AudioSpeechNewParamsResponseFormat(c.config.Format)
	}
	return params
}

// Synthesize converte texto em áudio.
// Retorna os bytes do áudio no formato configurado.
// Textos maiores que 4000 chars são divididos em chunks e sintetizados sequencialmente.
func (c *TTSClient) Synthesize(text string) ([]byte, error) {
	if text == "" {
		return nil, fmt.Errorf("text cannot be empty")
	}

	chunks := splitTextForTTS(text)
	var allData []byte
	for _, chunk := range chunks {
		params := c.buildParams(chunk, c.config.Voice)
		resp, err := c.client.Audio.Speech.New(context.Background(), params)
		if err != nil {
			return nil, fmt.Errorf("TTS synthesis failed: %w", err)
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read TTS response: %w", err)
		}
		allData = append(allData, data...)
	}
	return allData, nil
}

// SynthesizeWithVoice converte texto em áudio usando uma voz específica
func (c *TTSClient) SynthesizeWithVoice(text string, voice TTSVoice) ([]byte, error) {
	if text == "" {
		return nil, fmt.Errorf("text cannot be empty")
	}

	chunks := splitTextForTTS(text)
	var allData []byte
	for _, chunk := range chunks {
		params := c.buildParams(chunk, voice)
		resp, err := c.client.Audio.Speech.New(context.Background(), params)
		if err != nil {
			return nil, fmt.Errorf("TTS synthesis failed: %w", err)
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read TTS response: %w", err)
		}
		allData = append(allData, data...)
	}
	return allData, nil
}

// TTSStreamCallbacks callbacks para streaming de áudio
type TTSStreamCallbacks struct {
	OnChunk func(chunk []byte) // Chamado para cada chunk de áudio recebido
	OnDone  func()             // Chamado quando streaming termina com sucesso
	OnError func(err error)    // Chamado em caso de erro
}

// SynthesizeStream converte texto em áudio com streaming.
// O SDK retorna um *http.Response; lemos o body em chunks.
func (c *TTSClient) SynthesizeStream(ctx context.Context, text string, callbacks TTSStreamCallbacks) error {
	if text == "" {
		return fmt.Errorf("text cannot be empty")
	}

	chunks := splitTextForTTS(text)
	for _, chunk := range chunks {
		params := c.buildParams(chunk, c.config.Voice)
		resp, err := c.client.Audio.Speech.New(ctx, params)
		if err != nil {
			return fmt.Errorf("TTS stream failed: %w", err)
		}
		if err := readStreamChunks(ctx, resp.Body, callbacks); err != nil {
			resp.Body.Close()
			return err
		}
		resp.Body.Close()
	}

	return nil
}

// SynthesizeStreamWithVoice converte texto em áudio com streaming usando uma voz específica
func (c *TTSClient) SynthesizeStreamWithVoice(ctx context.Context, text string, voice TTSVoice, callbacks TTSStreamCallbacks) error {
	if text == "" {
		return fmt.Errorf("text cannot be empty")
	}

	chunks := splitTextForTTS(text)
	for _, chunk := range chunks {
		params := c.buildParams(chunk, voice)
		resp, err := c.client.Audio.Speech.New(ctx, params)
		if err != nil {
			return fmt.Errorf("TTS stream failed: %w", err)
		}
		if err := readStreamChunks(ctx, resp.Body, callbacks); err != nil {
			resp.Body.Close()
			return err
		}
		resp.Body.Close()
	}

	return nil
}

// splitTextForTTS divide texto longo em chunks de no máximo 4096 caracteres,
// quebrando em limites de frase/parágrafo para manter naturalidade.
func splitTextForTTS(text string) []string {
	const maxChunkSize = 4000 // margem de segurança abaixo do limite de 4096

	if len(text) <= maxChunkSize {
		return []string{text}
	}

	var chunks []string
	remaining := text

	for len(remaining) > 0 {
		if len(remaining) <= maxChunkSize {
			chunks = append(chunks, remaining)
			break
		}

		// Procura melhor ponto de quebra dentro do limite
		cutPoint := findBreakPoint(remaining, maxChunkSize)
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
	buffer := make([]byte, 8192)

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

// FetchVoices retorna vozes disponíveis para TTS.
// Para provedores com modelos TTS personalizados (ex: Piper/LocalAI com voice-*,
// qwen3-tts-*), retorna esses modelos como vozes — pois em backends como
// LocalAI cada modelo Piper corresponde a uma voz.
// Para provedores padrão (OpenAI com tts-1), retorna a lista estática de vozes.
func (c *TTSClient) FetchVoices() ([]TTSVoiceInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	page, err := c.listModelsSafe(ctx)
	if err != nil {
		return GetAvailableVoices(), nil
	}
	if page == nil {
		return GetAvailableVoices(), nil
	}

	var dynamicVoices []TTSVoiceInfo
	hasStandardTTS := false

	for _, m := range page.Data {
		if isTTSModel(m.ID) {
			lower := strings.ToLower(m.ID)
			if knownTTSModels[lower] {
				hasStandardTTS = true
			} else {
				dynamicVoices = append(dynamicVoices, TTSVoiceInfo{
					ID:       m.ID,
					Name:     m.ID,
					Provider: "localai",
				})
			}
		}
	}

	if len(dynamicVoices) > 0 {
		if hasStandardTTS {
			return append(GetAvailableVoices(), dynamicVoices...), nil
		}
		return dynamicVoices, nil
	}

	return GetAvailableVoices(), nil
}

// SpeechModelInfo informações sobre um modelo de speech (TTS ou STT).
type SpeechModelInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// staticTTSModels é o fallback quando /v1/models não está disponível.
var staticTTSModels = []SpeechModelInfo{
	{ID: "tts-1", Name: "tts-1"},
	{ID: "tts-1-hd", Name: "tts-1-hd"},
}

// staticSTTModels é o fallback quando /v1/models não está disponível.
var staticSTTModels = []SpeechModelInfo{
	{ID: "whisper-1", Name: "whisper-1"},
	{ID: "gpt-4o-transcribe", Name: "gpt-4o-transcribe"},
	{ID: "gpt-4o-mini-transcribe", Name: "gpt-4o-mini-transcribe"},
}

// StaticTTSModels retorna a lista estática de modelos TTS (para uso quando não há client).
func StaticTTSModels() []SpeechModelInfo { return staticTTSModels }

// StaticSTTModels retorna a lista estática de modelos STT (para uso quando não há client).
func StaticSTTModels() []SpeechModelInfo { return staticSTTModels }

// knownTTSModels são IDs exatos de modelos TTS reconhecidos.
var knownTTSModels = map[string]bool{
	"tts-1": true, "tts-1-hd": true,
	"tts-1-1106": true, "tts-1-hd-1106": true,
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
// Ex: qwen3-tts-0.6b-custom-voice, vllm-omni-qwen3-tts-custom-voice
var ttsInfixes = []string{"-tts-", "-tts"}

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

// IsDynamicTTSModel retorna true se o ID é um modelo TTS dinâmico (não-padrão),
// ou seja, é detectado como TTS mas NÃO é um dos modelos padrão (tts-1, tts-1-hd, etc.).
// Usado para provedores como LocalAI/Piper onde a "voz" selecionada é na verdade um modelo.
func IsDynamicTTSModel(id string) bool {
	lower := strings.ToLower(id)
	return isTTSModel(id) && !knownTTSModels[lower]
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

// FetchTTSModels retorna modelos TTS disponíveis no provider.
// Busca via /v1/models e filtra por prefixo "tts-".
// Em caso de falha, retorna a lista estática (tts-1, tts-1-hd).
func (c *TTSClient) FetchTTSModels() []SpeechModelInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	page, err := c.listModelsSafe(ctx)
	if err != nil {
		log.Printf("[FetchTTSModels] erro ao listar modelos: %v", err)
		return staticTTSModels
	}
	if page == nil {
		return staticTTSModels
	}

	var models []SpeechModelInfo
	for _, m := range page.Data {
		if isTTSModel(m.ID) {
			models = append(models, SpeechModelInfo{ID: m.ID, Name: m.ID})
		}
	}
	if len(models) == 0 {
		return staticTTSModels
	}
	return models
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

// GetAvailableVoices retorna a lista de vozes disponíveis
func GetAvailableVoices() []TTSVoiceInfo {
	return []TTSVoiceInfo{
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
}
