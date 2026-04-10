package sip

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"assistente/internal/messaging"
	"assistente/internal/speech"

	"github.com/emiago/diago"
	diagoaudio "github.com/emiago/diago/audio"
	resampling "github.com/tphakala/go-audio-resampling"
)

// AudioPipelineConfig configura o pipeline de ├íudio bidirecional.
type AudioPipelineConfig struct {
	// VAD ajusta o detector de atividade de voz
	VAD VADConfig

	// ReadFrameSize ├® o tamanho do frame lido do RTP em bytes (PCM 16-bit).
	// Padr├úo: 320 (20ms @ 8kHz mono 16-bit = 160 samples * 2 bytes)
	ReadFrameSize int

	// InputSampleRate ├® a taxa de amostragem do ├íudio de entrada (RTP/G.711).
	// Padr├úo: 8000
	InputSampleRate int

	// STTSampleRate ├® a taxa de amostragem esperada pelo STT (Whisper).
	// Padr├úo: 16000
	STTSampleRate int

	// TTSOutputFormat ├® o formato de ├íudio solicitado ao TTS.
	// "pcm" retorna raw PCM 24kHz 16-bit mono (OpenAI).
	// Padr├úo: "pcm"
	TTSOutputFormat string

	// EchoCooldownDuration é o tempo de supressão de eco após fim do playback.
	// Padrão: 300ms
	EchoCooldownDuration time.Duration

	// STTMaxRetries é o número máximo de tentativas para STT em erros transitórios.
	// Padrão: 2
	STTMaxRetries int

	// InputGain é o fator de amplificação aplicado ao áudio de entrada após
	// decodificação G.711. Compensa sinais fracos de SBCs/gateways telefônicos.
	// 1.0 = sem ganho, 10.0 = 10x amplificação. Clipping é prevenido.
	// Padrão: 10.0
	InputGain float64

	// BargeInRMSThreshold é o nível RMS mínimo para detectar barge-in durante
	// reprodução de TTS. Deve ser acima do nível de eco típico do softphone.
	// Padrão: 0.15
	BargeInRMSThreshold float64

	// BargeInMinFrames é o número mínimo de frames consecutivos acima do threshold
	// para confirmar barge-in. Cada frame = 20ms.
	// Padrão: 5 (100ms)
	BargeInMinFrames int
}

// DefaultAudioPipelineConfig retorna configuração padrão para pipeline SIP.
func DefaultAudioPipelineConfig() AudioPipelineConfig {
	return AudioPipelineConfig{
		VAD:                  DefaultVADConfig(),
		ReadFrameSize:        320,
		InputSampleRate:      8000,
		STTSampleRate:        16000,
		TTSOutputFormat:      "pcm",
		EchoCooldownDuration: 300 * time.Millisecond,
		STTMaxRetries:        2,
		InputGain:            15.0,
		BargeInRMSThreshold:  0.15,
		BargeInMinFrames:     5,
	}
}

// AudioPipeline gerencia o fluxo de ├íudio bidirecional de uma chamada SIP.
//
// Fluxo de entrada (call → LLM):
//
//	RTP → AudioReader (G.711 µ-law/A-law) → Decode → PCM 8kHz → VAD → Speech Segment → Resample 16kHz → WAV → Whisper STT → texto → Gateway
//
// DialogSession abstrai os m├®todos comuns entre DialogServerSession (inbound)
// e DialogClientSession (outbound) do diago, permitindo o pipeline funcionar
// com ambos os tipos de chamada.
type DialogSession interface {
	AudioReader(opts ...diago.AudioReaderOption) (io.Reader, error)
	PlaybackControlCreate() (diago.AudioPlaybackControl, error)
	PlaybackCreate() (diago.AudioPlayback, error)
	Context() context.Context
	Id() string
	Hangup(ctx context.Context) error
}

