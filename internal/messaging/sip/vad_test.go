package sip

import (
	"encoding/binary"
	"math"
	"testing"
	"time"
)

func generateSilence(durationMS int, sampleRate int) []byte {
	numSamples := (durationMS * sampleRate) / 1000
	return make([]byte, numSamples*2)
}

func generateSpeechLike(durationMS int, sampleRate int, amplitude float64) []byte {
	numSamples := (durationMS * sampleRate) / 1000
	buf := make([]byte, numSamples*2)

	for i := 0; i < numSamples; i++ {
		t := float64(i) / float64(sampleRate)
		envelope := 0.6 + 0.4*math.Sin(2*math.Pi*3*t)
		sample := envelope * amplitude * (0.55*math.Sin(2*math.Pi*140*t) +
			0.25*math.Sin(2*math.Pi*280*t) +
			0.12*math.Sin(2*math.Pi*700*t) +
			0.08*math.Sin(2*math.Pi*1220*t))
		if sample > 1 {
			sample = 1
		}
		if sample < -1 {
			sample = -1
		}
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(int16(sample*32767)))
	}

	return buf
}

func TestComputeRMS(t *testing.T) {
	tests := []struct {
		name    string
		pcm     []byte
		wantMin float64
		wantMax float64
	}{
		{
			name:    "silencio retorna zero",
			pcm:     make([]byte, 320),
			wantMin: 0,
			wantMax: 0.001,
		},
		{
			name:    "sinal de fala retorna energia",
			pcm:     generateSpeechLike(20, 8000, 0.7),
			wantMin: 0.15,
			wantMax: 0.60,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rms := computeRMS(tt.pcm)
			if rms < tt.wantMin || rms > tt.wantMax {
				t.Fatalf("computeRMS() = %f, want [%f, %f]", rms, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestSpeechDetectorSilenceDetection(t *testing.T) {
	var speechStarted bool
	var speechEnded bool

	cfg := DefaultVADConfig()
	cfg.SampleRate = 8000
	cfg.WarmupDuration = 0

	vad, err := NewSpeechDetector(cfg, func() { speechStarted = true }, func([]byte) { speechEnded = true })
	if err != nil {
		t.Fatalf("NewSpeechDetector() error = %v", err)
	}

	silence := generateSilence(20, 8000)
	for i := 0; i < 50; i++ {
		vad.ProcessFrame(silence)
	}

	if speechStarted {
		t.Fatal("VAD detectou fala em silencio")
	}
	if speechEnded {
		t.Fatal("VAD emitiu segmento em silencio")
	}
	if vad.State() != VADSilence {
		t.Fatalf("State() = %v, want %v", vad.State(), VADSilence)
	}
}

func TestSpeechDetectorWarmupSuppressesOnset(t *testing.T) {
	var speechStarted bool

	cfg := DefaultVADConfig()
	cfg.SampleRate = 8000
	cfg.Mode = 0
	cfg.SpeechDuration = 60 * time.Millisecond
	cfg.WarmupDuration = 400 * time.Millisecond

	vad, err := NewSpeechDetector(cfg, func() { speechStarted = true }, func([]byte) {})
	if err != nil {
		t.Fatalf("NewSpeechDetector() error = %v", err)
	}

	silence := generateSilence(20, 8000)
	speech := generateSpeechLike(20, 8000, 0.9)
	for i := 0; i < 10; i++ {
		vad.ProcessFrame(silence)
	}
	if speechStarted {
		t.Fatal("VAD detectou fala durante warm-up")
	}

	for i := 0; i < 10; i++ {
		vad.ProcessFrame(silence)
	}
	for i := 0; i < 10; i++ {
		vad.ProcessFrame(speech)
	}
	if !speechStarted {
		t.Fatal("VAD não detectou fala após warm-up")
	}
}

func TestSpeechDetectorSpeechSegment(t *testing.T) {
	var speechStarted bool
	var segment []byte

	cfg := DefaultVADConfig()
	cfg.SampleRate = 8000
	cfg.Mode = 0
	cfg.WarmupDuration = 0
	cfg.SpeechDuration = 60 * time.Millisecond
	cfg.SilenceDuration = 200 * time.Millisecond
	cfg.LeadingBufferDuration = 100 * time.Millisecond

	vad, err := NewSpeechDetector(cfg, func() { speechStarted = true }, func(seg []byte) {
		segment = append([]byte(nil), seg...)
	})
	if err != nil {
		t.Fatalf("NewSpeechDetector() error = %v", err)
	}

	silence := generateSilence(20, 8000)
	speech := generateSpeechLike(20, 8000, 0.9)

	for i := 0; i < 5; i++ {
		vad.ProcessFrame(silence)
	}
	for i := 0; i < 20; i++ {
		vad.ProcessFrame(speech)
	}
	for i := 0; i < 40; i++ {
		vad.ProcessFrame(silence)
	}

	if !speechStarted {
		t.Fatal("VAD não detectou início de fala")
	}
	if len(segment) == 0 {
		t.Fatal("VAD não emitiu segmento")
	}

	leadingBytes := int((100*time.Millisecond).Seconds()*8000) * 2
	if len(segment) < leadingBytes {
		t.Fatalf("segmento muito curto: got=%d want>=%d", len(segment), leadingBytes)
	}
}

func TestSpeechDetectorFlush(t *testing.T) {
	var segment []byte

	cfg := DefaultVADConfig()
	cfg.SampleRate = 8000
	cfg.Mode = 0
	cfg.WarmupDuration = 0
	cfg.SpeechDuration = 60 * time.Millisecond

	vad, err := NewSpeechDetector(cfg, func() {}, func(seg []byte) {
		segment = append([]byte(nil), seg...)
	})
	if err != nil {
		t.Fatalf("NewSpeechDetector() error = %v", err)
	}

	speech := generateSpeechLike(20, 8000, 0.9)
	for i := 0; i < 20; i++ {
		vad.ProcessFrame(speech)
	}

	vad.Flush()

	if len(segment) == 0 {
		t.Fatal("Flush() não emitiu segmento")
	}
	if vad.State() != VADSilence {
		t.Fatalf("State() after Flush = %v, want %v", vad.State(), VADSilence)
	}
}

func TestSpeechDetectorMaxSegmentDuration(t *testing.T) {
	var emitted int

	cfg := DefaultVADConfig()
	cfg.SampleRate = 8000
	cfg.Mode = 0
	cfg.WarmupDuration = 0
	cfg.SpeechDuration = 60 * time.Millisecond
	cfg.MaxSegmentDuration = 400 * time.Millisecond

	vad, err := NewSpeechDetector(cfg, nil, func([]byte) {
		emitted++
	})
	if err != nil {
		t.Fatalf("NewSpeechDetector() error = %v", err)
	}

	speech := generateSpeechLike(20, 8000, 0.9)
	for i := 0; i < 40; i++ {
		vad.ProcessFrame(speech)
	}

	if emitted < 2 {
		t.Fatalf("esperava múltiplos segmentos por max duration, got=%d", emitted)
	}
}

func TestAudioPreprocessorProcessCaptureKeepsFrameLength(t *testing.T) {
	pre, err := NewAudioPreprocessor(DefaultPreprocessConfig())
	if err != nil {
		t.Fatalf("NewAudioPreprocessor() error = %v", err)
	}
	defer pre.Close()

	input := generateSpeechLike(20, 8000, 0.4)
	output, err := pre.ProcessCapture(input)
	if err != nil {
		t.Fatalf("ProcessCapture() error = %v", err)
	}

	if len(output) != len(input) {
		t.Fatalf("ProcessCapture() len = %d, want %d", len(output), len(input))
	}
}
