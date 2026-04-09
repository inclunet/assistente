package sip

import (
	"encoding/binary"
	"math"
	"testing"
	"time"
)

// generateSilence cria um buffer PCM 16-bit com sil├¬ncio (zeros).
func generateSilence(durationMS int, sampleRate int) []byte {
	numSamples := (durationMS * sampleRate) / 1000
	return make([]byte, numSamples*2)
}

// generateTone cria um buffer PCM 16-bit com um tom senoidal.
func generateTone(durationMS int, sampleRate int, freq float64, amplitude float64) []byte {
	numSamples := (durationMS * sampleRate) / 1000
	buf := make([]byte, numSamples*2)

	for i := 0; i < numSamples; i++ {
		t := float64(i) / float64(sampleRate)
		val := amplitude * math.Sin(2*math.Pi*freq*t)
		sample := int16(val * 32767)
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(sample))
	}

	return buf
}

func TestEnergyVAD_SilenceDetection(t *testing.T) {
	var speechStarted bool
	var speechEnded bool

	cfg := DefaultVADConfig()
	cfg.SampleRate = 8000

	vad := NewEnergyVAD(cfg,
		func() { speechStarted = true },
		func(segment []byte) { speechEnded = true },
	)

	// Alimenta 1 segundo de sil├¬ncio
	silence := generateSilence(20, 8000) // 20ms frames
	for i := 0; i < 50; i++ {            // 50 * 20ms = 1s
		vad.ProcessFrame(silence)
	}

	if speechStarted {
		t.Error("VAD detectou fala em sil├¬ncio")
	}
	if speechEnded {
		t.Error("VAD emitiu segmento de fala em sil├¬ncio")
	}
	if vad.State() != VADSilence {
		t.Errorf("Estado esperado VADSilence, obteve %v", vad.State())
	}
}

func TestEnergyVAD_WarmupSuppressesOnset(t *testing.T) {
	var speechStarted bool

	cfg := DefaultVADConfig()
	cfg.SampleRate = 8000
	cfg.SpeechDuration = 100 * time.Millisecond
	cfg.WarmupDuration = 500 * time.Millisecond // 500ms warm-up

	vad := NewEnergyVAD(cfg,
		func() { speechStarted = true },
		func(segment []byte) {},
	)

	// Alimenta tom alto durante o warm-up (250ms < 500ms warm-up)
	tone := generateTone(20, 8000, 440.0, 0.5)
	for i := 0; i < 12; i++ { // 12 * 20ms = 240ms
		vad.ProcessFrame(tone)
	}

	if speechStarted {
		t.Error("VAD detectou fala durante warm-up")
	}
	if vad.State() != VADSilence {
		t.Errorf("Estado esperado VADSilence durante warm-up, obteve %v", vad.State())
	}

	// Continua até completar o warm-up (mais 260ms para totalizar 500ms)
	silence := generateSilence(20, 8000)
	for i := 0; i < 13; i++ { // 13 * 20ms = 260ms
		vad.ProcessFrame(silence)
	}

	// Agora alimenta tom alto após warm-up — deve detectar fala
	for i := 0; i < 10; i++ { // 10 * 20ms = 200ms > SpeechDuration 100ms
		vad.ProcessFrame(tone)
	}

	if !speechStarted {
		t.Error("VAD não detectou fala após warm-up completo")
	}
}

func TestEnergyVAD_SpeechDetection(t *testing.T) {
	var speechStarted bool
	var speechSegment []byte

	cfg := DefaultVADConfig()
	cfg.SampleRate = 8000
	cfg.SpeechDuration = 100 * time.Millisecond
	cfg.SilenceDuration = 200 * time.Millisecond
	cfg.WarmupDuration = 0 // desabilitar warm-up para teste direto

	vad := NewEnergyVAD(cfg,
		func() { speechStarted = true },
		func(segment []byte) { speechSegment = segment },
	)

	// Alimenta tom alto (simula fala) por 500ms
	tone := generateTone(20, 8000, 440.0, 0.5) // 20ms frames @ 440Hz
	for i := 0; i < 25; i++ {                   // 25 * 20ms = 500ms
		vad.ProcessFrame(tone)
	}

	if !speechStarted {
		t.Error("VAD n├úo detectou in├¡cio de fala")
	}
	if vad.State() != VADSpeech {
		t.Errorf("Estado esperado VADSpeech, obteve %v", vad.State())
	}

	// Alimenta sil├¬ncio para finalizar segmento
	silence := generateSilence(20, 8000)
	for i := 0; i < 50; i++ { // 50 * 20ms = 1s (> SilenceDuration)
		vad.ProcessFrame(silence)
	}

	if speechSegment == nil {
		t.Error("VAD n├úo emitiu segmento de fala ap├│s sil├¬ncio")
	}
	if len(speechSegment) == 0 {
		t.Error("Segmento de fala vazio")
	}
}

