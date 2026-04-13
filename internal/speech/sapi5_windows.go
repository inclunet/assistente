//go:build windows
// +build windows

package speech

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// Voice representa uma voz SAPI5
type Voice struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Language    string `json:"language"`
	Gender      string `json:"gender"`
	Age         string `json:"age"`
	Vendor      string `json:"vendor"`
	Description string `json:"description"`
	Source      string `json:"source"` // "sapi5"
}

// SAPI5Manager gerencia a síntese de voz via SAPI5 usando COM
type SAPI5Manager struct {
	mu          sync.Mutex
	initialized bool
	voices      []Voice
	spVoice     *ole.IDispatch // Instância persistente do SpVoice
}

var (
	manager     *SAPI5Manager
	managerOnce sync.Once
)

// GetSAPI5Manager retorna a instância singleton do manager
func GetSAPI5Manager() *SAPI5Manager {
	managerOnce.Do(func() {
		manager = &SAPI5Manager{}
	})
	return manager
}

// initialize inicializa o COM e carrega as vozes disponíveis.
// Deve ser chamado com m.mu já adquirido.
func (m *SAPI5Manager) initialize() error {
	if m.initialized {
		return nil
	}

	// Inicializa COM (pode ser chamado múltiplas vezes com segurança)
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		// Ignora erro se já inicializado
		oleErr, ok := err.(*ole.OleError)
		if !ok || (oleErr.Code() != 0x00000001 && oleErr.Code() != 0x80010106) {
			// 0x00000001 = S_FALSE (já inicializado)
			// 0x80010106 = RPC_E_CHANGED_MODE (modo diferente, mas OK)
			return fmt.Errorf("CoInitializeEx failed: %w", err)
		}
	}

	// Cria instância do SpVoice
	unknown, err := oleutil.CreateObject("SAPI.SpVoice")
	if err != nil {
		return fmt.Errorf("failed to create SpVoice: %w", err)
	}

	spVoice, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return fmt.Errorf("failed to query IDispatch: %w", err)
	}
	m.spVoice = spVoice

	// Carrega vozes disponíveis
	if err := m.loadVoices(); err != nil {
		return fmt.Errorf("failed to load voices: %w", err)
	}

	m.initialized = true
	return nil
}

// loadVoices carrega todas as vozes SAPI5 instaladas
func (m *SAPI5Manager) loadVoices() error {
	if m.spVoice == nil {
		return fmt.Errorf("SpVoice not initialized")
	}

	// Obtém a coleção de vozes
	voicesResult, err := oleutil.CallMethod(m.spVoice, "GetVoices")
	if err != nil {
		return fmt.Errorf("GetVoices failed: %w", err)
	}
	voicesCollection := voicesResult.ToIDispatch()
	defer voicesCollection.Release()

	// Obtém o número de vozes
	countResult, err := oleutil.GetProperty(voicesCollection, "Count")
	if err != nil {
		return fmt.Errorf("failed to get voice count: %w", err)
	}
	count := int(countResult.Val)

	m.voices = make([]Voice, 0, count)

	// Itera sobre as vozes
	for i := 0; i < count; i++ {
		itemResult, err := oleutil.CallMethod(voicesCollection, "Item", i)
		if err != nil {
			continue
		}
		voiceToken := itemResult.ToIDispatch()

		voice := m.extractVoiceInfo(voiceToken)
		if voice.Name != "" {
			m.voices = append(m.voices, voice)
		}

		voiceToken.Release()
	}

	return nil
}

// extractVoiceInfo extrai informações de um token de voz
func (m *SAPI5Manager) extractVoiceInfo(voiceToken *ole.IDispatch) Voice {
	voice := Voice{Source: "sapi5"}

	// Obtém o ID
	if idResult, err := oleutil.GetProperty(voiceToken, "Id"); err == nil {
		voice.ID = idResult.ToString()
	}

	// Obtém os atributos
	if attrsResult, err := oleutil.CallMethod(voiceToken, "GetAttribute", "Name"); err == nil {
		voice.Name = attrsResult.ToString()
	}

	if attrsResult, err := oleutil.CallMethod(voiceToken, "GetAttribute", "Language"); err == nil {
		langCode := attrsResult.ToString()
		voice.Language = lcidToLanguage(langCode)
	}

	if attrsResult, err := oleutil.CallMethod(voiceToken, "GetAttribute", "Gender"); err == nil {
		voice.Gender = attrsResult.ToString()
	}

	if attrsResult, err := oleutil.CallMethod(voiceToken, "GetAttribute", "Age"); err == nil {
		voice.Age = attrsResult.ToString()
	}

	if attrsResult, err := oleutil.CallMethod(voiceToken, "GetAttribute", "Vendor"); err == nil {
		voice.Vendor = attrsResult.ToString()
	}

	// Monta descrição
	voice.Description = fmt.Sprintf("%s (%s)", voice.Name, voice.Language)

	return voice
}

