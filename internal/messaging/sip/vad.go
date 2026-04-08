package sip

import (
	"encoding/binary"
	"log"
	"math"
	"sort"
	"time"
)

// VADState indica se o VAD detecta fala ou sil├¬ncio.
type VADState int

const (
	VADSilence VADState = iota
	VADSpeech
)

// VADConfig configura o detector de atividade de voz baseado em energia RMS.
type VADConfig struct {
	// SilenceThreshold ├® o n├¡vel RMS abaixo do qual ├® considerado sil├¬ncio (0.0ÔÇô1.0).
	// Padr├úo: 0.02
	SilenceThreshold float64

	// SpeechThreshold ├® o n├¡vel RMS acima do qual ├® considerado fala (0.0ÔÇô1.0).
	// Padr├úo: 0.03
	SpeechThreshold float64

	// SilenceDuration ├® o tempo cont├¡nuo de sil├¬ncio para finalizar um segmento de fala.
	// Padr├úo: 800ms
	SilenceDuration time.Duration

	// SpeechDuration ├® o tempo m├¡nimo de fala para considerar um segmento v├ílido.
	// Padr├úo: 250ms
	SpeechDuration time.Duration

	// LeadingBufferDuration ├® a quantidade de ├íudio antes do onset de fala capturada
	// para evitar cortar o in├¡cio de s├¡labas.
	// Padr├úo: 200ms
	LeadingBufferDuration time.Duration

	// SampleRate ├® a taxa de amostragem do ├íudio de entrada.
	// Padr├úo: 8000 (G.711)
	SampleRate int

	// AdaptiveNoise habilita threshold adaptativo baseado no n├¡vel de ru├¡do de fundo.
	// Quando habilitado, SpeechThreshold e SilenceThreshold ajustam automaticamente.
	// Padr├úo: true
	AdaptiveNoise bool

	// AdaptiveAlpha ├® a taxa de aprendizado do filtro EMA para estimar ru├¡do de fundo.
	// Valores menores = adapta├º├úo mais lenta (mais est├ível). Range: 0.001ÔÇô0.1
	// Padr├úo: 0.01
	AdaptiveAlpha float64

	// AdaptiveMargin ├® o multiplicador acima do n├¡vel de ru├¡do estimado para
	// definir o SpeechThreshold din├ómico. Ex: 2.5 = threshold = noiseFloor * 2.5
	// Padr├úo: 2.5
	AdaptiveMargin float64

	// ZCRWeight define o peso do ZCR (Zero Crossing Rate) na decis├úo de voz.
	// 0.0 = ignora ZCR (s├│ RMS), 1.0 = peso m├íximo. Fala tem ZCR m├®dio (50-200 Hz 8kHz).
	// Ru├¡do branco tem ZCR alto (>0.3). Cliques/tons puros t├¬m ZCR baixo (<0.05).
	// Padr├úo: 0.3
	ZCRWeight float64

	// MaxSegmentDuration ├® a dura├º├úo m├íxima de um segmento de fala antes de for├ºar
	// emiss├úo para o STT. Previne mega-segmentos causados por eco ou ru├¡do cont├¡nuo.
	// Padr├úo: 15s
	MaxSegmentDuration time.Duration
}

// DefaultVADConfig retorna configura├º├úo padr├úo para canal telef├┤nico G.711.
func DefaultVADConfig() VADConfig {
	return VADConfig{
		SilenceThreshold:      0.02,
		SpeechThreshold:       0.03,
		SilenceDuration:       800 * time.Millisecond,
		SpeechDuration:        250 * time.Millisecond,
		LeadingBufferDuration: 200 * time.Millisecond,
		SampleRate:            8000,
		AdaptiveNoise:         true,
		AdaptiveAlpha:         0.01,
		AdaptiveMargin:        2.5,
		ZCRWeight:             0.3,
		MaxSegmentDuration:    15 * time.Second,
	}
}

