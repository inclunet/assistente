package sip

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"assistente/internal/messaging"
	"assistente/internal/speech"

	"github.com/emiago/diago"
	mp3 "github.com/hajimehoshi/go-mp3"
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
}

// DefaultAudioPipelineConfig retorna configura├º├úo padr├úo para pipeline SIP.
func DefaultAudioPipelineConfig() AudioPipelineConfig {
	return AudioPipelineConfig{
		VAD:             DefaultVADConfig(),
		ReadFrameSize:   320, // 20ms @ 8kHz mono 16-bit
		InputSampleRate: 8000,
		STTSampleRate:   16000,
		TTSOutputFormat:      "pcm",
		EchoCooldownDuration: 300 * time.Millisecond,
		STTMaxRetries:        2,
	}
}

// AudioPipeline gerencia o fluxo de ├íudio bidirecional de uma chamada SIP.
//
// Fluxo de entrada (call ÔåÆ LLM):
//
//	RTP ÔåÆ AudioReader ÔåÆ PCM 8kHz ÔåÆ VAD ÔåÆ Speech Segment ÔåÆ Resample 16kHz ÔåÆ WAV ÔåÆ Whisper STT ÔåÆ texto ÔåÆ Gateway
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

// Run inicia o loop de captura de ├íudio ÔåÆ VAD ÔåÆ STT.
// Bloqueia at├® a chamada terminar (contexto cancelado ou BYE).
func (p *AudioPipeline) Run() error {
	// Obt├®m reader de ├íudio (PCM decodificado do RTP/G.711)
	reader, err := p.dialog.AudioReader()
	if err != nil {
		return fmt.Errorf("sip pipeline: erro ao obter audio reader: %w", err)
	}

	// Configura VAD com callbacks
	p.vad = NewEnergyVAD(
		p.config.VAD,
		p.onSpeechStart,
		p.onSpeechEnd,
	)

	// Frame buffer para leitura
	frameSize := p.config.ReadFrameSize
	if frameSize == 0 {
		frameSize = 320
	}
	buf := make([]byte, frameSize)

	// Cooldown ap├│s playback: tempo para eco do softphone remoto dissipar.
	const echoCooldown = 600 * time.Millisecond

	log.Printf("[SIP Pipeline] Iniciado para chamada %s", p.call.ID)

	for {
		select {
		case <-p.ctx.Done():
			// Chamada encerrada ÔÇö flush VAD para emitir segmento final
			p.vad.Flush()
			log.Printf("[SIP Pipeline] Encerrado para chamada %s", p.call.ID)
			return nil
		default:
		}

		n, err := reader.Read(buf)
		if err != nil {
			if err == io.EOF || p.ctx.Err() != nil {
				p.vad.Flush()
				return nil
			}
			return fmt.Errorf("sip pipeline: erro ao ler ├íudio: %w", err)
		}

		if n > 0 {
			// Supress├úo de eco: ignora frames do VAD durante playback e cooldown.
			// O RTP continua sendo lido (drenado) para n├úo acumular no buffer,
			// mas o VAD n├úo processa ÔåÆ evita false onset do eco do TTS.
			if p.playbackActive.Load() {
				continue
			}
			if endNano := p.playbackEndedAt.Load(); endNano > 0 {
				if time.Since(time.Unix(0, endNano)) < echoCooldown {
					continue
				}
			}
			p.vad.ProcessFrame(buf[:n])
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
	p.playbackEndedAt.Store(time.Now().UnixNano())
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
	p.playbackEndedAt.Store(time.Now().UnixNano())
	return completed, err
}

// SpeakText sintetiza texto via streaming TTS e reproduz progressivamente.
// Usa SpeechManager.SynthesizeStream para receber chunks de MP3 ├á medida que
// s├úo gerados, decodifica e reproduz em tempo real (menor lat├¬ncia que batch).
// Suporta barge-in: quando StopPlayback() ├® chamado (pelo VAD onSpeechStart),
// a playback para, o stream TTS HTTP ├® cancelado via contexto, e a goroutine
// de streaming ├® finalizada pela closure do pipe.
func (p *AudioPipeline) SpeakText(ctx context.Context, text, voiceID string) (bool, error) {
	if p.speechManager == nil {
		return false, fmt.Errorf("sip pipeline: speech manager indispon├¡vel")
	}

	// Context cancel├ível ÔÇö cancelado quando playback termina ou ├® interrompido (barge-in).
	// Isso cancela a requisi├º├úo HTTP do TTS stream, liberando recursos.
	streamCtx, streamCancel := context.WithCancel(ctx)
	defer streamCancel()

	// Pipe: goroutine TTS envia MP3 bytes ÔåÆ lado leitor decodifica
	pr, pw := io.Pipe()

	// Goroutine: streaming TTS ÔåÆ base64 decode ÔåÆ escrita no pipe
	go func() {
		defer pw.Close()
		err := p.speechManager.SynthesizeStream(streamCtx, text, voiceID, speech.StreamCallbacks{
			OnChunk: func(chunkBase64 string) {
				data, decErr := base64.StdEncoding.DecodeString(chunkBase64)
				if decErr == nil && len(data) > 0 {
					if _, writeErr := pw.Write(data); writeErr != nil {
						return // pipe fechado (barge-in ou EOF)
					}
				}
			},
			OnDone:  func() {},
			OnError: func(err error) { pw.CloseWithError(err) },
		})
		if err != nil {
			pw.CloseWithError(err)
		}
	}()

	// Decodifica MP3 streaming ÔåÆ PCM stereo
	decoder, err := mp3.NewDecoder(pr)
	if err != nil {
		pr.Close()
		return false, fmt.Errorf("sip pipeline: erro ao iniciar decoder MP3 stream: %w", err)
	}

	srcRate := decoder.SampleRate()
	log.Printf("[SIP Pipeline] MP3 stream: srcRate=%d Hz", srcRate)

	// Reader adapter: stereo MP3 PCM ÔåÆ mono 8kHz
	resampler := &mp3StreamToMono8kReader{
		dec:     decoder,
		srcRate: srcRate,
		tmp:     make([]byte, 8192),
	}

	completed, playErr := p.PlayStreamingAudio(resampler)

	// Fecha pipe reader para desbloquear a goroutine de streaming
	pr.Close()

	return completed, playErr
}

// mp3StreamToMono8kReader adapta a sa├¡da do decoder MP3 (stereo PCM, sample rate
// original) para mono PCM 8kHz ÔÇö formato esperado pelo codec G.711 do RTP.
// Implementa io.Reader para encadear com o playback do diago.
type mp3StreamToMono8kReader struct {
	dec     *mp3.Decoder
	srcRate int
	buf     bytes.Buffer
	tmp     []byte // buffer tempor├írio para leitura do decoder
}

func (r *mp3StreamToMono8kReader) Read(p []byte) (int, error) {
	// Serve dados j├í processados
	if r.buf.Len() >= len(p) {
		return r.buf.Read(p)
	}

	// L├¬ chunk do decoder MP3 (stereo interleaved 16-bit signed LE)
	n, readErr := r.dec.Read(r.tmp)
	if n > 0 {
		// Converte stereo ÔåÆ mono, depois resampla para 8kHz
		mono := stereoToMono(r.tmp[:n])
		resampled := resampleGenericTo8k(mono, r.srcRate)
		r.buf.Write(resampled)
	}

	if r.buf.Len() > 0 {
		nn, _ := r.buf.Read(p)
		if readErr == io.EOF && r.buf.Len() == 0 {
			return nn, io.EOF
		}
		return nn, nil
	}

	return 0, readErr
}

// onSpeechStart ├® chamado quando o VAD detecta in├¡cio de fala.
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