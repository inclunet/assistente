package sip

import (
	"fmt"
	"log"
	"time"

	webrtcvad "github.com/bytectlgo/webrtcvad-go"
)

type VADState int

const (
	VADSilence VADState = iota
	VADSpeech
)

type VADConfig struct {
	Mode                  int
	FrameDuration         time.Duration
	SilenceDuration       time.Duration
	SpeechDuration        time.Duration
	LeadingBufferDuration time.Duration
	SampleRate            int
	MaxSegmentDuration    time.Duration
	WarmupDuration        time.Duration
	NoiseFloorAlpha       float64
	NoiseFloorMin         float64
}

func DefaultVADConfig() VADConfig {
	return VADConfig{
		Mode:                  1,
		FrameDuration:         20 * time.Millisecond,
		SilenceDuration:       800 * time.Millisecond,
		SpeechDuration:        120 * time.Millisecond,
		LeadingBufferDuration: 200 * time.Millisecond,
		SampleRate:            8000,
		MaxSegmentDuration:    15 * time.Second,
		WarmupDuration:        1500 * time.Millisecond,
		NoiseFloorAlpha:       0.02,
		NoiseFloorMin:         0.01,
	}
}

type SpeechDetector struct {
	config VADConfig
	vad    *webrtcvad.VAD

	state VADState

	frameBytes             int
	speechFramesThreshold  int
	silenceFramesThreshold int
	warmupFramesThreshold  int
	maxSegmentBytes        int

	pendingBuf []byte
	leadingBuf []byte
	speechBuf  []byte

	speechFrames  int
	silenceFrames int
	warmupFrames  int

	noiseFloor float64

	onSpeechStart func()
	onSpeechEnd   func(segment []byte)
}

func NewSpeechDetector(cfg VADConfig, onSpeechStart func(), onSpeechEnd func(segment []byte)) (*SpeechDetector, error) {
	if cfg.SampleRate == 0 {
		cfg.SampleRate = 8000
	}
	if cfg.Mode < 0 || cfg.Mode > 3 {
		cfg.Mode = 2
	}
	if cfg.FrameDuration == 0 {
		cfg.FrameDuration = 20 * time.Millisecond
	}
	if cfg.FrameDuration != 10*time.Millisecond && cfg.FrameDuration != 20*time.Millisecond && cfg.FrameDuration != 30*time.Millisecond {
		return nil, fmt.Errorf("frame duration inválida para WebRTC VAD: %s", cfg.FrameDuration)
	}
	if cfg.SilenceDuration < 0 {
		cfg.SilenceDuration = 800 * time.Millisecond
	}
	if cfg.SpeechDuration < 0 {
		cfg.SpeechDuration = 120 * time.Millisecond
	}
	if cfg.LeadingBufferDuration < 0 {
		cfg.LeadingBufferDuration = 200 * time.Millisecond
	}
	if cfg.MaxSegmentDuration < 0 {
		cfg.MaxSegmentDuration = 15 * time.Second
	}
	if cfg.WarmupDuration < 0 {
		cfg.WarmupDuration = 1500 * time.Millisecond
	}
	if cfg.NoiseFloorAlpha <= 0 {
		cfg.NoiseFloorAlpha = 0.02
	}
	if cfg.NoiseFloorMin <= 0 {
		cfg.NoiseFloorMin = 0.01
	}

	frameSamples := int(cfg.FrameDuration.Seconds() * float64(cfg.SampleRate))
	if frameSamples <= 0 || !webrtcvad.ValidRateAndFrameLength(cfg.SampleRate, frameSamples) {
		return nil, fmt.Errorf("combinação inválida de sampleRate=%d e frame=%s para WebRTC VAD", cfg.SampleRate, cfg.FrameDuration)
	}

	vad, err := webrtcvad.New(cfg.Mode)
	if err != nil {
		return nil, fmt.Errorf("criar WebRTC VAD: %w", err)
	}

	frameBytes := frameSamples * 2
	speechFramesThreshold := durationToFrames(cfg.SpeechDuration, cfg.FrameDuration)
	silenceFramesThreshold := durationToFrames(cfg.SilenceDuration, cfg.FrameDuration)
	warmupFramesThreshold := 0
	if cfg.WarmupDuration > 0 {
		warmupFramesThreshold = durationToFrames(cfg.WarmupDuration, cfg.FrameDuration)
	}
	leadingBufBytes := 0
	if cfg.LeadingBufferDuration > 0 {
		leadingBufBytes = durationToFrames(cfg.LeadingBufferDuration, cfg.FrameDuration) * frameBytes
	}
	maxSegmentBytes := 0
	if cfg.MaxSegmentDuration > 0 {
		maxSegmentBytes = durationToFrames(cfg.MaxSegmentDuration, cfg.FrameDuration) * frameBytes
	}

	return &SpeechDetector{
		config:                 cfg,
		vad:                    vad,
		state:                  VADSilence,
		frameBytes:             frameBytes,
		speechFramesThreshold:  speechFramesThreshold,
		silenceFramesThreshold: silenceFramesThreshold,
		warmupFramesThreshold:  warmupFramesThreshold,
		leadingBuf:             make([]byte, 0, leadingBufBytes),
		speechBuf:              make([]byte, 0, max(frameBytes*8, leadingBufBytes)),
		noiseFloor:             cfg.NoiseFloorMin,
		maxSegmentBytes:        maxSegmentBytes,
		onSpeechStart:          onSpeechStart,
		onSpeechEnd:            onSpeechEnd,
	}, nil
}