// Fluxo de sa├¡da (LLM ÔåÆ call):
//
//	Gateway resposta ÔåÆ SIPAdapter.Send() ÔåÆ PlayAudio() ÔåÆ PCM ÔåÆ AudioWriter ÔåÆ RTP
type AudioPipeline struct {
	config  AudioPipelineConfig
	dialog  DialogSession
	vad     *EnergyVAD
	call    *CallSession
	handler messaging.IncomingMessageHandler

	// SpeechManager para STT (Whisper)
	speechManager *speech.SpeechManager

	// OnBargeIn ├® chamado quando o usu├írio interrompe a reprodu├º├úo (fala durante TTS).
	// Permite ao adapter cancelar o streaming LLM em andamento para a conversa.
	OnBargeIn func()

	// playback control├ível para barge-in (protegido por playbackMu)
	playback   *diago.AudioPlaybackControl
	playbackMu sync.Mutex

	// Supress├úo de eco: VAD ├® inibido durante playback e por um cooldown ap├│s.
	// O softphone remoto reproduz o TTS no speaker ÔåÆ microfone capta ÔåÆ envia
	// de volta via RTP ÔåÆ VAD detectaria como fala (false onset).
	playbackActive  atomic.Bool
	playbackEndedAt atomic.Int64 // UnixNano do fim do ├║ltimo playback

	ctx    context.Context
	cancel context.CancelFunc

	// processingWg rastreia goroutines de processamento ativas.
	processingWg sync.WaitGroup
}

// NewAudioPipeline cria um novo pipeline de ├íudio para uma chamada SIP.
func NewAudioPipeline(
	dialog DialogSession,
	call *CallSession,
	handler messaging.IncomingMessageHandler,
	speechMgr *speech.SpeechManager,
	cfg AudioPipelineConfig,
) *AudioPipeline {
	ctx, cancel := context.WithCancel(dialog.Context())

	p := &AudioPipeline{
		config:        cfg,
		dialog:        dialog,
		call:          call,
		handler:       handler,
		speechManager: speechMgr,
		ctx:           ctx,
		cancel:        cancel,
	}

	return p
}

