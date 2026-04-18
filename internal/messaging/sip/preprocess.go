package sip

import (
	"fmt"

	"assistente/internal/audio/speex"
)

const (
	sipProcessingSampleRate = 8000
	sipProcessingFrameMs    = 20
)

type NoiseSuppressionLevel int

const (
	NoiseSuppressionLow NoiseSuppressionLevel = iota + 1
	NoiseSuppressionModerate
	NoiseSuppressionHigh
	NoiseSuppressionVeryHigh
)

type PreprocessConfig struct {
	EnableDenoise    bool
	EnableAGC        bool
	NoiseSuppression NoiseSuppressionLevel
	NoiseSuppressDB  int
	AGCTarget        int
	AGCMaxGainDB     int
	AGCIncrementDB   int
	AGCDecrementDB   int
}

func DefaultPreprocessConfig() PreprocessConfig {
	return PreprocessConfig{
		EnableDenoise:    true,
		EnableAGC:        true,
		NoiseSuppression: NoiseSuppressionHigh,
		NoiseSuppressDB:  -24,
		AGCTarget:        24000,
		AGCMaxGainDB:     30,
		AGCIncrementDB:   12,
		AGCDecrementDB:   18,
	}
}

type AudioPreprocessor struct {
	processor  *speex.Preprocessor
	inputBuf   []byte
	frameBytes int
}

func NewAudioPreprocessor(cfg PreprocessConfig) (*AudioPreprocessor, error) {
	defaults := DefaultPreprocessConfig()
	if cfg.NoiseSuppression == 0 {
		cfg.NoiseSuppression = defaults.NoiseSuppression
	}
	if cfg.NoiseSuppressDB == 0 {
		cfg.NoiseSuppressDB = defaults.NoiseSuppressDB
	}
	if cfg.AGCTarget == 0 {
		cfg.AGCTarget = defaults.AGCTarget
	}
	if cfg.AGCMaxGainDB == 0 {
		cfg.AGCMaxGainDB = defaults.AGCMaxGainDB
	}
	if cfg.AGCIncrementDB == 0 {
		cfg.AGCIncrementDB = defaults.AGCIncrementDB
	}
	if cfg.AGCDecrementDB == 0 {
		cfg.AGCDecrementDB = defaults.AGCDecrementDB
	}

	frameSamples := sipProcessingSampleRate * sipProcessingFrameMs / 1000
	processor, err := speex.NewPreprocessor(sipProcessingSampleRate, frameSamples, speex.Config{
		EnableDenoise:   cfg.EnableDenoise,
		EnableAGC:       cfg.EnableAGC,
		NoiseSuppressDB: mapNoiseSuppressionLevel(cfg.NoiseSuppression, cfg.NoiseSuppressDB),
		AGCTarget:       cfg.AGCTarget,
		AGCMaxGainDB:    cfg.AGCMaxGainDB,
		AGCIncrementDB:  cfg.AGCIncrementDB,
		AGCDecrementDB:  cfg.AGCDecrementDB,
	})
	if err != nil {
		return nil, fmt.Errorf("criar preprocessador SpeexDSP: %w", err)
	}

	return &AudioPreprocessor{
		processor:  processor,
		frameBytes: frameSamples * 2,
		inputBuf:   make([]byte, 0, frameSamples*4),
	}, nil
}

func (p *AudioPreprocessor) Close() error {
	if p == nil || p.processor == nil {
		return nil
	}
	return p.processor.Close()
}

func (p *AudioPreprocessor) ProcessCapture(pcm []byte) ([]byte, error) {
	if len(pcm) == 0 {
		return nil, nil
	}

	p.inputBuf = append(p.inputBuf, pcm...)
	out := make([]byte, 0, len(pcm))

	for len(p.inputBuf) >= p.frameBytes {
		frame := p.inputBuf[:p.frameBytes]
		processed, err := p.processFrame(frame)
		if err != nil {
			return nil, err
		}
		out = append(out, processed...)
		p.inputBuf = p.inputBuf[p.frameBytes:]
	}

	return out, nil
}

func (p *AudioPreprocessor) processFrame(pcm []byte) ([]byte, error) {
	samples := pcmBytesToInt16(pcm)
	if err := p.processor.Process(samples); err != nil {
		return nil, fmt.Errorf("processar frame no SpeexDSP: %w", err)
	}

	return pcmInt16ToBytes(samples), nil
}

func mapNoiseSuppressionLevel(level NoiseSuppressionLevel, fallback int) int {
	switch level {
	case NoiseSuppressionLow:
		return -12
	case NoiseSuppressionModerate:
		return -18
	case NoiseSuppressionVeryHigh:
		return -32
	case NoiseSuppressionHigh:
		return -24
	default:
		return fallback
	}
}