func (d *SpeechDetector) ProcessFrame(pcm []byte) VADState {
	if len(pcm) == 0 {
		return d.state
	}

	d.pendingBuf = append(d.pendingBuf, pcm...)
	for len(d.pendingBuf) >= d.frameBytes {
		frame := make([]byte, d.frameBytes)
		copy(frame, d.pendingBuf[:d.frameBytes])
		d.pendingBuf = d.pendingBuf[d.frameBytes:]
		d.processOneFrame(frame)
	}

	return d.state
}

func (d *SpeechDetector) processOneFrame(frame []byte) {
	rms := computeRMS(frame)
	d.appendLeading(frame)

	isSpeech, err := d.vad.IsSpeech(frame, d.config.SampleRate)
	if err != nil {
		log.Printf("[SIP VAD] Erro ao processar frame: %v", err)
		return
	}

	if !isSpeech {
		d.updateNoiseFloor(rms)
	}

	if d.warmupFrames < d.warmupFramesThreshold {
		d.warmupFrames++
		if d.warmupFrames == d.warmupFramesThreshold {
			log.Printf("[SIP VAD] Warm-up completo (noiseFloor=%.4f)", d.noiseFloor)
		}
		return
	}

	switch d.state {
	case VADSilence:
		if isSpeech {
			d.speechFrames++
			if d.speechFrames >= d.speechFramesThreshold {
				d.state = VADSpeech
				d.silenceFrames = 0
				d.speechBuf = append(d.speechBuf[:0], d.leadingBuf...)
				if d.onSpeechStart != nil {
					d.onSpeechStart()
				}
			}
		} else {
			d.speechFrames = 0
		}
	case VADSpeech:
		d.speechBuf = append(d.speechBuf, frame...)
		if d.maxSegmentBytes > 0 && len(d.speechBuf) >= d.maxSegmentBytes {
			d.emitSegment()
			return
		}

		if isSpeech {
			d.silenceFrames = 0
		} else {
			d.silenceFrames++
			if d.silenceFrames >= d.silenceFramesThreshold {
				d.emitSegment()
			}
		}
	}
}

func (d *SpeechDetector) Flush() {
	if d.state == VADSpeech && len(d.speechBuf) > 0 {
		d.emitSegment()
	}
}

func (d *SpeechDetector) Reset() error {
	if err := d.vad.SetMode(d.config.Mode); err != nil {
		return err
	}
	d.state = VADSilence
	d.pendingBuf = d.pendingBuf[:0]
	d.leadingBuf = d.leadingBuf[:0]
	d.speechBuf = d.speechBuf[:0]
	d.speechFrames = 0
	d.silenceFrames = 0
	d.warmupFrames = 0
	d.noiseFloor = d.config.NoiseFloorMin
	return nil
}

func (d *SpeechDetector) State() VADState {
	return d.state
}

func (d *SpeechDetector) NoiseFloor() float64 {
	return d.noiseFloor
}

func (d *SpeechDetector) appendLeading(frame []byte) {
	limit := cap(d.leadingBuf)
	if limit == 0 {
		return
	}
	d.leadingBuf = append(d.leadingBuf, frame...)
	if len(d.leadingBuf) > limit {
		d.leadingBuf = d.leadingBuf[len(d.leadingBuf)-limit:]
	}
}

func (d *SpeechDetector) updateNoiseFloor(rms float64) {
	if rms <= 0 {
		return
	}
	alpha := d.config.NoiseFloorAlpha
	d.noiseFloor = alpha*rms + (1-alpha)*d.noiseFloor
	if d.noiseFloor < d.config.NoiseFloorMin {
		d.noiseFloor = d.config.NoiseFloorMin
	}
}

func (d *SpeechDetector) emitSegment() {
	segment := make([]byte, len(d.speechBuf))
	copy(segment, d.speechBuf)
	if d.onSpeechEnd != nil && len(segment) > 0 {
		d.onSpeechEnd(segment)
	}
	d.state = VADSilence
	d.speechBuf = d.speechBuf[:0]
	d.speechFrames = 0
	d.silenceFrames = 0
}

func durationToFrames(total time.Duration, frame time.Duration) int {
	if total <= 0 || frame <= 0 {
		return 1
	}
	frames := int(total / frame)
	if total%frame != 0 {
		frames++
	}
	if frames < 1 {
		return 1
	}
	return frames
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
