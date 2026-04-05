package sip

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestResample8to16(t *testing.T) {
	// Cria 4 amostras a 8kHz: [100, 200, 300, 400]
	input := make([]byte, 8)
	binary.LittleEndian.PutUint16(input[0:], uint16(100))
	binary.LittleEndian.PutUint16(input[2:], uint16(200))
	binary.LittleEndian.PutUint16(input[4:], uint16(300))
	binary.LittleEndian.PutUint16(input[6:], uint16(400))

	out := Resample8to16(input)

	// Espera 8 amostras (dobro)
	if len(out) != 16 {
		t.Fatalf("sa├¡da esperada 16 bytes, obteve %d", len(out))
	}

	samples := make([]int16, 8)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(out[i*2:]))
	}

	// Verifica: s[0]=100, s[1]=midpoint(100,200)=150, s[2]=200, s[3]=midpoint(200,300)=250, etc.
	if samples[0] != 100 {
		t.Errorf("sample[0] = %d, esperado 100", samples[0])
	}
	if samples[1] != 150 {
		t.Errorf("sample[1] = %d, esperado 150", samples[1])
	}
	if samples[2] != 200 {
		t.Errorf("sample[2] = %d, esperado 200", samples[2])
	}
	if samples[3] != 250 {
		t.Errorf("sample[3] = %d, esperado 250", samples[3])
	}
}

func TestResample8to16_Empty(t *testing.T) {
	if out := Resample8to16(nil); out != nil {
		t.Errorf("esperado nil para entrada vazia, obteve %v", out)
	}
	if out := Resample8to16([]byte{}); out != nil {
		t.Errorf("esperado nil para entrada vazia, obteve %v", out)
	}
}

func TestResample16to8(t *testing.T) {
	// Cria 4 amostras a 16kHz: [100, 200, 300, 400]
	input := make([]byte, 8)
	binary.LittleEndian.PutUint16(input[0:], uint16(100))
	binary.LittleEndian.PutUint16(input[2:], uint16(200))
	binary.LittleEndian.PutUint16(input[4:], uint16(300))
	binary.LittleEndian.PutUint16(input[6:], uint16(400))

	out := Resample16to8(input)

	// Espera 2 amostras (metade): avg(100,200)=150, avg(300,400)=350
	if len(out) != 4 {
		t.Fatalf("sa├¡da esperada 4 bytes, obteve %d", len(out))
	}

	s0 := int16(binary.LittleEndian.Uint16(out[0:]))
	s1 := int16(binary.LittleEndian.Uint16(out[2:]))

	if s0 != 150 {
		t.Errorf("sample[0] = %d, esperado 150", s0)
	}
	if s1 != 350 {
		t.Errorf("sample[1] = %d, esperado 350", s1)
	}
}

func TestResample24to8(t *testing.T) {
	// Cria 6 amostras a 24kHz: [100, 200, 300, 400, 500, 600]
	input := make([]byte, 12)
	for i := 0; i < 6; i++ {
		binary.LittleEndian.PutUint16(input[i*2:], uint16((i+1)*100))
	}

	out := Resample24to8(input)

	// Raz├úo 3:1 ÔåÆ 2 amostras: avg(100,200,300)=200, avg(400,500,600)=500
	if len(out) != 4 {
		t.Fatalf("sa├¡da esperada 4 bytes, obteve %d", len(out))
	}

	s0 := int16(binary.LittleEndian.Uint16(out[0:]))
	s1 := int16(binary.LittleEndian.Uint16(out[2:]))

	if s0 != 200 {
		t.Errorf("sample[0] = %d, esperado 200", s0)
	}
	if s1 != 500 {
		t.Errorf("sample[1] = %d, esperado 500", s1)
	}
}

func TestResample24to16(t *testing.T) {
	// Cria 6 amostras a 24kHz
	input := make([]byte, 12)
	for i := 0; i < 6; i++ {
		binary.LittleEndian.PutUint16(input[i*2:], uint16((i+1)*100))
	}

	out := Resample24to16(input)

	// Raz├úo 3:2 ÔåÆ 4 amostras
	if len(out) != 8 {
		t.Fatalf("sa├¡da esperada 8 bytes, obteve %d", len(out))
	}

	// Verifica que os samples s├úo razo├íveis (interpola├º├úo)
	for i := 0; i < 4; i++ {
		s := int16(binary.LittleEndian.Uint16(out[i*2:]))
		if s < 0 || s > 700 {
			t.Errorf("sample[%d] = %d fora do range esperado", i, s)
		}
	}
}

