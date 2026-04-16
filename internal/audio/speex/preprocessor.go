package speex

/*
#cgo CFLAGS: -I${SRCDIR}/include -I${SRCDIR} -DHAVE_CONFIG_H

#include <speex/speex_preprocess.h>
#include <stdlib.h>

static int speex_preprocess_set_int(SpeexPreprocessState *st, int request, int value) {
	return speex_preprocess_ctl(st, request, &value);
}

static int speex_preprocess_get_int(SpeexPreprocessState *st, int request) {
	int value = 0;
	speex_preprocess_ctl(st, request, &value);
	return value;
}
*/
import "C"

import "fmt"

type Config struct {
	EnableDenoise   bool
	EnableAGC       bool
	NoiseSuppressDB int
	AGCTarget       int
	AGCMaxGainDB    int
	AGCIncrementDB  int
	AGCDecrementDB  int
}

func DefaultConfig() Config {
	return Config{
		EnableDenoise:   true,
		EnableAGC:       true,
		NoiseSuppressDB: -24,
		AGCTarget:       24000,
		AGCMaxGainDB:    30,
		AGCIncrementDB:  12,
		AGCDecrementDB:  18,
	}
}

type Preprocessor struct {
	state      *C.SpeexPreprocessState
	frameSize  int
	sampleRate int
}

func NewPreprocessor(sampleRate, frameSize int, cfg Config) (*Preprocessor, error) {
	if sampleRate <= 0 {
		return nil, fmt.Errorf("sampleRate inválido: %d", sampleRate)
	}
	if frameSize <= 0 {
		return nil, fmt.Errorf("frameSize inválido: %d", frameSize)
	}

	state := C.speex_preprocess_state_init(C.int(frameSize), C.int(sampleRate))
	if state == nil {
		return nil, fmt.Errorf("falha ao criar SpeexPreprocessState")
	}

	p := &Preprocessor{
		state:      state,
		frameSize:  frameSize,
		sampleRate: sampleRate,
	}

	if err := p.applyConfig(cfg); err != nil {
		_ = p.Close()
		return nil, err
	}

	return p, nil
}

func (p *Preprocessor) Close() error {
	if p == nil || p.state == nil {
		return nil
	}
	C.speex_preprocess_state_destroy(p.state)
	p.state = nil
	return nil
}

func (p *Preprocessor) Process(samples []int16) error {
	if p == nil || p.state == nil {
		return fmt.Errorf("preprocessador Speex fechado")
	}
	if len(samples) != p.frameSize {
		return fmt.Errorf("frame inválido: got=%d want=%d", len(samples), p.frameSize)
	}

	C.speex_preprocess_run(p.state, (*C.spx_int16_t)(&samples[0]))
	return nil
}

func (p *Preprocessor) NoiseSuppressDB() int {
	return int(C.speex_preprocess_get_int(p.state, C.SPEEX_PREPROCESS_GET_NOISE_SUPPRESS))
}

func (p *Preprocessor) AGCGainDB() int {
	return int(C.speex_preprocess_get_int(p.state, C.SPEEX_PREPROCESS_GET_AGC_GAIN))
}

func (p *Preprocessor) applyConfig(cfg Config) error {
	if err := p.setBool(C.SPEEX_PREPROCESS_SET_DENOISE, cfg.EnableDenoise); err != nil {
		return err
	}
	if err := p.setBool(C.SPEEX_PREPROCESS_SET_AGC, cfg.EnableAGC); err != nil {
		return err
	}
	if cfg.NoiseSuppressDB != 0 {
		if err := p.setInt(C.SPEEX_PREPROCESS_SET_NOISE_SUPPRESS, cfg.NoiseSuppressDB); err != nil {
			return err
		}
	}
	if cfg.EnableAGC {
		if cfg.AGCTarget > 0 {
			if err := p.setInt(C.SPEEX_PREPROCESS_SET_AGC_TARGET, cfg.AGCTarget); err != nil {
				return err
			}
		}
		if cfg.AGCMaxGainDB > 0 {
			if err := p.setInt(C.SPEEX_PREPROCESS_SET_AGC_MAX_GAIN, cfg.AGCMaxGainDB); err != nil {
				return err
			}
		}
		if cfg.AGCIncrementDB > 0 {
			if err := p.setInt(C.SPEEX_PREPROCESS_SET_AGC_INCREMENT, cfg.AGCIncrementDB); err != nil {
				return err
			}
		}
		if cfg.AGCDecrementDB > 0 {
			if err := p.setInt(C.SPEEX_PREPROCESS_SET_AGC_DECREMENT, cfg.AGCDecrementDB); err != nil {
				return err
			}
		}
	}

	return nil
}

func (p *Preprocessor) setBool(request C.int, value bool) error {
	v := 0
	if value {
		v = 1
	}
	return p.setInt(request, v)
}

func (p *Preprocessor) setInt(request C.int, value int) error {
	if rc := C.speex_preprocess_set_int(p.state, request, C.int(value)); rc == 0 {
		return nil
	}
	return fmt.Errorf("speex_preprocess_ctl falhou request=%d value=%d", int(request), value)
}
