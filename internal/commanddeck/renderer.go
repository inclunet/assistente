// Package commanddeck contém a apresentação testável de dispositivos tipo
// Stream Deck. Ele não abre HID nem registra callbacks físicos; essa borda fica
// para o adapter real. O renderer decide quais teclas precisam ser reenviadas e
// quando um handle novo exige frame completo.
package commanddeck

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

var (
	ErrInvalidModel  = errors.New("modelo de deck inválido")
	ErrInvalidDevice = errors.New("dispositivo de deck inválido")
	ErrInvalidFrame  = errors.New("frame de deck inválido")
)

// Model descreve a geometria necessária para validar posições e cachear
// imagens no tamanho correto. A lista de modelos reais será preenchida pelo
// adapter HID depois da validação de biblioteca/licença/build.
type Model struct {
	ID          string
	Name        string
	Rows        int
	Columns     int
	KeyImageW   int
	KeyImageH   int
	SupportsHID bool
}

func (m Model) KeyCount() int { return m.Rows * m.Columns }

func (m Model) validate() error {
	if !validText(m.ID) || !validText(m.Name) || m.Rows <= 0 || m.Columns <= 0 || m.KeyImageW <= 0 || m.KeyImageH <= 0 {
		return ErrInvalidModel
	}
	return nil
}

// DeviceID é a identidade estável da instância física, distinta do modelo.
type DeviceID string

// KeyView é o estado visual efetivo de uma tecla.
type KeyView struct {
	Title     string
	ImageID   string
	ImageRGBA []byte
	State     string
	Announce  string
}

// Frame é a fotografia desejada para um dispositivo inteiro.
type Frame struct {
	Device DeviceID
	Model  Model
	Keys   map[int]KeyView
}

// KeyUpdate é o envio necessário para uma tecla.
type KeyUpdate struct {
	Index     int
	View      KeyView
	ImageHash string
}

// RenderPlan contém o diff para um dispositivo. FullFrame indica que o estado
// anterior foi invalidado e o adapter físico deve reenviar todas as teclas.
type RenderPlan struct {
	Device    DeviceID
	FullFrame bool
	Updates   []KeyUpdate
}

type Renderer struct {
	mu      sync.Mutex
	devices map[DeviceID]deviceState
	images  map[imageKey]string
}

type deviceState struct {
	model       Model
	rendered    map[int]string
	forceFull   bool
	handleEpoch uint64
}

type imageKey struct {
	modelID string
	imageID string
	width   int
	height  int
	sum     string
}

func NewRenderer() *Renderer {
	return &Renderer{devices: make(map[DeviceID]deviceState), images: make(map[imageKey]string)}
}

// OpenDevice registra ou reabre um handle. Toda abertura/reconexão invalida o
// diff daquele dispositivo, forçando o próximo Render a emitir frame completo.
func (r *Renderer) OpenDevice(device DeviceID, model Model) error {
	if r == nil || !validText(string(device)) {
		return ErrInvalidDevice
	}
	if err := model.validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	state := r.devices[device]
	state.model = model
	state.rendered = make(map[int]string)
	state.forceFull = true
	state.handleEpoch++
	r.devices[device] = state
	return nil
}

// InvalidateDevice força frame completo sem esquecer o modelo.
func (r *Renderer) InvalidateDevice(device DeviceID) error {
	if r == nil || !validText(string(device)) {
		return ErrInvalidDevice
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	state, ok := r.devices[device]
	if !ok {
		return ErrInvalidDevice
	}
	state.rendered = make(map[int]string)
	state.forceFull = true
	state.handleEpoch++
	r.devices[device] = state
	return nil
}

// RemoveDevice esquece estado em memória e cache dependente da identidade
// física. Não remove persistência de usuário.
func (r *Renderer) RemoveDevice(device DeviceID) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.devices, device)
}

// Render compara a fotografia desejada com a última fotografia enviada. Em um
// handle recém-aberto/reconectado, emite todas as posições do modelo, inclusive
// teclas vazias, para garantir estado seguro antes de voltar ao diff incremental.
func (r *Renderer) Render(frame Frame) (RenderPlan, error) {
	if r == nil || !validText(string(frame.Device)) {
		return RenderPlan{}, ErrInvalidDevice
	}
	if err := frame.Model.validate(); err != nil {
		return RenderPlan{}, err
	}
	if err := validateFrame(frame); err != nil {
		return RenderPlan{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	state, ok := r.devices[frame.Device]
	if !ok {
		return RenderPlan{}, ErrInvalidDevice
	}
	if state.model.ID != frame.Model.ID || state.model.KeyCount() != frame.Model.KeyCount() || state.model.KeyImageW != frame.Model.KeyImageW || state.model.KeyImageH != frame.Model.KeyImageH {
		return RenderPlan{}, ErrInvalidModel
	}
	plan := RenderPlan{Device: frame.Device, FullFrame: state.forceFull}
	indices := desiredIndices(frame.Model, state.forceFull, frame.Keys)
	for _, index := range indices {
		view := frame.Keys[index]
		hash := r.viewHash(frame.Model, view)
		if state.forceFull || state.rendered[index] != hash {
			plan.Updates = append(plan.Updates, KeyUpdate{Index: index, View: cloneKeyView(view), ImageHash: hash})
			state.rendered[index] = hash
		}
	}
	state.forceFull = false
	r.devices[frame.Device] = state
	return plan, nil
}

func validateFrame(frame Frame) error {
	count := frame.Model.KeyCount()
	for index, view := range frame.Keys {
		if index < 0 || index >= count {
			return fmt.Errorf("%w: índice %d fora de %d teclas", ErrInvalidFrame, index, count)
		}
		if strings.ContainsRune(view.Title, '\x00') || strings.ContainsRune(view.State, '\x00') || strings.ContainsRune(view.Announce, '\x00') || strings.ContainsRune(view.ImageID, '\x00') {
			return ErrInvalidFrame
		}
		if view.ImageID == "" && len(view.ImageRGBA) > 0 {
			return ErrInvalidFrame
		}
	}
	return nil
}

func desiredIndices(model Model, full bool, keys map[int]KeyView) []int {
	if full {
		values := make([]int, model.KeyCount())
		for i := range values {
			values[i] = i
		}
		return values
	}
	values := make([]int, 0, len(keys))
	for index := range keys {
		values = append(values, index)
	}
	sort.Ints(values)
	return values
}

func (r *Renderer) viewHash(model Model, view KeyView) string {
	imageHash := r.imageHash(model, view)
	h := sha256.New()
	_, _ = h.Write([]byte(view.Title))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(view.State))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(view.Announce))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(imageHash))
	return hex.EncodeToString(h.Sum(nil))
}

func (r *Renderer) imageHash(model Model, view KeyView) string {
	if view.ImageID == "" {
		return ""
	}
	sumBytes := sha256.Sum256(view.ImageRGBA)
	sum := hex.EncodeToString(sumBytes[:])
	key := imageKey{modelID: model.ID, imageID: view.ImageID, width: model.KeyImageW, height: model.KeyImageH, sum: sum}
	if cached, ok := r.images[key]; ok {
		return cached
	}
	valueBytes := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d\x00%d\x00%s", key.modelID, key.imageID, key.width, key.height, key.sum)))
	value := hex.EncodeToString(valueBytes[:])
	r.images[key] = value
	return value
}

func cloneKeyView(view KeyView) KeyView {
	clone := view
	if view.ImageRGBA != nil {
		clone.ImageRGBA = append([]byte(nil), view.ImageRGBA...)
	}
	return clone
}

func validText(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && !strings.ContainsRune(value, '\x00')
}