// EnergyVAD ├® um detector de atividade de voz baseado em energia RMS com
// Zero Crossing Rate (ZCR) e threshold adaptativo ao ru├¡do de fundo.
// Processa ├íudio PCM 16-bit signed little-endian mono.
// Usa contagem de samples para medir dura├º├Áes (n├úo depende de time.Now).
type EnergyVAD struct {
	config VADConfig
	state  VADState

	// Contadores de samples para medir dura├º├Áes
	speechSamples  int // samples consecutivos acima do threshold
	silenceSamples int // samples consecutivos abaixo do threshold

	// Thresholds em n├║mero de samples
	speechSamplesThreshold  int
	silenceSamplesThreshold int
	maxSegmentBytes         int // m├íximo de bytes no speech buffer antes de force-emit

	// Threshold adaptativo
	noiseFloor      float64   // n├¡vel RMS estimado do ru├¡do de fundo (EMA)
	noiseInited     bool      // se o noiseFloor j├í convergiu (primeiros ~500ms)
	noiseInitBuf    []float64
	noiseInitSize   int // samples para inicializa├º├úo (~500ms)
	noiseInitFrames int // total de frames processados durante inicializa├º├úo

	// Buffer circular de leading audio (pr├®-onset)
	leadingBuf     []byte
	leadingBufSize int // tamanho m├íximo em bytes
	leadingBufPos  int // posi├º├úo de escrita no buffer circular

	// Acumula ├íudio do segmento de fala atual
	speechBuf []byte

	// Callbacks
	onSpeechStart func()
	onSpeechEnd   func(segment []byte)
}

// NewEnergyVAD cria um novo detector VAD com a configura├º├úo fornecida.
func NewEnergyVAD(cfg VADConfig, onSpeechStart func(), onSpeechEnd func(segment []byte)) *EnergyVAD {
	if cfg.SampleRate == 0 {
		cfg.SampleRate = 8000
	}
	if cfg.SilenceThreshold == 0 {
		cfg.SilenceThreshold = 0.02
	}
	if cfg.SpeechThreshold == 0 {
		cfg.SpeechThreshold = 0.03
	}
	if cfg.SilenceDuration == 0 {
		cfg.SilenceDuration = 800 * time.Millisecond
	}
	if cfg.SpeechDuration == 0 {
		cfg.SpeechDuration = 250 * time.Millisecond
	}
	if cfg.LeadingBufferDuration == 0 {
		cfg.LeadingBufferDuration = 200 * time.Millisecond
	}
	if cfg.AdaptiveAlpha == 0 {
		cfg.AdaptiveAlpha = 0.01
	}
	if cfg.AdaptiveMargin == 0 {
		cfg.AdaptiveMargin = 2.5
	}
	if cfg.MaxSegmentDuration == 0 {
		cfg.MaxSegmentDuration = 15 * time.Second
	}

	// Converte dura├º├Áes em n├║mero de samples
	speechSamplesThreshold := int(cfg.SpeechDuration.Seconds() * float64(cfg.SampleRate))
	silenceSamplesThreshold := int(cfg.SilenceDuration.Seconds() * float64(cfg.SampleRate))

	// Leading buffer: samples * 2 bytes por sample
	leadingSamples := int(cfg.LeadingBufferDuration.Seconds() * float64(cfg.SampleRate))
	leadingBufSize := leadingSamples * 2 // 16-bit = 2 bytes per sample

	// Inicializa├º├úo do noise floor: coleta ~500ms de ├íudio antes de usar adaptive.
	// Cada ProcessFrame tipicamente recebe 20ms de ├íudio, ent├úo 500ms Ôëê 25 frames.
	noiseInitSize := 25

	// Max segment: dura├º├úo em bytes (samples * 2 bytes por sample)
	maxSegmentBytes := int(cfg.MaxSegmentDuration.Seconds() * float64(cfg.SampleRate)) * 2

	return &EnergyVAD{
		config:                  cfg,
		state:                   VADSilence,
		speechSamplesThreshold:  speechSamplesThreshold,
		silenceSamplesThreshold: silenceSamplesThreshold,
		maxSegmentBytes:         maxSegmentBytes,
		noiseFloor:              cfg.SilenceThreshold,
		noiseInitSize:           noiseInitSize,
		leadingBuf:              make([]byte, leadingBufSize),
		leadingBufSize:          leadingBufSize,
		onSpeechStart:           onSpeechStart,
		onSpeechEnd:             onSpeechEnd,
	}
}