// Run inicia o loop de captura de áudio → VAD → STT.
// Bloqueia até a chamada terminar (contexto cancelado ou BYE).
func (p *AudioPipeline) Run() error {
	// Obtém reader de áudio e detecta codec negociado (µ-law / A-law).
	// AudioReader retorna áudio CODIFICADO (G.711), não PCM.
	var mediaProps diago.MediaProps
	reader, err := p.dialog.AudioReader(diago.WithAudioReaderMediaProps(&mediaProps))
	if err != nil {
		return fmt.Errorf("sip pipeline: erro ao obter audio reader: %w", err)
	}

	// Seleciona decoder G.711 baseado no codec negociado via SDP.
	// G.711 µ-law (PCMU, PT=0) e A-law (PCMA, PT=8) usam 1 byte/sample.
	// Decodificação produz PCM 16-bit signed LE (2 bytes/sample).
	codecName := mediaProps.Codec.Name
	var decodeG711 func(lpcm []byte, encoded []byte) (int, error)
	switch codecName {
	case "PCMA":
		decodeG711 = diagoaudio.DecodeAlawTo
	default:
		// PCMU é o padrão para SIP
		decodeG711 = diagoaudio.DecodeUlawTo
		if codecName != "PCMU" && codecName != "" {
			log.Printf("[SIP Pipeline] Codec %q desconhecido, assumindo PCMU", codecName)
		}
	}
	log.Printf("[SIP Pipeline] Codec negociado: %s (PT=%d, rate=%d), media local=%s remote=%s",
		codecName, mediaProps.Codec.PayloadType, mediaProps.Codec.SampleRate,
		mediaProps.Laddr, mediaProps.Raddr)

	// Configura VAD com callbacks
	p.vad = NewEnergyVAD(
		p.config.VAD,
		p.onSpeechStart,
		p.onSpeechEnd,
	)

	// Frame buffer para leitura de dados codificados G.711.
	// G.711: 1 byte/sample, 160 bytes = 20ms @ 8kHz.
	// Usamos buffer maior para acomodar pacotes RTP variáveis.
	frameSize := p.config.ReadFrameSize
	if frameSize == 0 {
		frameSize = 320
	}
	encodedBuf := make([]byte, frameSize)
	// PCM 16-bit: 2 bytes por sample G.711, então buffer = 2x o tamanho
	pcmBuf := make([]byte, frameSize*2)

	// Cooldown após playback: tempo para eco do softphone remoto dissipar.
	const echoCooldown = 600 * time.Millisecond

	// Ganho de entrada para compensar sinais fracos de SBCs telefônicos.
	inputGain := p.config.InputGain
	if inputGain <= 0 {
		inputGain = 1.0
	}

	// Diagnóstico: loga RMS a cada ~5 segundos para verificar se áudio real chega.
	var diagFrames int
	var diagMaxRMS float64

	// Barge-in durante playback: detecta fala do usuário por energia alta.
	bargeInThreshold := p.config.BargeInRMSThreshold
	if bargeInThreshold <= 0 {
		bargeInThreshold = 0.15
	}
	bargeInMinFrames := p.config.BargeInMinFrames
	if bargeInMinFrames <= 0 {
		bargeInMinFrames = 5
	}
	var bargeInFrames int

	log.Printf("[SIP Pipeline] Iniciado para chamada %s (gain=%.1fx)", p.call.ID, inputGain)

	for {
		select {
		case <-p.ctx.Done():
			// Chamada encerrada — flush VAD para emitir segmento final
			p.vad.Flush()
			log.Printf("[SIP Pipeline] Encerrado para chamada %s", p.call.ID)
			return nil
		default:
		}

		n, err := reader.Read(encodedBuf)
		if err != nil {
			if err == io.EOF || p.ctx.Err() != nil {
				p.vad.Flush()
				return nil
			}
			return fmt.Errorf("sip pipeline: erro ao ler áudio: %w", err)
		}

		if n > 0 {
			// Decodifica G.711 (µ-law/A-law) → PCM 16-bit signed LE.
			// Decodifica SEMPRE — mesmo durante playback — para barge-in.
			pcmN, decErr := decodeG711(pcmBuf, encodedBuf[:n])
			if decErr != nil {
				log.Printf("[SIP Pipeline] Erro ao decodificar G.711: %v", decErr)
				continue
			}

			// Aplica ganho de entrada para compensar sinais fracos do SBC.
			if inputGain != 1.0 {
				applyGain(pcmBuf[:pcmN], inputGain)
			}

			// Diagnóstico: calcula RMS do frame (pós-ganho) e loga a cada ~5s.
			rms := computeRMS(pcmBuf[:pcmN])
			if rms > diagMaxRMS {
				diagMaxRMS = rms
			}
			diagFrames++
			if diagFrames%250 == 0 {
				log.Printf("[SIP Diag] frames=%d, maxRMS=%.4f, gain=%.0fx",
					diagFrames, diagMaxRMS, inputGain)
				diagMaxRMS = 0
			}

			// Durante playback: detecta barge-in por energia, mas NÃO alimenta VAD.
			// O eco do TTS pelo speaker remoto tem RMS moderado; fala real do
			// usuário sobre o eco produz RMS muito mais alto.
			if p.playbackActive.Load() {
				if rms > bargeInThreshold {
					bargeInFrames++
					if bargeInFrames >= bargeInMinFrames {
						log.Printf("[SIP Pipeline] Barge-in durante playback (RMS=%.4f, frames=%d)", rms, bargeInFrames)
						p.StopPlayback()
						if p.OnBargeIn != nil {
							func() {
								defer func() {
									if r := recover(); r != nil {
										log.Printf("[SIP Pipeline] Panic em OnBargeIn (barge-in playback): %v", r)
									}
								}()
								p.OnBargeIn()
							}()
						}
						bargeInFrames = 0
					}
				} else {
					bargeInFrames = 0
				}
				continue
			}

			// Supressão de eco pós-playback: ignora frames por um curto período
			// após o playback terminar naturalmente (não por barge-in).
			if endNano := p.playbackEndedAt.Load(); endNano > 0 {
				if time.Since(time.Unix(0, endNano)) < echoCooldown {
					continue
				}
			}

			p.vad.ProcessFrame(pcmBuf[:pcmN])
		}
	}
}