func TestEnergyVAD_Flush(t *testing.T) {
	var flushedSegment []byte

	cfg := DefaultVADConfig()
	cfg.SampleRate = 8000
	cfg.SpeechDuration = 50 * time.Millisecond
	cfg.WarmupDuration = 0

	vad := NewEnergyVAD(cfg,
		func() {},
		func(segment []byte) { flushedSegment = segment },
	)

	// Coloca em estado de fala
	tone := generateTone(20, 8000, 440.0, 0.5)
	for i := 0; i < 10; i++ {
		vad.ProcessFrame(tone)
	}

	// Flush sem sil├¬ncio pr├®vio
	vad.Flush()

	if flushedSegment == nil {
		t.Error("Flush n├úo emitiu segmento pendente")
	}
	if vad.State() != VADSilence {
		t.Errorf("Estado ap├│s flush esperado VADSilence, obteve %v", vad.State())
	}
}

func TestEnergyVAD_Reset(t *testing.T) {
	cfg := DefaultVADConfig()
	cfg.SampleRate = 8000
	cfg.SpeechDuration = 50 * time.Millisecond

	vad := NewEnergyVAD(cfg, func() {}, func([]byte) {})

	// Coloca em estado de fala
	tone := generateTone(20, 8000, 440.0, 0.5)
	for i := 0; i < 10; i++ {
		vad.ProcessFrame(tone)
	}

	vad.Reset()

	if vad.State() != VADSilence {
		t.Errorf("Estado ap├│s reset esperado VADSilence, obteve %v", vad.State())
	}
}