// ProcessFrame processa um frame de ├íudio PCM 16-bit signed LE mono.
// Combina RMS energy com ZCR para decis├úo mais robusta. Usa threshold
// adaptativo ao ru├¡do de fundo quando AdaptiveNoise est├í habilitado.
func (v *EnergyVAD) ProcessFrame(pcm []byte) VADState {
	rms := computeRMS(pcm)
	frameSamples := len(pcm) / 2

	// Atualiza noise floor adaptativo
	v.updateNoiseFloor(rms)

	// Calcula score combinado RMS + ZCR
	score := v.computeScore(pcm, rms)

	// Resolve thresholds (adaptativos ou fixos)
	speechTh, silenceTh := v.resolveThresholds()

	switch v.state {
	case VADSilence:
		v.processInSilence(pcm, score, speechTh, frameSamples)
	case VADSpeech:
		v.processInSpeech(pcm, score, silenceTh, frameSamples)
	}

	return v.state
}

// State retorna o estado atual do VAD.
func (v *EnergyVAD) State() VADState {
	return v.state
}

// Reset volta o VAD ao estado inicial.
func (v *EnergyVAD) Reset() {
	v.state = VADSilence
	v.speechSamples = 0
	v.silenceSamples = 0
	v.speechBuf = nil
	v.leadingBufPos = 0
	v.noiseFloor = v.config.SilenceThreshold
	v.noiseInited = false
	v.noiseInitBuf = nil
	for i := range v.leadingBuf {
		v.leadingBuf[i] = 0
	}
}

// Flush for├ºa a emiss├úo do segmento de fala atual (se houver).
// ├Ütil para encerramento de chamada.
func (v *EnergyVAD) Flush() {
	if v.state == VADSpeech && len(v.speechBuf) > 0 {
		if v.onSpeechEnd != nil {
			v.onSpeechEnd(v.speechBuf)
		}
		v.speechBuf = nil
		v.state = VADSilence
		v.speechSamples = 0
		v.silenceSamples = 0
	}
}

func (v *EnergyVAD) processInSilence(pcm []byte, score float64, speechThreshold float64, frameSamples int) {
	// Mant├®m leading buffer atualizado (inclui frames pr├®-onset)
	v.appendLeadingBuffer(pcm)

	if score >= speechThreshold {
		v.speechSamples += frameSamples
		v.silenceSamples = 0

		// Verifica se atingiu dura├º├úo m├¡nima de speech (em samples)
		if v.speechSamples >= v.speechSamplesThreshold {
			v.state = VADSpeech
			v.silenceSamples = 0

			log.Printf("[VAD] Onset ÔåÆ Speech (score=%.4f th=%.4f noiseFloor=%.4f speechMs=%d)",
				score, speechThreshold, v.noiseFloor, v.speechSamples*1000/v.config.SampleRate)

			// Inicia speech buffer com leading audio (inclui frames pr├®-onset)
			v.speechBuf = make([]byte, 0, v.leadingBufSize+len(pcm))
			v.speechBuf = append(v.speechBuf, v.drainLeadingBuffer()...)
			v.speechBuf = append(v.speechBuf, pcm...)

			if v.onSpeechStart != nil {
				v.onSpeechStart()
			}
		}
	} else {
		v.silenceSamples += frameSamples
		// Tolera at├® 2 frames de sil├¬ncio (40ms) dentro de um onset de fala.
		// Codec G.711 e jitter RTP podem causar frames intermitentes silenciosos.
		// S├│ reseta se o sil├¬ncio durar mais que a toler├óncia.
		hangoverSamples := v.config.SampleRate / 25 // ~40ms
		if v.silenceSamples > hangoverSamples {
			v.speechSamples = 0
			v.silenceSamples = 0
		}
	}
}