// Stop encerra o pipeline e aguarda goroutines de processamento pendentes.
func (p *AudioPipeline) Stop() {
	p.cancel()
	done := make(chan struct{})
	go func() {
		p.processingWg.Wait()
		close(done)
	}()
	select {
	case <-done:
		// OK
	case <-time.After(5 * time.Second):
		log.Printf("[SIP Pipeline] Timeout aguardando processamento de segmentos (chamada %s)", p.call.ID)
	}
}

// PlayAudio reproduz ├íudio PCM na chamada ativa.
// O ├íudio deve ser PCM 16-bit signed LE mono na taxa de sa├¡da do codec
// (tipicamente 8kHz para G.711).
// Retorna true se a reprodu├º├úo completou, false se foi interrompida (barge-in).
func (p *AudioPipeline) PlayAudio(pcmData []byte) (bool, error) {
	if len(pcmData) == 0 {
		return true, nil
	}

	playback, err := p.dialog.PlaybackControlCreate()
	if err != nil {
		return false, fmt.Errorf("sip pipeline: erro ao criar playback: %w", err)
	}

	p.playbackActive.Store(true)
	p.playbackMu.Lock()
	p.playback = &playback
	p.playbackMu.Unlock()

	reader := bytes.NewReader(pcmData)
	_, err = playback.Play(reader, "audio/pcm")
	completed := err == nil

	p.playbackMu.Lock()
	p.playback = nil
	p.playbackMu.Unlock()
	p.playbackActive.Store(false)
	if completed {
		// Playback natural: aplica cooldown de eco
		p.playbackEndedAt.Store(time.Now().UnixNano())
	} else {
		// Barge-in: sem cooldown — VAD deve captar fala imediatamente
		p.playbackEndedAt.Store(0)
	}
	return completed, err
}

// StopPlayback interrompe a reprodu├º├úo atual (barge-in).
func (p *AudioPipeline) StopPlayback() {
	p.playbackMu.Lock()
	pb := p.playback
	p.playbackMu.Unlock()
	if pb != nil {
		pb.Stop()
	}
}

// PlayStreamingAudio reproduz ├íudio de um io.Reader na chamada ativa.
// O reader deve fornecer PCM 16-bit signed LE mono 8kHz.
// Bloqueia at├® o reader retornar EOF ou a playback ser interrompida.
func (p *AudioPipeline) PlayStreamingAudio(reader io.Reader) (bool, error) {
	playback, err := p.dialog.PlaybackControlCreate()
	if err != nil {
		return false, fmt.Errorf("sip pipeline: erro ao criar playback streaming: %w", err)
	}

	p.playbackActive.Store(true)
	p.playbackMu.Lock()
	p.playback = &playback
	p.playbackMu.Unlock()

	_, err = playback.Play(reader, "audio/pcm")
	completed := err == nil

	p.playbackMu.Lock()
	p.playback = nil
	p.playbackMu.Unlock()
	p.playbackActive.Store(false)
	if completed {
		p.playbackEndedAt.Store(time.Now().UnixNano())
	} else {
		p.playbackEndedAt.Store(0)
	}
	return completed, err
}

