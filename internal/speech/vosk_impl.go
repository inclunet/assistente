//go:build vosk
// +build vosk

package speech

/*
#cgo LDFLAGS: -lvosk
#include <stdlib.h>
#include "vosk_api.h"
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"sync"
	"unsafe"
)

var (
	voskModel      *C.VoskModel
	voskRecognizer *C.VoskRecognizer
	voskMutex      sync.Mutex
	voskStreaming  bool
	streamConfig   VoskStreamingConfig
)

// isVoskAvailable retorna se Vosk está disponível
func isVoskAvailable() bool {
	return true
}

// loadVoskModel carrega um modelo Vosk
func loadVoskModel(modelPath string) error {
	voskMutex.Lock()
	defer voskMutex.Unlock()

	// Libera modelo anterior se existir
	if voskModel != nil {
		C.vosk_model_free(voskModel)
		voskModel = nil
	}

	// Carrega novo modelo
	cPath := C.CString(modelPath)
	defer C.free(unsafe.Pointer(cPath))

	voskModel = C.vosk_model_new(cPath)
	if voskModel == nil {
		return fmt.Errorf("falha ao carregar modelo Vosk: %s", modelPath)
	}

	return nil
}

// transcribeWithVosk transcreve áudio com Vosk
func transcribeWithVosk(audioData []byte, config VoskConfig) (string, error) {
	voskMutex.Lock()
	defer voskMutex.Unlock()

	if voskModel == nil {
		return "", fmt.Errorf("modelo Vosk não carregado")
	}

	// Cria recognizer
	recognizer := C.vosk_recognizer_new(voskModel, C.float(config.SampleRate))
	if recognizer == nil {
		return "", fmt.Errorf("falha ao criar recognizer Vosk")
	}
	defer C.vosk_recognizer_free(recognizer)

	// Processa áudio
	dataPtr := (*C.char)(unsafe.Pointer(&audioData[0]))
	C.vosk_recognizer_accept_waveform(recognizer, dataPtr, C.int(len(audioData)))

	// Obtém resultado final
	cResult := C.vosk_recognizer_final_result(recognizer)
	result := C.GoString(cResult)

	// Parse do JSON
	var parsed struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		return "", fmt.Errorf("erro ao parsear resultado: %w", err)
	}

	return parsed.Text, nil
}

// startVoskStreaming inicia streaming STT com Vosk
func startVoskStreaming(config VoskConfig, sConfig VoskStreamingConfig) error {
	voskMutex.Lock()
	defer voskMutex.Unlock()

	if voskModel == nil {
		return fmt.Errorf("modelo Vosk não carregado")
	}

	if voskStreaming {
		return fmt.Errorf("streaming já está ativo")
	}

	// Cria recognizer para streaming
	voskRecognizer = C.vosk_recognizer_new(voskModel, C.float(config.SampleRate))
	if voskRecognizer == nil {
		return fmt.Errorf("falha ao criar recognizer Vosk")
	}

	streamConfig = sConfig
	voskStreaming = true

	return nil
}

// processStreamingChunk processa um chunk de áudio durante streaming
func processStreamingChunk(audioChunk []byte) {
	voskMutex.Lock()
	defer voskMutex.Unlock()

	if !voskStreaming || voskRecognizer == nil {
		return
	}

	dataPtr := (*C.char)(unsafe.Pointer(&audioChunk[0]))
	accepted := C.vosk_recognizer_accept_waveform(voskRecognizer, dataPtr, C.int(len(audioChunk)))

	if accepted != 0 {
		// Resultado parcial disponível
		cResult := C.vosk_recognizer_partial_result(voskRecognizer)
		result := C.GoString(cResult)

		var parsed struct {
			Partial string `json:"partial"`
		}
		if err := json.Unmarshal([]byte(result), &parsed); err == nil {
			if streamConfig.OnPartialResult != nil && parsed.Partial != "" {
				streamConfig.OnPartialResult(parsed.Partial)
			}
		}
	}
}

// finalizeStreaming finaliza o streaming e retorna resultado final
func finalizeStreaming() string {
	voskMutex.Lock()
	defer voskMutex.Unlock()

	if !voskStreaming || voskRecognizer == nil {
		return ""
	}

	cResult := C.vosk_recognizer_final_result(voskRecognizer)
	result := C.GoString(cResult)

	var parsed struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err == nil {
		if streamConfig.OnFinalResult != nil && parsed.Text != "" {
			streamConfig.OnFinalResult(parsed.Text)
		}
		return parsed.Text
	}

	return ""
}

// stopVoskStreaming para streaming STT
func stopVoskStreaming() {
	voskMutex.Lock()
	defer voskMutex.Unlock()

	if voskRecognizer != nil {
		C.vosk_recognizer_free(voskRecognizer)
		voskRecognizer = nil
	}

	voskStreaming = false
	streamConfig = VoskStreamingConfig{}
}

// cleanupVosk libera recursos do Vosk
func cleanupVosk() {
	stopVoskStreaming()

	voskMutex.Lock()
	defer voskMutex.Unlock()

	if voskModel != nil {
		C.vosk_model_free(voskModel)
		voskModel = nil
	}
}