func TestResample_RoundTrip(t *testing.T) {
	// Gera tom senoidal a 8kHz
	numSamples := 800 // 100ms @ 8kHz
	input := make([]byte, numSamples*2)
	for i := 0; i < numSamples; i++ {
		val := math.Sin(2 * math.Pi * 440 * float64(i) / 8000)
		sample := int16(val * 16000)
		binary.LittleEndian.PutUint16(input[i*2:], uint16(sample))
	}

	// 8kHz ÔåÆ 16kHz ÔåÆ 8kHz round-trip
	up := Resample8to16(input)
	down := Resample16to8(up)

	if len(down) != len(input) {
		t.Fatalf("tamanho p├│s round-trip: %d, esperado %d", len(down), len(input))
	}

	// Verifica que as amostras s├úo pr├│ximas (margem de erro por interpola├º├úo)
	maxDiff := int16(0)
	for i := 0; i < numSamples; i++ {
		orig := int16(binary.LittleEndian.Uint16(input[i*2:]))
		roundTrip := int16(binary.LittleEndian.Uint16(down[i*2:]))
		diff := orig - roundTrip
		if diff < 0 {
			diff = -diff
		}
		if diff > maxDiff {
			maxDiff = diff
		}
	}

	// Toler├óncia: round-trip com interpola├º├úo linear pode ter at├® ~10% de erro pico
	if maxDiff > 3200 { // ~20% de 16000
		t.Errorf("Erro m├íximo no round-trip muito alto: %d", maxDiff)
	}
}

func TestStereoToMono(t *testing.T) {
	// Cria 2 frames stereo: L=100,R=200 / L=300,R=400
	stereo := make([]byte, 8)
	binary.LittleEndian.PutUint16(stereo[0:], uint16(100))
	binary.LittleEndian.PutUint16(stereo[2:], uint16(200))
	binary.LittleEndian.PutUint16(stereo[4:], uint16(300))
	binary.LittleEndian.PutUint16(stereo[6:], uint16(400))

	mono := stereoToMono(stereo)

	if len(mono) != 4 {
		t.Fatalf("sa├¡da esperada 4 bytes, obteve %d", len(mono))
	}

	s0 := int16(binary.LittleEndian.Uint16(mono[0:]))
	s1 := int16(binary.LittleEndian.Uint16(mono[2:]))

	if s0 != 150 {
		t.Errorf("sample[0] = %d, esperado 150 (avg 100,200)", s0)
	}
	if s1 != 350 {
		t.Errorf("sample[1] = %d, esperado 350 (avg 300,400)", s1)
	}
}

func TestResampleGenericTo8k(t *testing.T) {
	// 16 amostras a 16kHz
	input := make([]byte, 32)
	for i := 0; i < 16; i++ {
		binary.LittleEndian.PutUint16(input[i*2:], uint16(i*1000))
	}

	out := resampleGenericTo8k(input, 16000)

	// Espera 8 amostras (ratio 2:1)
	outSamples := len(out) / 2
	if outSamples != 8 {
		t.Fatalf("esperadas 8 amostras, obteve %d", outSamples)
	}
}

func TestResampleGenericTo8k_SameRate(t *testing.T) {
	input := make([]byte, 16)
	out := resampleGenericTo8k(input, 8000)
	if len(out) != len(input) {
		t.Errorf("mesmo sample rate deve retornar mesmo tamanho")
	}
}

func TestEncodePCMToWAV(t *testing.T) {
	pcm := make([]byte, 3200) // 100ms @ 16kHz
	wav := encodePCMToWAV(pcm, 16000, 16, 1)

	// Header WAV = 44 bytes
	if len(wav) != 44+len(pcm) {
		t.Fatalf("WAV esperado %d bytes, obteve %d", 44+len(pcm), len(wav))
	}

	// Verifica RIFF header
	if string(wav[0:4]) != "RIFF" {
		t.Error("Missing RIFF header")
	}
	if string(wav[8:12]) != "WAVE" {
		t.Error("Missing WAVE format")
	}

	// Verifica sample rate
	sr := binary.LittleEndian.Uint32(wav[24:28])
	if sr != 16000 {
		t.Errorf("sample rate no WAV: %d, esperado 16000", sr)
	}
}

func TestExtractPCMFromWAV(t *testing.T) {
	pcm := make([]byte, 1600)
	wav := encodePCMToWAV(pcm, 16000, 16, 1)

	extractedPCM, sampleRate := extractPCMFromWAV(wav)

	if sampleRate != 16000 {
		t.Errorf("sample rate: %d, esperado 16000", sampleRate)
	}
	if len(extractedPCM) != len(pcm) {
		t.Errorf("PCM extra├¡do: %d bytes, esperado %d", len(extractedPCM), len(pcm))
	}
}

func TestExtractPCMFromWAV_Invalid(t *testing.T) {
	pcm, sr := extractPCMFromWAV([]byte("short"))
	if pcm != nil || sr != 0 {
		t.Error("WAV inv├ílido deveria retornar nil, 0")
	}
}

func TestConvertToPCM8k_WAV(t *testing.T) {
	// WAV 16kHz ÔåÆ 8kHz
	pcm16k := make([]byte, 320) // 10ms @ 16kHz
	wav := encodePCMToWAV(pcm16k, 16000, 16, 1)

	result, err := convertToPCM8k(wav, "audio/wav")
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if result == nil {
		t.Fatal("resultado nil")
	}
	// 16kHz ÔåÆ 8kHz: metade dos samples
	expectedLen := len(pcm16k) / 2
	if len(result) != expectedLen {
		t.Errorf("resultado: %d bytes, esperado %d", len(result), expectedLen)
	}
}

func TestConvertToPCM8k_UnsupportedFormat(t *testing.T) {
	_, err := convertToPCM8k([]byte("data"), "video/mp4")
	if err == nil {
		t.Error("esperado erro para formato n├úo suportado")
	}
}