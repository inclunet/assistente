package sip

// Resample8to16 converte ├íudio PCM 16-bit signed LE de 8kHz para 16kHz
// usando interpola├º├úo linear. Cada sample ├® duplicado com valor interpolado
// entre amostras vizinhas para suavizar a sa├¡da.
//
// O ├íudio de entrada ├® PCM 16-bit signed little-endian mono (padr├úo G.711 decodificado).
// A sa├¡da ├® PCM 16-bit signed little-endian mono em 16kHz (formato esperado pelo Whisper).
func Resample8to16(pcm8k []byte) []byte {
	numSamples := len(pcm8k) / 2
	if numSamples == 0 {
		return nil
	}

	// Sa├¡da tem o dobro de samples
	out := make([]byte, numSamples*4) // 2x samples * 2 bytes cada

	for i := 0; i < numSamples; i++ {
		// Sample atual
		s := int16(uint16(pcm8k[i*2]) | uint16(pcm8k[i*2+1])<<8)

		// Pr├│ximo sample (ou repete o ├║ltimo)
		var sNext int16
		if i+1 < numSamples {
			sNext = int16(uint16(pcm8k[(i+1)*2]) | uint16(pcm8k[(i+1)*2+1])<<8)
		} else {
			sNext = s
		}

		// Interpola├º├úo: sample original + ponto m├®dio
		mid := int16((int32(s) + int32(sNext)) / 2)

		outIdx := i * 4
		out[outIdx] = byte(s)
		out[outIdx+1] = byte(s >> 8)
		out[outIdx+2] = byte(mid)
		out[outIdx+3] = byte(mid >> 8)
	}

	return out
}

// Resample16to8 converte ├íudio PCM 16-bit signed LE de 16kHz para 8kHz
// tomando a m├®dia de pares de amostras consecutivas.
//
// Entrada: PCM 16-bit signed little-endian mono 16kHz
// Sa├¡da: PCM 16-bit signed little-endian mono 8kHz
func Resample16to8(pcm16k []byte) []byte {
	numSamples := len(pcm16k) / 2
	if numSamples < 2 {
		return nil
	}

	outSamples := numSamples / 2
	out := make([]byte, outSamples*2)

	for i := 0; i < outSamples; i++ {
		srcIdx := i * 4
		s1 := int16(uint16(pcm16k[srcIdx]) | uint16(pcm16k[srcIdx+1])<<8)
		s2 := int16(uint16(pcm16k[srcIdx+2]) | uint16(pcm16k[srcIdx+3])<<8)

		avg := int16((int32(s1) + int32(s2)) / 2)
		out[i*2] = byte(avg)
		out[i*2+1] = byte(avg >> 8)
	}

	return out
}

// Resample24to16 converte ├íudio PCM 16-bit signed LE de 24kHz para 16kHz.
// Usa decima├º├úo com filtro de m├®dia (a cada 3 amostras, toma 2 sa├¡das).
// Entrada: PCM 16-bit signed little-endian mono 24kHz (formato OpenAI TTS "pcm").
// Sa├¡da: PCM 16-bit signed little-endian mono 16kHz.
func Resample24to16(pcm24k []byte) []byte {
	numSamples := len(pcm24k) / 2
	if numSamples < 3 {
		return nil
	}

	// Raz├úo 24kÔåÆ16k = 3:2
	// Para cada bloco de 3 amostras, gera 2 amostras de sa├¡da
	outSamples := (numSamples * 2) / 3
	out := make([]byte, outSamples*2)

	outIdx := 0
	for i := 0; i+2 < numSamples && outIdx+1 < outSamples; i += 3 {
		s0 := readSample(pcm24k, i)
		s1 := readSample(pcm24k, i+1)
		s2 := readSample(pcm24k, i+2)

		// Interpola├º├úo: out[0] = s0*2/3 + s1*1/3, out[1] = s1*1/3 + s2*2/3
		o0 := int16((int32(s0)*2 + int32(s1)) / 3)
		o1 := int16((int32(s1) + int32(s2)*2) / 3)

		writeSample(out, outIdx, o0)
		writeSample(out, outIdx+1, o1)
		outIdx += 2
	}

	return out[:outIdx*2]
}

// Resample24to8 converte ├íudio PCM 16-bit signed LE de 24kHz para 8kHz.
// Raz├úo 3:1 ÔÇö toma a m├®dia de cada 3 amostras consecutivas.
// Entrada: PCM 16-bit signed little-endian mono 24kHz (formato OpenAI TTS "pcm").
// Sa├¡da: PCM 16-bit signed little-endian mono 8kHz (formato G.711/RTP).
func Resample24to8(pcm24k []byte) []byte {
	numSamples := len(pcm24k) / 2
	if numSamples < 3 {
		return nil
	}

	outSamples := numSamples / 3
	out := make([]byte, outSamples*2)

	for i := 0; i < outSamples; i++ {
		srcIdx := i * 3
		s0 := readSample(pcm24k, srcIdx)
		s1 := readSample(pcm24k, srcIdx+1)
		s2 := readSample(pcm24k, srcIdx+2)

		avg := int16((int32(s0) + int32(s1) + int32(s2)) / 3)
		writeSample(out, i, avg)
	}

	return out
}

func readSample(pcm []byte, idx int) int16 {
	off := idx * 2
	return int16(uint16(pcm[off]) | uint16(pcm[off+1])<<8)
}

func writeSample(buf []byte, idx int, val int16) {
	off := idx * 2
	buf[off] = byte(val)
	buf[off+1] = byte(val >> 8)
}