// SpeakText sintetiza texto via TTS e reproduz na chamada SIP.
// Pede WAV (PCM lossless) ao TTS, coleta o áudio inteiro, resampla de uma vez
// para 8kHz (evitando artefatos de chunk boundary), e envia ao diago.
// Cadeia: TTS → WAV lossless → one-shot resample 8kHz → diago codifica G.711.
func (p *AudioPipeline) SpeakText(ctx context.Context, text, voiceID string) (bool, error) {
	if p.speechManager == nil {
		return false, fmt.Errorf("sip pipeline: speech manager indisponível")
	}

	streamCtx, streamCancel := context.WithCancel(ctx)
	defer streamCancel()

	// Coleta todo o WAV em memória. Para TTS de voz (frases curtas),
	// o áudio inteiro ocupa poucos KB — streaming não compensa a complexidade.
	var wavBuf bytes.Buffer
	err := p.speechManager.SynthesizeStreamRaw(streamCtx, text, voiceID, "wav", speech.TTSStreamCallbacks{
		OnChunk: func(chunk []byte) { wavBuf.Write(chunk) },
		OnDone:  func() {},
		OnError: func(err error) {},
	})
	if err != nil {
		return false, fmt.Errorf("sip pipeline: TTS falhou: %w", err)
	}

	// Converte WAV → PCM 8kHz mono (one-shot, sem artefatos de chunk)
	pcm8k, convErr := wavToPCM8kMono(wavBuf.Bytes())
	if convErr != nil {
		return false, fmt.Errorf("sip pipeline: conversão WAV→8kHz: %w", convErr)
	}

	log.Printf("[SIP Pipeline] TTS pronto: %d bytes PCM 8kHz (%dms)",
		len(pcm8k), len(pcm8k)/16) // 8000 samples/s * 2 bytes = 16 bytes/ms

	return p.PlayAudio(pcm8k)
}

// wavToPCM8kMono converte WAV (qualquer rate/canais) para PCM 16-bit 8kHz mono.
// Faz resample one-shot para evitar artefatos de chunk boundary.
func wavToPCM8kMono(wavData []byte) ([]byte, error) {
	r := bytes.NewReader(wavData)
	srcRate, channels, err := parseWAVHeader(r)
	if err != nil {
		return nil, err
	}

	// Lê todos os dados PCM de uma vez
	pcmData, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler PCM do WAV: %w", err)
	}

	// Stereo → mono se necessário
	if channels == 2 {
		pcmData = stereoToMono(pcmData)
	}

	// Se já está a 8kHz, não precisa resample
	if srcRate == 8000 {
		return pcmData, nil
	}

	// Converte int16 → float64 normalizado [-1,1]
	numSamples := len(pcmData) / 2
	floats := make([]float64, numSamples)
	for i := 0; i < numSamples; i++ {
		s := int16(binary.LittleEndian.Uint16(pcmData[i*2:]))
		floats[i] = float64(s) / 32768.0
	}

	// Resample one-shot (polyphase FIR + Kaiser window, sem chunk boundary)
	resampled, err := resampling.ResampleMono(floats, float64(srcRate), 8000, resampling.QualityHigh)
	if err != nil {
		return nil, fmt.Errorf("resample %d→8000: %w", srcRate, err)
	}

	// Converte float64 → int16 LE
	out := make([]byte, len(resampled)*2)
	for i, f := range resampled {
		if f > 1.0 {
			f = 1.0
		} else if f < -1.0 {
			f = -1.0
		}
		binary.LittleEndian.PutUint16(out[i*2:], uint16(int16(f*32767.0)))
	}
	return out, nil
}