// lcidToLanguage converte código LCID hexadecimal para código de idioma
func lcidToLanguage(lcid string) string {
	// Mapeamento comum de LCIDs
	lcidMap := map[string]string{
		"409":  "en-US",
		"809":  "en-GB",
		"416":  "pt-BR",
		"816":  "pt-PT",
		"40A":  "es-ES",
		"80A":  "es-MX",
		"40C":  "fr-FR",
		"407":  "de-DE",
		"410":  "it-IT",
		"411":  "ja-JP",
		"412":  "ko-KR",
		"804":  "zh-CN",
		"404":  "zh-TW",
		"419":  "ru-RU",
		"41D":  "sv-SE",
		"406":  "da-DK",
		"413":  "nl-NL",
		"414":  "nb-NO",
		"40B":  "fi-FI",
		"408":  "el-GR",
		"40E":  "hu-HU",
		"415":  "pl-PL",
		"405":  "cs-CZ",
		"41F":  "tr-TR",
		"40D":  "he-IL",
		"401":  "ar-SA",
		"41E":  "th-TH",
		"42A":  "vi-VN",
		"421":  "id-ID",
		"41A":  "hr-HR",
		"424":  "sl-SI",
		"418":  "ro-RO",
		"41B":  "sk-SK",
		"422":  "uk-UA",
		"402":  "bg-BG",
		"403":  "ca-ES",
		"42D":  "eu-ES",
		"456":  "gl-ES",
		"C0A":  "es-ES", // Espanhol moderno
		"1009": "en-CA",
		"1409": "en-NZ",
		"C09":  "en-AU",
		"1809": "en-IE",
	}

	if lang, ok := lcidMap[lcid]; ok {
		return lang
	}
	return lcid // Retorna o código original se não encontrar
}

// GetVoices retorna a lista de vozes disponíveis
func (m *SAPI5Manager) GetVoices() []Voice {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.initialized {
		if err := m.initialize(); err != nil {
			log.Printf("[SAPI5] falha ao inicializar COM: %v", err)
			return []Voice{}
		}
	}

	return m.voices
}

// Speak sintetiza texto usando a voz padrão ou uma específica
func (m *SAPI5Manager) Speak(text string, voiceName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.initialized {
		if err := m.initialize(); err != nil {
			return err
		}
	}

	// Se um nome de voz foi especificado, seleciona a voz
	if voiceName != "" {
		if err := m.selectVoice(voiceName); err != nil {
			// Se falhar ao selecionar, continua com a voz padrão
			log.Printf("Warning: failed to select voice '%s': %v", voiceName, err)
		}
	}

	// SVSFlagsAsync = 1 (fala de forma assíncrona)
	// SVSFPurgeBeforeSpeak = 2 (limpa buffer antes)
	const SVSFlagsAsync = 1
	const SVSFPurgeBeforeSpeak = 2

	_, err := oleutil.CallMethod(m.spVoice, "Speak", text, SVSFlagsAsync|SVSFPurgeBeforeSpeak)
	if err != nil {
		return fmt.Errorf("Speak failed: %w", err)
	}

	return nil
}