func TestComputeRMS(t *testing.T) {
	tests := []struct {
		name    string
		pcm     []byte
		wantMin float64
		wantMax float64
	}{
		{
			name:    "sil├¬ncio retorna 0",
			pcm:     make([]byte, 320),
			wantMin: 0,
			wantMax: 0.001,
		},
		{
			name:    "tom alto retorna > 0.3",
			pcm:     generateTone(20, 8000, 440, 0.5),
			wantMin: 0.3,
			wantMax: 0.5,
		},
		{
			name:    "tom baixo retorna < 0.05",
			pcm:     generateTone(20, 8000, 440, 0.01),
			wantMin: 0.005,
			wantMax: 0.015,
		},
		{
			name:    "buffer vazio retorna 0",
			pcm:     nil,
			wantMin: 0,
			wantMax: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rms := computeRMS(tt.pcm)
			if rms < tt.wantMin || rms > tt.wantMax {
				t.Errorf("computeRMS() = %f, want [%f, %f]", rms, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestEnergyVAD_LeadingBuffer(t *testing.T) {
	var speechSegment []byte

	cfg := DefaultVADConfig()
	cfg.SampleRate = 8000
	cfg.LeadingBufferDuration = 100 * time.Millisecond
	cfg.SpeechDuration = 50 * time.Millisecond
	cfg.SilenceDuration = 200 * time.Millisecond
	cfg.WarmupDuration = 0

	vad := NewEnergyVAD(cfg,
		func() {},
		func(segment []byte) { speechSegment = segment },
	)

	// Alimenta sil├¬ncio (deve acumular no leading buffer)
	silence := generateSilence(20, 8000)
	for i := 0; i < 10; i++ {
		vad.ProcessFrame(silence)
	}

	// Inicia fala
	tone := generateTone(20, 8000, 440.0, 0.5)
	for i := 0; i < 10; i++ {
		vad.ProcessFrame(tone)
	}

	// Finaliza com sil├¬ncio
	for i := 0; i < 20; i++ {
		vad.ProcessFrame(silence)
	}

	if speechSegment == nil {
		t.Fatal("Segmento de fala n├úo emitido")
	}

	// O segmento deve incluir o leading buffer + a fala
	// Leading: 100ms @ 8kHz = 800 samples = 1600 bytes
	// Speech: ~200ms @ 8kHz = 1600 samples = 3200 bytes + sil├¬ncio p├│s-fala
	leadingBytes := (100 * 8000 / 1000) * 2
	if len(speechSegment) < leadingBytes {
		t.Errorf("Segmento (%d bytes) menor que o leading buffer esperado (%d bytes)",
			len(speechSegment), leadingBytes)
	}
}

func TestComputeZCR(t *testing.T) {
	tests := []struct {
		name    string
		pcm     []byte
		wantMin float64
		wantMax float64
	}{
		{
			name:    "sil├¬ncio tem ZCR 0",
			pcm:     generateSilence(20, 8000),
			wantMin: 0,
			wantMax: 0.001,
		},
		{
			name:    "tom 440Hz a 8kHz tem ZCR ~0.11",
			pcm:     generateTone(20, 8000, 440, 0.5),
			wantMin: 0.05,
			wantMax: 0.20,
		},
		{
			name:    "tom 2000Hz a 8kHz tem ZCR alto ~0.5",
			pcm:     generateTone(20, 8000, 2000, 0.5),
			wantMin: 0.35,
			wantMax: 0.65,
		},
		{
			name:    "buffer curto retorna 0",
			pcm:     make([]byte, 2), // 1 sample
			wantMin: 0,
			wantMax: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zcr := computeZCR(tt.pcm)
			if zcr < tt.wantMin || zcr > tt.wantMax {
				t.Errorf("computeZCR() = %f, want [%f, %f]", zcr, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestMedianFloat64(t *testing.T) {
	tests := []struct {
		name string
		vals []float64
		want float64
	}{
		{"vazio", nil, 0},
		{"um valor", []float64{5.0}, 5.0},
		{"├¡mpar", []float64{3, 1, 2}, 2.0},
		{"par", []float64{1, 2, 3, 4}, 2.5},
		{"j├í ordenado", []float64{1, 2, 3, 4, 5}, 3.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := medianFloat64(tt.vals)
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("medianFloat64(%v) = %f, want %f", tt.vals, got, tt.want)
			}
		})
	}
}

func TestComputeScore_ZCRWeightZero(t *testing.T) {
	cfg := DefaultVADConfig()
	cfg.ZCRWeight = 0
	vad := NewEnergyVAD(cfg, func() {}, func([]byte) {})

	tone := generateTone(20, 8000, 440, 0.5)
	rms := computeRMS(tone)
	score := vad.computeScore(tone, rms)

	// Sem ZCR, score deve ser igual ao RMS
	if math.Abs(score-rms) > 0.001 {
		t.Errorf("ZCRWeight=0: score=%f, want RMS=%f", score, rms)
	}
}

func TestComputeScore_ZCRBoostsSpeech(t *testing.T) {
	cfg := DefaultVADConfig()
	cfg.ZCRWeight = 0.3
	vad := NewEnergyVAD(cfg, func() {}, func([]byte) {})

	// Tom 440Hz ÔåÆ ZCR na faixa de fala ÔåÆ deve receber boost
	tone := generateTone(20, 8000, 440, 0.5)
	rms := computeRMS(tone)
	score := vad.computeScore(tone, rms)

	// Score deve ser > RMS (boost de ZCR)
	if score <= rms {
		t.Errorf("440Hz speech-like: score=%f deveria ser > RMS=%f", score, rms)
	}

	// Score esperado: RMS * (1 + 0.3*1.0) = RMS * 1.3
	expected := rms * 1.3
	if math.Abs(score-expected) > 0.01 {
		t.Errorf("Score=%f, expected ~%f (RMS*1.3)", score, expected)
	}
}

func TestEnergyVAD_AdaptiveNoiseFloor(t *testing.T) {
	cfg := DefaultVADConfig()
	cfg.SampleRate = 8000
	cfg.AdaptiveNoise = true
	cfg.AdaptiveAlpha = 0.05 // Converg├¬ncia mais r├ípida para o teste

	vad := NewEnergyVAD(cfg, func() {}, func([]byte) {})

	// Alimenta 30 frames de baixo ru├¡do (acima de sil├¬ncio absoluto)
	// para inicializar o noise floor (noiseInitSize=25 frames)
	lowNoise := generateTone(20, 8000, 100, 0.01)
	for i := 0; i < 30; i++ {
		vad.ProcessFrame(lowNoise)
	}

	// Ap├│s inicializa├º├úo, noise floor deve estar pr├│ximo do RMS do ru├¡do
	expectedFloor := computeRMS(lowNoise)
	floor := vad.NoiseFloor()

	if math.Abs(floor-expectedFloor) > 0.01 {
		t.Errorf("NoiseFloor=%f, expected ~%f (RMS do ru├¡do baixo)", floor, expectedFloor)
	}
}

func TestEnergyVAD_AdaptiveDisabled(t *testing.T) {
	cfg := DefaultVADConfig()
	cfg.SampleRate = 8000
	cfg.AdaptiveNoise = false

	vad := NewEnergyVAD(cfg, func() {}, func([]byte) {})

	// Alimenta frames de ru├¡do
	noise := generateTone(20, 8000, 100, 0.01)
	for i := 0; i < 30; i++ {
		vad.ProcessFrame(noise)
	}

	// Noise floor deve permanecer no valor inicial (SilenceThreshold)
	if vad.NoiseFloor() != cfg.SilenceThreshold {
		t.Errorf("NoiseFloor=%f com adaptive=false, expected %f", vad.NoiseFloor(), cfg.SilenceThreshold)
	}
}

func TestEnergyVAD_ResolveThresholds_Adaptive(t *testing.T) {
	cfg := DefaultVADConfig()
	cfg.SampleRate = 8000
	cfg.AdaptiveNoise = true
	cfg.AdaptiveMargin = 2.5

	vad := NewEnergyVAD(cfg, func() {}, func([]byte) {})

	// Inicializa noise floor com ru├¡do baixo
	lowNoise := generateTone(20, 8000, 100, 0.01)
	for i := 0; i < 30; i++ {
		vad.ProcessFrame(lowNoise)
	}

	speechTh, silenceTh := vad.resolveThresholds()

	// SpeechTh = max(noiseFloor * margin, minSpeechTh=0.003)
	expectedSpeechTh := vad.NoiseFloor() * cfg.AdaptiveMargin
	if expectedSpeechTh < 0.003 {
		expectedSpeechTh = 0.003
	}

	if math.Abs(speechTh-expectedSpeechTh) > 0.001 {
		t.Errorf("SpeechTh=%f, expected %f", speechTh, expectedSpeechTh)
	}

	// SilenceTh deve ser menor que SpeechTh
	if silenceTh >= speechTh {
		t.Errorf("SilenceTh=%f deveria ser < SpeechTh=%f", silenceTh, speechTh)
	}
}
func TestApplyGain(t *testing.T) {
	// Cria buffer com samples ±100
	pcm := make([]byte, 4) // 2 samples
	binary.LittleEndian.PutUint16(pcm[0:], uint16(int16(100)))
	neg := int16(-100)
	binary.LittleEndian.PutUint16(pcm[2:], uint16(neg))

	applyGain(pcm, 10.0)

	s0 := int16(binary.LittleEndian.Uint16(pcm[0:]))
	s1 := int16(binary.LittleEndian.Uint16(pcm[2:]))
	if s0 != 1000 {
		t.Errorf("sample[0] = %d, want 1000", s0)
	}
	if s1 != -1000 {
		t.Errorf("sample[1] = %d, want -1000", s1)
	}

	// Testa clipping
	binary.LittleEndian.PutUint16(pcm[0:], uint16(int16(10000)))
	applyGain(pcm[:2], 10.0)
	s0 = int16(binary.LittleEndian.Uint16(pcm[0:]))
	if s0 != 32767 {
		t.Errorf("clip: sample = %d, want 32767", s0)
	}
}

func TestEnergyVAD_MaxSegmentDuration(t *testing.T) {
	var emitted [][]byte

	cfg := DefaultVADConfig()
	cfg.MaxSegmentDuration = 500 * time.Millisecond // 500ms para teste r├ípido
	cfg.SpeechDuration = 60 * time.Millisecond      // onset r├ípido
	cfg.WarmupDuration = 0

	vad := NewEnergyVAD(cfg, nil, func(seg []byte) {
		cp := make([]byte, len(seg))
		copy(cp, seg)
		emitted = append(emitted, cp)
	})

	// Gera frame de fala alto (acima do threshold)
	loudFrame := generateTone(20, 8000, 440, 0.5)

	// Envia frames continuos de fala por 1.5s (3x o max de 500ms)
	framesNeeded := int(1500*time.Millisecond / (20 * time.Millisecond))
	for i := 0; i < framesNeeded; i++ {
		vad.ProcessFrame(loudFrame)
	}

	// Deve ter emitido pelo menos 2 segmentos (1500ms / 500ms = 3 potenciais)
	if len(emitted) < 2 {
		t.Errorf("Esperado ÔëÑ2 segmentos por max duration, obteve %d", len(emitted))
	}

	// Cada segmento n├úo deve exceder ~500ms de ├íudio (com toler├óncia de 1 frame)
	maxBytes := int(cfg.MaxSegmentDuration.Seconds()*float64(cfg.SampleRate))*2 + len(loudFrame)
	for i, seg := range emitted {
		if len(seg) > maxBytes {
			t.Errorf("Segmento %d: %d bytes excede max %d bytes", i, len(seg), maxBytes)
		}
	}
}