// parseWAVHeader lê o header WAV de um reader e retorna sample rate e
// número de canais. Após retornar, o reader está posicionado no início dos dados PCM.
func parseWAVHeader(r io.Reader) (sampleRate, channels int, err error) {
	header := make([]byte, 12)
	if _, err = io.ReadFull(r, header); err != nil {
		return 0, 0, fmt.Errorf("erro ao ler RIFF header: %w", err)
	}
	if string(header[0:4]) != "RIFF" || string(header[8:12]) != "WAVE" {
		return 0, 0, fmt.Errorf("não é um arquivo WAV válido")
	}

	chunkHeader := make([]byte, 8)
	for {
		if _, err = io.ReadFull(r, chunkHeader); err != nil {
			return 0, 0, fmt.Errorf("erro ao ler chunk header: %w", err)
		}
		chunkID := string(chunkHeader[0:4])
		chunkSize := int64(binary.LittleEndian.Uint32(chunkHeader[4:8]))

		switch chunkID {
		case "fmt ":
			if chunkSize < 16 {
				return 0, 0, fmt.Errorf("fmt chunk muito pequeno: %d", chunkSize)
			}
			fmtData := make([]byte, chunkSize)
			if _, err = io.ReadFull(r, fmtData); err != nil {
				return 0, 0, fmt.Errorf("erro ao ler fmt chunk: %w", err)
			}
			channels = int(binary.LittleEndian.Uint16(fmtData[2:4]))
			sampleRate = int(binary.LittleEndian.Uint32(fmtData[4:8]))
		case "data":
			return sampleRate, channels, nil
		default:
			if _, err = io.CopyN(io.Discard, r, chunkSize); err != nil {
				return 0, 0, fmt.Errorf("erro ao pular chunk %q: %w", chunkID, err)
			}
		}
	}
}

// onSpeechStart é chamado quando o VAD detecta início de fala.
func (p *AudioPipeline) onSpeechStart() {
	// Verifica se o pipeline já foi encerrado antes de agir
	select {
	case <-p.ctx.Done():
		return
	default:
	}

	log.Printf("[SIP Pipeline] Fala detectada na chamada %s", p.call.ID)

	// Se estiver tocando, interrompe (barge-in)
	p.StopPlayback()

	// Cancela streaming LLM em andamento (se houver)
	if p.OnBargeIn != nil {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[SIP Pipeline] Panic em OnBargeIn (chamada %s): %v", p.call.ID, r)
			}
		}()
		p.OnBargeIn()
	}
}

// onSpeechEnd ├® chamado quando o VAD detecta fim de segmento de fala.
// Processa em goroutine para n├úo bloquear o loop de leitura de frames do VAD.
func (p *AudioPipeline) onSpeechEnd(segment []byte) {
	if len(segment) == 0 {
		return
	}

	// Verifica se o pipeline já foi encerrado
	select {
	case <-p.ctx.Done():
		return
	default:
	}

	// Copia o segmento pois o VAD pode reutilizar o buffer
	seg := make([]byte, len(segment))
	copy(seg, segment)

	// Rastreia goroutine via WaitGroup para Stop() poder aguardá-la e evitar leak
	p.processingWg.Add(1)
	go func() {
		defer p.processingWg.Done()
		p.processSegment(seg)
	}()
}

// isWhisperHallucination detecta saídas conhecidas do Whisper quando recebe
// ruído, silêncio ou áudio ininteligível. Esses artefatos não devem ser enviados
// ao LLM como transcrição do usuário.
func isWhisperHallucination(text string) bool {
	t := strings.TrimSpace(strings.ToLower(text))
	// Marcadores comuns de silêncio/ruído
	hallucinations := []string{
		"[blank_audio]",
		"[silence]",
		"[music]",
		"[applause]",
		"[laughter]",
		"[inaudible]",
		"[no speech]",
		"(silence)",
		"(music)",
		"you",           // Whisper "you" hallucination on silence
		"thank you.",     // Common hallucination on short noise
		"thanks.",
		"bye.",
		"thank you for watching.",
		"thanks for watching.",
		"subscribe.",
	}
	for _, h := range hallucinations {
		if t == h {
			return true
		}
	}
	return false
}