func (v *EnergyVAD) processInSpeech(pcm []byte, score float64, silenceThreshold float64, frameSamples int) {
	// Acumula ├íudio
	v.speechBuf = append(v.speechBuf, pcm...)

	// Force-emit: se o segmento excedeu a dura├º├úo m├íxima, emite para n├úo
	// acumular mega-segmentos (causados por eco do TTS, ru├¡do cont├¡nuo, etc).
	// O STT (Whisper) funciona melhor com segmentos de 5-15s.
	if v.maxSegmentBytes > 0 && len(v.speechBuf) >= v.maxSegmentBytes {
		log.Printf("[VAD] MaxSegment atingido (%.1fs) ÔÇö force-emit",
			v.config.MaxSegmentDuration.Seconds())
		if v.onSpeechEnd != nil && len(v.speechBuf) > 0 {
			v.onSpeechEnd(v.speechBuf)
		}
		v.speechBuf = nil
		v.state = VADSilence
		v.speechSamples = 0
		v.silenceSamples = 0
		v.leadingBufPos = 0
		return
	}

	if score < silenceThreshold {
		v.silenceSamples += frameSamples

		if v.silenceSamples >= v.silenceSamplesThreshold {
			// Sil├¬ncio confirmado: emite segmento
			if v.onSpeechEnd != nil && len(v.speechBuf) > 0 {
				v.onSpeechEnd(v.speechBuf)
			}
			v.speechBuf = nil
			v.state = VADSilence
			v.speechSamples = 0
			v.silenceSamples = 0
			v.leadingBufPos = 0
		}
	} else {
		// Fala detectada ÔÇö reseta timer de sil├¬ncio
		v.silenceSamples = 0
	}
}

// updateNoiseFloor atualiza a estimativa do ru├¡do de fundo usando EMA.
// S├│ atualiza durante sil├¬ncio (quando n├úo h├í fala ativa).
func (v *EnergyVAD) updateNoiseFloor(rms float64) {
	if !v.config.AdaptiveNoise {
		return
	}

	// Fase de inicializa├º├úo: coleta frames para estimar o noise floor inicial.
	// Filtra frames com RMS alto (prov├ível fala ou ru├¡do impulsivo) para n├úo
	// inflar o noise floor. Usa percentil 25 para ser conservador.
	if !v.noiseInited {
		// S├│ coleta frames com RMS baixo (< 0.15) para evitar incluir fala
		if rms < 0.15 {
			v.noiseInitBuf = append(v.noiseInitBuf, rms)
		}
		// Tamb├®m conta total de frames processados para n├úo esperar infinitamente
		v.noiseInitFrames++
		if len(v.noiseInitBuf) >= v.noiseInitSize || v.noiseInitFrames >= v.noiseInitSize*3 {
			if len(v.noiseInitBuf) > 0 {
				v.noiseFloor = medianFloat64(v.noiseInitBuf)
			}
			v.noiseInited = true
			v.noiseInitBuf = nil
		}
		return
	}

	// S├│ atualiza noise floor quando est├í em sil├¬ncio
	if v.state == VADSilence {
		alpha := v.config.AdaptiveAlpha
		v.noiseFloor = alpha*rms + (1-alpha)*v.noiseFloor
	}
}

// resolveThresholds retorna os thresholds de speech e silence efetivos.
// Se AdaptiveNoise est├í habilitado e o noise floor j├í convergiu, usa
// thresholds din├ómicos baseados no n├¡vel de ru├¡do estimado.
func (v *EnergyVAD) resolveThresholds() (speechTh, silenceTh float64) {
	if !v.config.AdaptiveNoise || !v.noiseInited {
		return v.config.SpeechThreshold, v.config.SilenceThreshold
	}

	// Speech threshold = noise floor * margin
	speechTh = v.noiseFloor * v.config.AdaptiveMargin
	// Silence threshold = metade do speech threshold
	silenceTh = speechTh * 0.6

	// Clamp: nunca abaixo dos m├¡nimos configurados
	if speechTh < v.config.SpeechThreshold {
		speechTh = v.config.SpeechThreshold
	}
	if silenceTh < v.config.SilenceThreshold {
		silenceTh = v.config.SilenceThreshold
	}

	// Upper bound: thresholds absurdamente altos tornam detec├º├úo imposs├¡vel
	const maxSpeechTh = 0.5
	const maxSilenceTh = 0.3
	if speechTh > maxSpeechTh {
		speechTh = maxSpeechTh
	}
	if silenceTh > maxSilenceTh {
		silenceTh = maxSilenceTh
	}

	return speechTh, silenceTh
}