// SynthesizeToBytes sintetiza texto via SAPI5 e retorna bytes WAV.
// Redireciona o output do SpVoice para um SpFileStream temporário,
// gerando um arquivo WAV que é lido e retornado como []byte.
func (m *SAPI5Manager) SynthesizeToBytes(text, voiceName string, rate, volume int) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.initialized {
		if err := m.initialize(); err != nil {
			return nil, err
		}
	}

	if voiceName != "" {
		if err := m.selectVoice(voiceName); err != nil {
			log.Printf("[SAPI5] Warning: failed to select voice '%s': %v", voiceName, err)
		}
	}

	// Configura rate e volume (best-effort: loga erro mas continua)
	if rate >= -10 && rate <= 10 {
		if v, err := oleutil.PutProperty(m.spVoice, "Rate", rate); err != nil {
			log.Printf("[SAPI5] Warning: failed to set Rate: %v", err)
		} else if v != nil {
			v.Clear()
		}
	}
	if volume >= 0 && volume <= 100 {
		if v, err := oleutil.PutProperty(m.spVoice, "Volume", volume); err != nil {
			log.Printf("[SAPI5] Warning: failed to set Volume: %v", err)
		} else if v != nil {
			v.Clear()
		}
	}

	// Cria arquivo temporário para saída WAV
	tmpFile, err := os.CreateTemp("", "sapi5-*.wav")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpFile.Close()

	// Converte path para absoluto (COM precisa de path completo)
	absPath, err := filepath.Abs(tmpFile.Name())
	if err != nil {
		os.Remove(tmpFile.Name())
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}
	defer os.Remove(absPath)

	// Salva referência ao output padrão antes de redirecionar
	defaultOutputResult, err := oleutil.GetProperty(m.spVoice, "AudioOutputStream")
	if err != nil {
		return nil, fmt.Errorf("failed to get default AudioOutputStream: %w", err)
	}
	var defaultOutput *ole.IDispatch
	if defaultOutputResult.VT != ole.VT_EMPTY && defaultOutputResult.VT != ole.VT_NULL {
		defaultOutput = defaultOutputResult.ToIDispatch()
		// AddRef: garante que o IDispatch sobrevive ao Clear() do VARIANT
		defaultOutput.AddRef()
	}
	defaultOutputResult.Clear()

	// Garante Release() do defaultOutput e restauração do AudioOutputStream
	// em todos os caminhos de retorno (inclusive erros antes do Speak).
	redirected := false
	defer func() {
		if redirected {
			if defaultOutput != nil {
				if v, err := oleutil.PutPropertyRef(m.spVoice, "AudioOutputStream", defaultOutput); err != nil {
					log.Printf("[SAPI5] Warning: failed to restore AudioOutputStream: %v", err)
				} else if v != nil {
					v.Clear()
				}
			} else {
				m.restoreDefaultOutput()
			}
		}
		if defaultOutput != nil {
			defaultOutput.Release()
		}
	}()

	// Cria SpFileStream COM object
	fileStreamUnknown, err := oleutil.CreateObject("SAPI.SpFileStream")
	if err != nil {
		return nil, fmt.Errorf("failed to create SpFileStream: %w", err)
	}
	defer fileStreamUnknown.Release()
	fileStream, err := fileStreamUnknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return nil, fmt.Errorf("failed to query SpFileStream IDispatch: %w", err)
	}
	defer fileStream.Release()

	// Abre o arquivo para escrita (SSFMCreateForWrite = 3)
	const SSFMCreateForWrite = 3
	if v, err := oleutil.CallMethod(fileStream, "Open", absPath, SSFMCreateForWrite, false); err != nil {
		return nil, fmt.Errorf("SpFileStream.Open failed: %w", err)
	} else if v != nil {
		v.Clear()
	}

	// Redireciona SpVoice para escrever no arquivo
	if v, err := oleutil.PutPropertyRef(m.spVoice, "AudioOutputStream", fileStream); err != nil {
		if cv, _ := oleutil.CallMethod(fileStream, "Close"); cv != nil {
			cv.Clear()
		}
		return nil, fmt.Errorf("failed to redirect AudioOutputStream: %w", err)
	} else if v != nil {
		v.Clear()
	}
	redirected = true

	// Fala síncrona — escreve WAV no arquivo
	speakResult, speakErr := oleutil.CallMethod(m.spVoice, "Speak", text, 0) // 0 = síncrono
	if speakResult != nil {
		speakResult.Clear()
	}

	// Fecha o stream de arquivo
	if v, _ := oleutil.CallMethod(fileStream, "Close"); v != nil {
		v.Clear()
	}

	// Nota: restauração do output e Release do defaultOutput são
	// feitos pelo defer acima, cobrindo inclusive caminhos de erro.

	if speakErr != nil {
		return nil, fmt.Errorf("SAPI5 Speak (to file) failed: %w", speakErr)
	}

	// Lê o arquivo WAV gerado
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read WAV file: %w", err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("SAPI5 generated empty WAV")
	}

	return data, nil
}