// processSegment processa um segmento de fala: trim, resample, WAV, STT, envia ao gateway.
// Executado em goroutine separada com prote├º├úo contra panic para n├úo derrubar a chamada.
func (p *AudioPipeline) processSegment(segment []byte) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[SIP Pipeline] PANIC em processSegment (chamada %s): %v", p.call.ID, r)
		}
	}()

	// Remove trailing silence usando noise floor adaptativo do VAD.
	noiseFloor := 0.01 // fallback m├¡nimo
	if p.vad != nil {
		noiseFloor = p.vad.NoiseFloor()
	}
	segment = trimTrailingSilence(segment, p.config.InputSampleRate, noiseFloor)

	duration := time.Duration(len(segment)/2) * time.Second / time.Duration(p.config.InputSampleRate)

	// Ignora segmentos muito curtos (< 300ms) ÔÇö provavelmente ru├¡do, n├úo fala
	if duration < 300*time.Millisecond {
		log.Printf("[SIP Pipeline] Segmento descartado (%.1fs < 0.3s) na chamada %s",
			duration.Seconds(), p.call.ID)
		return
	}

	log.Printf("[SIP Pipeline] Segmento de fala: %d bytes (%.1fs) na chamada %s",
		len(segment), duration.Seconds(), p.call.ID)

	// Resample 8kHz ÔåÆ 16kHz para Whisper
	pcm16k := Resample8to16(segment)

	// Encoda como WAV
	wavData := encodePCMToWAV(pcm16k, uint32(p.config.STTSampleRate), 16, 1)

	// Envia para STT (Whisper) com timeout de 30s
	audioBase64 := base64.StdEncoding.EncodeToString(wavData)

	if p.speechManager == nil {
		log.Printf("[SIP Pipeline] SpeechManager indispon├¡vel, ignorando segmento")
		return
	}

	maxRetries := p.config.STTMaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}

	var result *speech.TranscriptionResult
	var err error
	retryDelay := 500 * time.Millisecond
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if p.ctx.Err() != nil {
			log.Printf("[SIP Pipeline] STT cancelado (chamada encerrada): %s", p.call.ID)
			return
		}
		if attempt > 0 {
			log.Printf("[SIP Pipeline] STT retry %d/%d para chamada %s (aguardando %s)",
				attempt, maxRetries, p.call.ID, retryDelay)
			select {
			case <-p.ctx.Done():
				log.Printf("[SIP Pipeline] STT cancelado durante retry: %s", p.call.ID)
				return
			case <-time.After(retryDelay):
			}
			retryDelay *= 2
			if retryDelay > 5*time.Second {
				retryDelay = 5 * time.Second
			}
		}

		sttCtx, sttCancel := context.WithTimeout(p.ctx, 30*time.Second)
		result, err = p.speechManager.TranscribeWithContext(sttCtx, audioBase64, "audio.wav")
		sttCancel()

		if err == nil {
			break
		}
		if p.ctx.Err() != nil {
			log.Printf("[SIP Pipeline] STT cancelado (chamada encerrada): %s", p.call.ID)
			return
		}
		log.Printf("[SIP Pipeline] Erro STT (tentativa %d/%d): %v", attempt+1, maxRetries+1, err)
	}
	if err != nil {
		log.Printf("[SIP Pipeline] STT falhou após %d tentativas, segmento perdido (chamada %s)",
			maxRetries+1, p.call.ID)
		return
	}

	if result == nil || result.Text == "" {
		log.Printf("[SIP Pipeline] STT retornou texto vazio, ignorando")
		return
	}

	// Filtra marcadores de alucinação do Whisper (segmentos de ruído/silêncio)
	if isWhisperHallucination(result.Text) {
		log.Printf("[SIP Pipeline] STT descartado (hallucination): %q", result.Text)
		return
	}

	log.Printf("[SIP Pipeline] STT: \"%s\" (chamada %s)", result.Text, p.call.ID)

	// Envia texto transcrito para o gateway como IncomingMessage
	if p.handler != nil {
		msg := messaging.IncomingMessage{
			ID: fmt.Sprintf("%s-stt-%d", p.call.ID, time.Now().UnixMilli()),
			From: messaging.Contact{
				ID:          p.call.CallerID,
				DisplayName: p.call.CallerName,
				Username:    p.call.CallerID,
			},
			Text:    result.Text,
			Channel: "sip",
			Attachments: []messaging.Attachment{
				{
					Filename: "audio.wav",
					MIMEType: "audio/wav",
					Data:     wavData,
					Size:     int64(len(wavData)),
				},
			},
			Timestamp: time.Now(),
		}
		p.handler(p.ctx, msg)
	}
}

