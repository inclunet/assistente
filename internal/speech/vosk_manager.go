package speech

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

// VoskManager gerenciador do Vosk para wake word e STT offline
type VoskManager struct {
	config       VoskConfig
	modelsDir    string
	isEnabled    bool
	isListening  bool
	currentModel string

	// Callbacks
	onWakeWord func()
	onResult   func(text string)
	onPartial  func(text string)
	onError    func(err error)

	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

// NewVoskManager cria um novo gerenciador Vosk
func NewVoskManager(config VoskConfig) *VoskManager {
	// Define diretório de modelos
	homeDir, _ := os.UserHomeDir()
	modelsDir := filepath.Join(homeDir, ".assistente", "models", "vosk")

	return &VoskManager{
		config:    config,
		modelsDir: modelsDir,
		isEnabled: false,
	}
}

// IsAvailable verifica se Vosk está disponível
// Retorna true se o binding Vosk estiver compilado
func (vm *VoskManager) IsAvailable() bool {
	// TODO: Verificar se o binding Vosk está disponível
	// Isso requer compilação com CGO e a biblioteca Vosk
	return isVoskAvailable()
}

// IsModelInstalled verifica se um modelo está instalado
func (vm *VoskManager) IsModelInstalled(modelID string) bool {
	modelPath := filepath.Join(vm.modelsDir, modelID)
	info, err := os.Stat(modelPath)
	return err == nil && info.IsDir()
}

// GetInstalledModels retorna os modelos instalados
func (vm *VoskManager) GetInstalledModels() []VoskModelInfo {
	var installed []VoskModelInfo

	for _, model := range AvailableVoskModels {
		if vm.IsModelInstalled(model.ID) {
			model.IsInstalled = true
			model.LocalPath = filepath.Join(vm.modelsDir, model.ID)
			installed = append(installed, model)
		}
	}

	return installed
}

// GetAvailableModels retorna todos os modelos disponíveis
func (vm *VoskManager) GetAvailableModels() []VoskModelInfo {
	models := make([]VoskModelInfo, len(AvailableVoskModels))
	copy(models, AvailableVoskModels)

	for i := range models {
		models[i].IsInstalled = vm.IsModelInstalled(models[i].ID)
		if models[i].IsInstalled {
			models[i].LocalPath = filepath.Join(vm.modelsDir, models[i].ID)
		}
	}

	return models
}

// DownloadModel baixa um modelo do Vosk
func (vm *VoskManager) DownloadModel(modelID string, onProgress func(percent int)) error {
	// Encontra o modelo
	var model *VoskModelInfo
	for _, m := range AvailableVoskModels {
		if m.ID == modelID {
			model = &m
			break
		}
	}

	if model == nil {
		return fmt.Errorf("modelo não encontrado: %s", modelID)
	}

	// Cria diretório se não existir
	if err := os.MkdirAll(vm.modelsDir, 0755); err != nil {
		return fmt.Errorf("erro ao criar diretório: %w", err)
	}

	// Baixa o arquivo
	zipPath := filepath.Join(vm.modelsDir, modelID+".zip")
	if err := vm.downloadFile(model.DownloadURL, zipPath, model.Size, onProgress); err != nil {
		return fmt.Errorf("erro ao baixar modelo: %w", err)
	}

	// Extrai o arquivo
	if err := vm.unzip(zipPath, vm.modelsDir); err != nil {
		os.Remove(zipPath)
		return fmt.Errorf("erro ao extrair modelo: %w", err)
	}

	// Remove o arquivo zip
	os.Remove(zipPath)

	return nil
}

// downloadFile baixa um arquivo com progresso
func (vm *VoskManager) downloadFile(url, destPath string, expectedSize int64, onProgress func(int)) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status code: %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	var downloaded int64
	buf := make([]byte, 32*1024)

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			_, writeErr := out.Write(buf[:n])
			if writeErr != nil {
				return writeErr
			}
			downloaded += int64(n)

			if onProgress != nil && expectedSize > 0 {
				percent := int(float64(downloaded) / float64(expectedSize) * 100)
				onProgress(percent)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	return nil
}

// unzip extrai um arquivo zip
func (vm *VoskManager) unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)

		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}
	}

	return nil
}

