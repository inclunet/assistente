//go:build !vosk
// +build !vosk

package speech

// Este arquivo contém stubs para quando Vosk não está habilitado.
// Para habilitar Vosk, compile com: go build -tags vosk
//
// Requisitos para compilar com Vosk:
// 1. Instalar a biblioteca Vosk (https://alphacephei.com/vosk/)
// 2. Instalar o binding Go (go get github.com/alphacep/vosk-api/go)
// 3. Compilar com a tag: go build -tags vosk

import (
	"fmt"
)

// isVoskAvailable retorna se Vosk está disponível
func isVoskAvailable() bool {
	return false
}

// loadVoskModel carrega um modelo Vosk
func loadVoskModel(modelPath string) error {
	return fmt.Errorf("Vosk não está habilitado. Compile com: go build -tags vosk")
}

// transcribeWithVosk transcreve áudio com Vosk
func transcribeWithVosk(audioData []byte, config VoskConfig) (string, error) {
	return "", fmt.Errorf("Vosk não está habilitado")
}

// startVoskStreaming inicia streaming STT com Vosk
func startVoskStreaming(config VoskConfig, streamConfig VoskStreamingConfig) error {
	return fmt.Errorf("Vosk não está habilitado")
}

// stopVoskStreaming para streaming STT
func stopVoskStreaming() {
	// No-op
}

// cleanupVosk libera recursos do Vosk
func cleanupVosk() {
	// No-op
}