// trimTrailingSilence remove sil├¬ncio do final do segmento PCM 16-bit mono.
// Preserva pelo menos 100ms de trailing silence para n├úo cortar a fala.
// noiseFloor ├® o n├¡vel RMS do ru├¡do de fundo estimado pelo VAD (adaptativo).
func trimTrailingSilence(pcm []byte, sampleRate int, noiseFloor float64) []byte {
	if len(pcm) < 4 {
		return pcm
	}

	frameSize := sampleRate / 50 // 20ms em samples
	if frameSize < 1 {
		frameSize = 160
	}
	frameSizeBytes := frameSize * 2
	numSamples := len(pcm) / 2

	// Threshold adaptativo: 2x o noise floor, com m├¡nimo de 0.01
	silenceRMS := noiseFloor * 2.0
	if silenceRMS < 0.01 {
		silenceRMS = 0.01
	}

	// Encontra o ├║ltimo frame n├úo-silencioso de tr├ís para frente
	lastSpeechEnd := numSamples
	for pos := numSamples - frameSize; pos >= 0; pos -= frameSize {
		startByte := pos * 2
		endByte := startByte + frameSizeBytes
		if endByte > len(pcm) {
			endByte = len(pcm)
		}
		rms := computeRMS(pcm[startByte:endByte])
		if rms >= silenceRMS {
			// Preserva 100ms ap├│s o ├║ltimo frame com fala
			trailSamples := sampleRate / 10
			lastSpeechEnd = pos + frameSize + trailSamples
			if lastSpeechEnd > numSamples {
				lastSpeechEnd = numSamples
			}
			break
		}
	}

	truncBytes := lastSpeechEnd * 2
	if truncBytes > len(pcm) {
		truncBytes = len(pcm)
	}
	return pcm[:truncBytes]
}

// encodePCMToWAV cria um arquivo WAV a partir de dados PCM brutos.
func encodePCMToWAV(pcm []byte, sampleRate uint32, bitsPerSample uint16, numChannels uint16) []byte {
	dataSize := uint32(len(pcm))
	byteRate := sampleRate * uint32(numChannels) * uint32(bitsPerSample) / 8
	blockAlign := numChannels * bitsPerSample / 8

	var buf bytes.Buffer
	buf.Grow(44 + len(pcm))

	// RIFF header
	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(36+dataSize)) // ChunkSize
	buf.WriteString("WAVE")

	// fmt chunk
	buf.WriteString("fmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(16))         // SubchunkSize
	binary.Write(&buf, binary.LittleEndian, uint16(1))          // AudioFormat (PCM)
	binary.Write(&buf, binary.LittleEndian, numChannels)        // NumChannels
	binary.Write(&buf, binary.LittleEndian, sampleRate)         // SampleRate
	binary.Write(&buf, binary.LittleEndian, byteRate)           // ByteRate
	binary.Write(&buf, binary.LittleEndian, blockAlign)         // BlockAlign
	binary.Write(&buf, binary.LittleEndian, bitsPerSample)      // BitsPerSample

	// data chunk
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, dataSize)
	buf.Write(pcm)

	return buf.Bytes()
}