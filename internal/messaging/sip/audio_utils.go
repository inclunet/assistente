package sip

import (
	"encoding/binary"
	"math"

	resampling "github.com/tphakala/go-audio-resampling"
)

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

func pcmBytesToInt16(pcm []byte) []int16 {
	samples := make([]int16, len(pcm)/2)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(pcm[i*2 : i*2+2]))
	}
	return samples
}

func pcmInt16ToBytes(samples []int16) []byte {
	out := make([]byte, len(samples)*2)
	for i, sample := range samples {
		binary.LittleEndian.PutUint16(out[i*2:], uint16(sample))
	}
	return out
}

func resamplePCMMono(samples []int16, srcRate, dstRate int) ([]int16, error) {
	if len(samples) == 0 || srcRate == dstRate {
		out := make([]int16, len(samples))
		copy(out, samples)
		return out, nil
	}

	floats := make([]float64, len(samples))
	for i, sample := range samples {
		floats[i] = float64(sample) / 32768.0
	}

	resampled, err := resampling.ResampleMono(floats, float64(srcRate), float64(dstRate), resampling.QualityHigh)
	if err != nil {
		return nil, err
	}

	out := make([]int16, len(resampled))
	for i, sample := range resampled {
		switch {
		case sample > 1.0:
			sample = 1.0
		case sample < -1.0:
			sample = -1.0
		}
		out[i] = int16(math.Round(sample * 32767.0))
	}
	return out, nil
}