// DeleteModel remove um modelo instalado
func (vm *VoskManager) DeleteModel(modelID string) error {
	modelPath := filepath.Join(vm.modelsDir, modelID)
	if !vm.IsModelInstalled(modelID) {
		return fmt.Errorf("modelo não instalado: %s", modelID)
	}
	return os.RemoveAll(modelPath)
}

// LoadModel carrega um modelo para uso
func (vm *VoskManager) LoadModel(modelID string) error {
	if !vm.IsAvailable() {
		return fmt.Errorf("Vosk não está disponível (requer compilação com CGO)")
	}

	if !vm.IsModelInstalled(modelID) {
		return fmt.Errorf("modelo não instalado: %s", modelID)
	}

	vm.mu.Lock()
	defer vm.mu.Unlock()

	modelPath := filepath.Join(vm.modelsDir, modelID)
	vm.config.ModelPath = modelPath
	vm.currentModel = modelID

	return loadVoskModel(modelPath)
}

// StartWakeWordListening inicia escuta de wake word
func (vm *VoskManager) StartWakeWordListening(keyword string, onWakeWord func()) error {
	if !vm.IsAvailable() {
		return fmt.Errorf("Vosk não está disponível")
	}

	vm.mu.Lock()
	defer vm.mu.Unlock()

	if vm.isListening {
		return nil
	}

	vm.onWakeWord = onWakeWord
	vm.ctx, vm.cancel = context.WithCancel(context.Background())

	// Inicia escuta em goroutine
	go vm.wakeWordLoop(keyword)

	vm.isListening = true
	return nil
}

// StopWakeWordListening para a escuta de wake word
func (vm *VoskManager) StopWakeWordListening() {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	if !vm.isListening {
		return
	}

	if vm.cancel != nil {
		vm.cancel()
	}

	vm.isListening = false
}

// wakeWordLoop loop de detecção de wake word
func (vm *VoskManager) wakeWordLoop(keyword string) {
	// TODO: Implementar loop de detecção real com Vosk
	// Isso requer:
	// 1. Abrir stream de áudio do microfone
	// 2. Enviar chunks para o Vosk
	// 3. Verificar se o texto transcrito contém a keyword
	// 4. Chamar onWakeWord quando detectado

	<-vm.ctx.Done()
}

// TranscribeAudio transcreve áudio usando Vosk
func (vm *VoskManager) TranscribeAudio(audioData []byte) (*TranscriptionResult, error) {
	if !vm.IsAvailable() {
		return nil, fmt.Errorf("Vosk não está disponível")
	}

	text, err := transcribeWithVosk(audioData, vm.config)
	if err != nil {
		return nil, err
	}

	return &TranscriptionResult{
		Text:     text,
		Provider: "vosk",
	}, nil
}

// StartStreamingSTT inicia transcrição em streaming
func (vm *VoskManager) StartStreamingSTT(config VoskStreamingConfig) error {
	if !vm.IsAvailable() {
		return fmt.Errorf("Vosk não está disponível")
	}

	vm.mu.Lock()
	defer vm.mu.Unlock()

	vm.onPartial = config.OnPartialResult
	vm.onResult = config.OnFinalResult
	vm.onError = config.OnError

	return startVoskStreaming(vm.config, config)
}

// StopStreamingSTT para a transcrição em streaming
func (vm *VoskManager) StopStreamingSTT() {
	stopVoskStreaming()
}

// IsListening retorna se está ouvindo wake word
func (vm *VoskManager) IsListening() bool {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	return vm.isListening
}

// GetCurrentModel retorna o modelo atual
func (vm *VoskManager) GetCurrentModel() string {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	return vm.currentModel
}

// Cleanup libera recursos
func (vm *VoskManager) Cleanup() {
	vm.StopWakeWordListening()
	vm.StopStreamingSTT()
	cleanupVosk()
}