// computeScore combina RMS energy com ZCR para gerar um score de atividade vocal.
// Fala humana tem RMS m├®dio e ZCR na faixa 0.05ÔÇô0.20 (8kHz).
// Ru├¡do branco tem ZCR alto (>0.3). Cliques t├¬m ZCR muito baixo.
func (v *EnergyVAD) computeScore(pcm []byte, rms float64) float64 {
	if v.config.ZCRWeight <= 0 {
		return rms // Sem ZCR, usa RMS puro
	}

	zcr := computeZCR(pcm)

	// ZCR de fala est├í tipicamente entre 0.05 e 0.20 para 8kHz.
	// Penaliza ZCR muito alto (ru├¡do) ou muito baixo (tom puro/clique).
	var zcrBonus float64
	if zcr >= 0.04 && zcr <= 0.25 {
		// ZCR na faixa de fala ÔåÆ b├┤nus
		zcrBonus = 1.0
	} else if zcr > 0.25 && zcr <= 0.40 {
		// ZCR moderadamente alto ÔåÆ b├┤nus parcial (pode ser fricativa)
		zcrBonus = 0.5
	} else {
		// ZCR fora da faixa ÔåÆ sem b├┤nus (provavelmente n├úo ├® fala)
		zcrBonus = 0.0
	}

	// Score = RMS * (1 + ZCRWeight * zcrBonus)
	// Se zcrBonus=1 e weight=0.3: score = RMS * 1.3 (boost para fala)
	// Se zcrBonus=0 e weight=0.3: score = RMS * 1.0 (sem boost)
	return rms * (1.0 + v.config.ZCRWeight*zcrBonus)
}

// NoiseFloor retorna o n├¡vel estimado de ru├¡do de fundo atual.
func (v *EnergyVAD) NoiseFloor() float64 {
	return v.noiseFloor
}

func (v *EnergyVAD) appendLeadingBuffer(pcm []byte) {
	if v.leadingBufSize == 0 {
		return
	}

	for _, b := range pcm {
		v.leadingBuf[v.leadingBufPos%v.leadingBufSize] = b
		v.leadingBufPos++
	}
}

func (v *EnergyVAD) drainLeadingBuffer() []byte {
	if v.leadingBufSize == 0 || v.leadingBufPos == 0 {
		return nil
	}

	if v.leadingBufPos <= v.leadingBufSize {
		// Buffer n├úo completou um ciclo, retorna tudo
		result := make([]byte, v.leadingBufPos)
		copy(result, v.leadingBuf[:v.leadingBufPos])
		return result
	}

	// Buffer circular cheio ÔÇö reordena
	start := v.leadingBufPos % v.leadingBufSize
	result := make([]byte, v.leadingBufSize)
	copy(result, v.leadingBuf[start:])
	copy(result[v.leadingBufSize-start:], v.leadingBuf[:start])
	return result
}

// computeZCR calcula o Zero Crossing Rate de um buffer PCM 16-bit signed LE.
// Retorna a fra├º├úo de transi├º├Áes de sinal (0.0ÔÇô1.0).
// Fala humana tipicamente 0.05ÔÇô0.20 (8kHz). Ru├¡do branco >0.3.
func computeZCR(pcm []byte) float64 {
	numSamples := len(pcm) / 2
	if numSamples < 2 {
		return 0
	}

	var crossings int
	prevSample := int16(binary.LittleEndian.Uint16(pcm[0:2]))

	for i := 1; i < numSamples; i++ {
		sample := int16(binary.LittleEndian.Uint16(pcm[i*2 : i*2+2]))
		if (prevSample >= 0 && sample < 0) || (prevSample < 0 && sample >= 0) {
			crossings++
		}
		prevSample = sample
	}

	return float64(crossings) / float64(numSamples-1)
}

// medianFloat64 retorna a mediana de um slice de float64.
// Cria uma c├│pia para n├úo alterar o slice original.
func medianFloat64(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)

	n := len(sorted)
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return sorted[n/2]
}

// computeRMS calcula o RMS (Root Mean Square) normalizado de um buffer PCM 16-bit signed LE.
// Retorna um valor entre 0.0 e 1.0.
func computeRMS(pcm []byte) float64 {
	if len(pcm) < 2 {
		return 0
	}

	numSamples := len(pcm) / 2
	var sumSquares float64

	for i := 0; i < numSamples; i++ {
		sample := int16(binary.LittleEndian.Uint16(pcm[i*2 : i*2+2]))
		normalized := float64(sample) / 32768.0
		sumSquares += normalized * normalized
	}

	return math.Sqrt(sumSquares / float64(numSamples))
}