// restoreDefaultOutput restaura a conexão com o output de áudio padrão.
// Seta AudioOutputStream para nil (VT_EMPTY), o que faz o SpVoice
// voltar a usar o dispositivo de áudio padrão do sistema.
func (m *SAPI5Manager) restoreDefaultOutput() {
	if v, err := oleutil.PutProperty(m.spVoice, "AudioOutputStream", nil); err != nil {
		log.Printf("[SAPI5] Warning: failed to restore default output: %v", err)
	} else if v != nil {
		v.Clear()
	}
}

// selectVoice seleciona uma voz pelo nome
func (m *SAPI5Manager) selectVoice(voiceName string) error {
	// Obtém a coleção de vozes
	voicesResult, err := oleutil.CallMethod(m.spVoice, "GetVoices")
	if err != nil {
		return err
	}
	voicesCollection := voicesResult.ToIDispatch()
	defer voicesCollection.Release()

	// Obtém o número de vozes
	countResult, err := oleutil.GetProperty(voicesCollection, "Count")
	if err != nil {
		return err
	}
	count := int(countResult.Val)

	// Procura a voz pelo nome ou ID
	for i := 0; i < count; i++ {
		itemResult, err := oleutil.CallMethod(voicesCollection, "Item", i)
		if err != nil {
			continue
		}
		voiceToken := itemResult.ToIDispatch()

		// Tenta match por Name
		if nameResult, err := oleutil.CallMethod(voiceToken, "GetAttribute", "Name"); err == nil {
			if nameResult.ToString() == voiceName {
				_, err := oleutil.PutPropertyRef(m.spVoice, "Voice", voiceToken)
				voiceToken.Release()
				return err
			}
		}

		// Fallback: tenta match por ID (registry path)
		if idResult, err := oleutil.GetProperty(voiceToken, "Id"); err == nil {
			if idResult.ToString() == voiceName {
				_, err := oleutil.PutPropertyRef(m.spVoice, "Voice", voiceToken)
				voiceToken.Release()
				return err
			}
		}

		voiceToken.Release()
	}

	return fmt.Errorf("voice '%s' not found", voiceName)
}

// Stop para a síntese atual
func (m *SAPI5Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.initialized || m.spVoice == nil {
		return nil
	}

	// SVSFPurgeBeforeSpeak limpa o buffer e para a síntese
	_, err := oleutil.CallMethod(m.spVoice, "Speak", "", 2) // 2 = SVSFPurgeBeforeSpeak
	return err
}

// SetVolume define o volume (0-100)
func (m *SAPI5Manager) SetVolume(volume int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.initialized || m.spVoice == nil {
		return fmt.Errorf("not initialized")
	}

	if volume < 0 {
		volume = 0
	}
	if volume > 100 {
		volume = 100
	}

	_, err := oleutil.PutProperty(m.spVoice, "Volume", volume)
	return err
}

// SetRate define a velocidade (-10 a 10, 0 é normal)
func (m *SAPI5Manager) SetRate(rate int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.initialized || m.spVoice == nil {
		return fmt.Errorf("not initialized")
	}

	if rate < -10 {
		rate = -10
	}
	if rate > 10 {
		rate = 10
	}

	_, err := oleutil.PutProperty(m.spVoice, "Rate", rate)
	return err
}

// IsSpeaking verifica se está falando
func (m *SAPI5Manager) IsSpeaking() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.initialized || m.spVoice == nil {
		return false
	}

	// Status.RunningState: 0=done, 1=waiting, 2=speaking
	statusResult, err := oleutil.GetProperty(m.spVoice, "Status")
	if err != nil {
		return false
	}
	status := statusResult.ToIDispatch()
	defer status.Release()

	runningResult, err := oleutil.GetProperty(status, "RunningState")
	if err != nil {
		return false
	}

	return runningResult.Val == 2 // SRSEIsSpeaking
}

// WaitUntilDone aguarda a síntese terminar
func (m *SAPI5Manager) WaitUntilDone(timeoutMs int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.initialized || m.spVoice == nil {
		return true
	}

	result, err := oleutil.CallMethod(m.spVoice, "WaitUntilDone", timeoutMs)
	if err != nil {
		return true
	}

	return result.Val != 0 // true se terminou, false se timeout
}

// Cleanup libera recursos
func (m *SAPI5Manager) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.spVoice != nil {
		m.spVoice.Release()
		m.spVoice = nil
	}

	if m.initialized {
		ole.CoUninitialize()
		m.initialized = false
	}
}

// StopSpeaking para a síntese atual (função global para compatibilidade)
func StopSpeaking() {
	GetSAPI5Manager().Stop()
}
