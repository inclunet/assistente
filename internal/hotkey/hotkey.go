// Package hotkey fornece hotkeys globais com ownership e no-repeat no Windows.
// Outras plataformas recusam registro até possuírem um adapter equivalente.
package hotkey

import (
	"assistente/internal/logging"
	"context"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.design/x/hotkey"
)

// Teclas comuns - removidas as re-exportações para evitar erros de compilação cross-platform
// As teclas são usadas diretamente através do pacote hotkey onde necessário

// HotkeyCallback função chamada quando hotkey é pressionado
type HotkeyCallback func()

// ErrCombinationAlreadyOwned indica que a combinação já está reservada por
// outra inscrição, inclusive enquanto a inscrição anterior ainda está sendo
// removida nativamente.
var ErrCombinationAlreadyOwned = errors.New("hotkey combination already owned")

// NativeCombination é a identidade de uma combinação no espaço nativo da
// plataforma. Modifiers contém a máscara nativa combinada; não é uma máscara
// DOM e não deve ser convertida em KeyboardEvent.code.
type NativeCombination struct {
	Modifiers hotkey.Modifier
	Key       hotkey.Key
}

// NativeOwnershipSnapshot é uma cópia thread-safe das combinações atualmente
// reservadas pelo Manager. O conteúdo é nativo por desenho: em particular,
// KeyA pode ter significado físico diferente conforme o layout do sistema.
type NativeOwnershipSnapshot struct {
	Generation   uint64
	Combinations []NativeCombination
}

type nativeHotkey interface {
	Register() error
	Unregister() error
	Keydown() <-chan hotkey.Event
}

type hotkeyFactory func([]hotkey.Modifier, hotkey.Key) nativeHotkey

// RegisteredHotkey representa um hotkey registrado
type RegisteredHotkey struct {
	ID                int
	Modifiers         []hotkey.Modifier
	Key               hotkey.Key
	Callback          HotkeyCallback
	down              <-chan hotkey.Event
	active            atomic.Bool
	unregisterDone    chan struct{}
	unregistering     bool
	unregisterErr     error
	unregisterDoneSet bool
	removed           bool
	slot              *hotkeySlot
}

type nativeCombination struct {
	modifiers hotkey.Modifier
	key       hotkey.Key
}

// hotkeySlot is the single native registration for one combination. Logical
// registrations can outlive a capture while a temporary, higher-priority
// reservation owns the slot.
type hotkeySlot struct {
	combination   nativeCombination
	registrations map[int]*RegisteredHotkey
	temporary     *temporaryReservation
	capture       *hotkeyCapture
	tearingDown   bool
}

type hotkeyCapture struct {
	binding  *RegisteredHotkey
	listener *RegisteredHotkey
	native   nativeHotkey
	ctx      context.Context
	cancel   context.CancelFunc
}

type temporaryReservation struct {
	slot    *hotkeySlot
	binding *RegisteredHotkey
	once    sync.Once
	err     error
}

// Manager gerencia hotkeys globais
type Manager struct {
	hotkeys             map[int]*RegisteredHotkey
	slots               map[nativeCombination]*hotkeySlot
	nextID              int
	generation          uint64
	registrationStarted bool
	ownershipBarrier    *OwnershipBarrier
	ownershipBarrierSet bool
	mu                  sync.RWMutex
	teardownMu          sync.Mutex
	factory             hotkeyFactory
}

// singleton
var (
	globalManager *Manager
	managerOnce   sync.Once
)

// GetManager retorna o manager singleton
func GetManager() *Manager {
	managerOnce.Do(func() {
		globalManager = newManager(newNativeHotkey)
	})
	return globalManager
}

func newManager(factory hotkeyFactory) *Manager {
	if factory == nil {
		factory = newNativeHotkey
	}
	return &Manager{
		hotkeys: make(map[int]*RegisteredHotkey),
		slots:   make(map[nativeCombination]*hotkeySlot),
		nextID:  1,
		factory: factory,
	}
}

func canonicalModifiers(modifiers []hotkey.Modifier) ([]hotkey.Modifier, hotkey.Modifier) {
	if len(modifiers) == 0 {
		return nil, 0
	}

	seen := make(map[hotkey.Modifier]struct{}, len(modifiers))
	var mask hotkey.Modifier
	for _, modifier := range modifiers {
		mask |= modifier
		seen[modifier] = struct{}{}
	}
	canonical := make([]hotkey.Modifier, 0, len(seen))
	for modifier := range seen {
		canonical = append(canonical, modifier)
	}
	sort.Slice(canonical, func(i, j int) bool { return canonical[i] < canonical[j] })
	return canonical, mask
}

func combinationDescription(combination nativeCombination) string {
	return fmt.Sprintf("modifiers=%#x key=%#x", uint32(combination.modifiers), uint32(combination.key))
}

const ownershipPublishTimeout = 10 * time.Second

var (
	ErrOwnershipBarrierUnsupported = errors.New("hotkey ownership barrier requires Windows")
	ErrOwnershipBarrierNil         = errors.New("hotkey ownership barrier is nil")
	ErrOwnershipBarrierConfigured  = errors.New("hotkey ownership barrier already configured")
)

// SetOwnershipBarrier instala a barreira opcional que sincroniza o ownership
// nativo com o DOM. Ela deve ser configurada antes da primeira tentativa de
// registro e só pode ser configurada uma vez. A barreira é deliberadamente
// Windows-only: as outras plataformas não têm, neste percurso, o contrato
// nativo necessário para publicar essa reserva global.
func (m *Manager) SetOwnershipBarrier(barrier *OwnershipBarrier) error {
	if runtime.GOOS != "windows" {
		return ErrOwnershipBarrierUnsupported
	}
	if barrier == nil {
		return ErrOwnershipBarrierNil
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ownershipBarrierSet || m.registrationStarted {
		return ErrOwnershipBarrierConfigured
	}
	m.ownershipBarrier = barrier
	m.ownershipBarrierSet = true
	return nil
}

func (m *Manager) ownershipCombinationsLocked(extra *nativeCombination) []OwnershipCombination {
	combinations := make([]OwnershipCombination, 0, len(m.slots)+1)
	seen := make(map[nativeCombination]struct{}, len(m.slots)+1)
	for combination := range m.slots {
		combinations = append(combinations, OwnershipCombination{
			Key:       uint32(combination.key),
			Modifiers: uint32(combination.modifiers),
		})
		seen[combination] = struct{}{}
	}
	if extra != nil {
		if _, exists := seen[*extra]; exists {
			return combinations
		}
		combinations = append(combinations, OwnershipCombination{
			Key:       uint32(extra.key),
			Modifiers: uint32(extra.modifiers),
		})
	}
	sort.Slice(combinations, func(i, j int) bool {
		if combinations[i].Key != combinations[j].Key {
			return combinations[i].Key < combinations[j].Key
		}
		return combinations[i].Modifiers < combinations[j].Modifiers
	})
	return combinations
}

func (m *Manager) ensureMapsLocked() {
	if m.hotkeys == nil {
		m.hotkeys = make(map[int]*RegisteredHotkey)
	}
	if m.slots == nil {
		m.slots = make(map[nativeCombination]*hotkeySlot)
	}
	if m.factory == nil {
		m.factory = newNativeHotkey
	}
}

func (m *Manager) nextRegistrationIDLocked() int {
	if m.nextID == 0 {
		m.nextID = 1
	}
	id := m.nextID
	m.nextID++
	return id
}

func validWindowsCombination(barrier *OwnershipBarrier, modifierMask hotkey.Modifier, key hotkey.Key) bool {
	return barrier == nil || (uint32(key) != 0 && uint32(key) <= 0xFE && uint32(modifierMask)&^uint32(0x0F) == 0)
}

func (m *Manager) acquireCaptureLocked(binding *RegisteredHotkey) (*hotkeyCapture, error) {
	hk := m.factory(binding.Modifiers, binding.Key)
	if hk == nil {
		return nil, fmt.Errorf("hotkey factory returned nil")
	}
	if err := hk.Register(); err != nil {
		return nil, fmt.Errorf("failed to register hotkey: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	listener := &RegisteredHotkey{
		ID:       binding.ID,
		Callback: binding.Callback,
		down:     hk.Keydown(),
	}
	listener.active.Store(true)
	return &hotkeyCapture{
		binding:  binding,
		listener: listener,
		native:   hk,
		ctx:      ctx,
		cancel:   cancel,
	}, nil
}

func stopCapture(capture *hotkeyCapture) {
	if capture == nil {
		return
	}
	capture.listener.active.Store(false)
	capture.cancel()
}

func startCaptureListener(capture *hotkeyCapture) {
	if capture == nil {
		return
	}
	go listenHotkey(capture.ctx, capture.listener)
}

func (m *Manager) chooseRegistrationLocked(slot *hotkeySlot) *RegisteredHotkey {
	if slot == nil {
		return nil
	}
	var selected *RegisteredHotkey
	for _, registration := range slot.registrations {
		if registration.unregistering || registration.removed {
			continue
		}
		if selected == nil || registration.ID < selected.ID {
			selected = registration
		}
	}
	return selected
}

// publishOwnershipLocked aguarda o ACK da publicação sem expor o mutex do
// Manager ao publisher. O manager serializa a proposta enquanto aguarda para
// que nenhum registro concorrente publique uma geração intermediária.
func (m *Manager) publishOwnershipLocked(combinations []OwnershipCombination) error {
	if m.ownershipBarrier == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), ownershipPublishTimeout)
	defer cancel()
	return m.ownershipBarrier.Publish(ctx, combinations)
}

// Register registra um novo hotkey global
func (m *Manager) Register(modifiers []hotkey.Modifier, key hotkey.Key, callback HotkeyCallback) (int, error) {
	if callback == nil {
		return 0, fmt.Errorf("hotkey callback is nil")
	}
	canonical, modifierMask := canonicalModifiers(modifiers)
	ownership := nativeCombination{modifiers: modifierMask, key: key}

	m.mu.Lock()
	m.registrationStarted = true
	m.ensureMapsLocked()
	if m.slots[ownership] != nil && (m.slots[ownership].temporary == nil || m.slots[ownership].tearingDown) {
		m.mu.Unlock()
		return 0, fmt.Errorf("%w: %s", ErrCombinationAlreadyOwned, combinationDescription(ownership))
	}
	if !validWindowsCombination(m.ownershipBarrier, modifierMask, key) {
		m.mu.Unlock()
		return 0, fmt.Errorf("invalid Windows hotkey combination: modifiers=%#x key=%#x", uint32(modifierMask), uint32(key))
	}
	id := m.nextRegistrationIDLocked()
	registered := &RegisteredHotkey{
		ID:             id,
		Modifiers:      append([]hotkey.Modifier(nil), canonical...),
		Key:            key,
		Callback:       callback,
		unregisterDone: make(chan struct{}),
	}
	if slot := m.slots[ownership]; slot != nil {
		if len(slot.registrations) != 0 {
			m.mu.Unlock()
			return 0, fmt.Errorf("%w: %s", ErrCombinationAlreadyOwned, combinationDescription(ownership))
		}
		slot.registrations[id] = registered
		registered.slot = slot
		m.hotkeys[id] = registered
		m.mu.Unlock()
		return id, nil
	}
	previousOwnership := m.ownershipCombinationsLocked(nil)
	slot := &hotkeySlot{combination: ownership, registrations: make(map[int]*RegisteredHotkey)}
	m.slots[ownership] = slot
	registered.slot = slot
	if err := m.publishOwnershipLocked(m.ownershipCombinationsLocked(nil)); err != nil {
		delete(m.slots, ownership)
		rollbackErr := m.publishOwnershipLocked(previousOwnership)
		m.mu.Unlock()
		return 0, errors.Join(fmt.Errorf("failed to publish hotkey ownership: %w", err), rollbackErr)
	}
	capture, err := m.acquireCaptureLocked(registered)
	if err != nil {
		delete(m.slots, ownership)
		rollbackErr := m.publishOwnershipLocked(previousOwnership)
		m.mu.Unlock()
		return 0, errors.Join(err, rollbackErr)
	}
	slot.registrations[id] = registered
	slot.capture = capture
	m.hotkeys[id] = registered
	m.generation++
	m.mu.Unlock()

	// O listener só começa depois de liberar manager.mu; nunca chama callback
	// sob o mutex do Manager.
	startCaptureListener(capture)

	logging.Infof(context.Background(), "hotkey.hotkey", "Hotkey registrado: ID=%d, Modifiers=%v, Key=%v", id, modifiers, key)
	return id, nil
}

// ReserveTemporary reserva uma combinação com prioridade sobre o registro
// configurável existente. O slot continua único no espaço nativo: a captura
// inferior é encerrada e recriada quando a reserva é liberada.
func (m *Manager) ReserveTemporary(modifiers []hotkey.Modifier, key hotkey.Key, callback HotkeyCallback) (func() error, error) {
	if callback == nil {
		return nil, fmt.Errorf("hotkey callback is nil")
	}
	canonical, modifierMask := canonicalModifiers(modifiers)
	ownership := nativeCombination{modifiers: modifierMask, key: key}

	m.mu.Lock()
	m.registrationStarted = true
	m.ensureMapsLocked()
	if !validWindowsCombination(m.ownershipBarrier, modifierMask, key) {
		m.mu.Unlock()
		return nil, fmt.Errorf("invalid Windows hotkey combination: modifiers=%#x key=%#x", uint32(modifierMask), uint32(key))
	}
	slot := m.slots[ownership]
	if slot != nil && (slot.temporary != nil || slot.tearingDown) {
		m.mu.Unlock()
		return nil, fmt.Errorf("%w: %s", ErrCombinationAlreadyOwned, combinationDescription(ownership))
	}
	if slot != nil && slot.capture != nil && slot.capture.binding.unregistering {
		m.mu.Unlock()
		return nil, fmt.Errorf("%w: %s", ErrCombinationAlreadyOwned, combinationDescription(ownership))
	}
	slotCreated := slot == nil
	previousOwnership := m.ownershipCombinationsLocked(nil)
	if slot == nil {
		slot = &hotkeySlot{combination: ownership, registrations: make(map[int]*RegisteredHotkey)}
		m.slots[ownership] = slot
	}
	reservation := &temporaryReservation{
		slot:    slot,
		binding: &RegisteredHotkey{Modifiers: append([]hotkey.Modifier(nil), canonical...), Key: key, Callback: callback},
	}
	slot.temporary = reservation
	// A prioridade vale desde o pedido, inclusive enquanto aguardamos o ACK.
	// Em falha de transporte, não entregar a ocorrência ao comando inferior.
	stopCapture(slot.capture)
	// Mesmo quando o slot é novo, o ACK é a admissão imediatamente anterior
	// à aquisição da captura nativa temporária.
	if err := m.publishOwnershipLocked(m.ownershipCombinationsLocked(nil)); err != nil {
		slot.temporary = nil
		if slotCreated {
			delete(m.slots, ownership)
		}
		rollbackErr := error(nil)
		if slotCreated {
			rollbackErr = m.publishOwnershipLocked(previousOwnership)
		}
		m.mu.Unlock()
		return nil, errors.Join(fmt.Errorf("failed to publish temporary hotkey ownership: %w", err), rollbackErr)
	}
	if slot.capture != nil {
		lower := slot.capture
		stopCapture(lower)
		if err := lower.native.Unregister(); err != nil {
			// Sem prova de remoção, a captura antiga continua sendo o owner
			// conservador; nenhum callback inferior é reativado.
			slot.temporary = nil
			m.mu.Unlock()
			return nil, fmt.Errorf("failed to unregister lower hotkey for temporary reservation: %w", err)
		}
		slot.capture = nil
	}

	capture, acquireErr := m.acquireCaptureLocked(reservation.binding)
	if acquireErr != nil {
		// A captura inferior já foi removida. Não a reative: a reserva de
		// prioridade falhou e o slot deve permanecer fail-closed, sem entregar
		// callbacks configuráveis até uma nova tentativa explícita.
		slot.temporary = nil
		var cleanupErr error
		if slotCreated && len(slot.registrations) == 0 {
			delete(m.slots, ownership)
			cleanupErr = m.publishOwnershipLocked(m.ownershipCombinationsLocked(nil))
		}
		m.mu.Unlock()
		return nil, errors.Join(acquireErr, cleanupErr)
	}
	reservation.binding.active.Store(true)
	slot.capture = capture
	if slotCreated {
		m.generation++
	}
	m.mu.Unlock()
	startCaptureListener(capture)

	return func() error { return reservation.release(m) }, nil
}

func (reservation *temporaryReservation) release(m *Manager) error {
	reservation.once.Do(func() {
		reservation.err = m.releaseTemporary(reservation)
	})
	return reservation.err
}

func (m *Manager) releaseTemporary(reservation *temporaryReservation) error {
	m.mu.Lock()
	slot := reservation.slot
	if slot == nil || m.slots[slot.combination] != slot || slot.temporary != reservation || slot.tearingDown {
		m.mu.Unlock()
		return nil
	}
	capture := slot.capture
	if capture == nil {
		m.mu.Unlock()
		return fmt.Errorf("temporary hotkey capture is unavailable")
	}
	// Remover nunca depende de ACK: a interface pode já ter encerrado. O
	// conjunto de ownership continua conservador durante toda a transição.
	stopCapture(capture)
	if err := capture.native.Unregister(); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("failed to unregister temporary hotkey: %w", err)
	}
	slot.capture = nil
	slot.temporary = nil
	lower := m.chooseRegistrationLocked(slot)
	if lower == nil {
		delete(m.slots, slot.combination)
		m.generation++
		err := m.publishOwnershipLocked(m.ownershipCombinationsLocked(nil))
		m.mu.Unlock()
		return err
	}
	// Só uma NOVA captura exige ACK. Falha deixa o registro inferior lógico,
	// mas desabilitado; não recria a reserva de um diálogo que já fechou.
	if err := m.publishOwnershipLocked(m.ownershipCombinationsLocked(nil)); err != nil {
		m.mu.Unlock()
		return err
	}
	replacement, err := m.acquireCaptureLocked(lower)
	if err != nil {
		m.mu.Unlock()
		return err
	}
	slot.capture = replacement
	m.mu.Unlock()
	startCaptureListener(replacement)
	return nil
}

// RegisterSimple registra hotkey com interface simplificada (para compatibilidade)
func (m *Manager) RegisterSimple(modifiers uint32, key uint32, callback HotkeyCallback) (int, error) {
	mods := parseModifiersUint(modifiers)
	k := hotkey.Key(key)
	return m.Register(mods, k, callback)
}

// Unregister remove um hotkey registrado
func (m *Manager) Unregister(id int) error {
	m.mu.Lock()
	registered, exists := m.hotkeys[id]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("hotkey ID %d not found", id)
	}
	if registered.unregistering {
		done := registered.unregisterDone
		m.mu.Unlock()
		<-done
		return registered.unregisterErr
	}
	registered.unregistering = true
	registered.active.Store(false)
	slot := registered.slot
	if slot == nil || slot.capture == nil || slot.capture.binding != registered {
		err := m.removeLogicalLocked(registered)
		m.mu.Unlock()
		return err
	}
	capture := slot.capture
	stopCapture(capture)
	m.mu.Unlock()

	return m.finishUnregister(registered, capture)
}

func (m *Manager) removeLogicalLocked(registered *RegisteredHotkey) error {
	slot := registered.slot
	delete(m.hotkeys, registered.ID)
	registered.removed = true
	if slot == nil {
		completeUnregisterLocked(registered, nil)
		return nil
	}
	delete(slot.registrations, registered.ID)
	var err error
	if slot.temporary == nil && slot.capture == nil && len(slot.registrations) == 0 {
		delete(m.slots, slot.combination)
		m.generation++
		err = m.publishOwnershipLocked(m.ownershipCombinationsLocked(nil))
	}
	completeUnregisterLocked(registered, err)
	return err
}

func completeUnregisterLocked(registered *RegisteredHotkey, err error) {
	if registered.unregisterDoneSet {
		return
	}
	registered.unregisterErr = err
	registered.unregisterDoneSet = true
	close(registered.unregisterDone)
}

func (m *Manager) finishUnregister(registered *RegisteredHotkey, capture *hotkeyCapture) error {
	nativeErr := capture.native.Unregister()
	if nativeErr != nil {
		logging.Warnf(context.Background(), "hotkey.hotkey", "Warning: failed to unregister hotkey %d: %v", registered.ID, nativeErr)
	}
	err := nativeErr

	m.mu.Lock()
	// Falha nativa é fail-closed: sem prova de remoção, o slot e o registro
	// permanecem reservados e nenhum callback inferior é ressuscitado.
	if nativeErr == nil && m.hotkeys[registered.ID] == registered && registered.slot != nil && registered.slot.capture == capture {
		slot := registered.slot
		slot.capture = nil
		delete(m.hotkeys, registered.ID)
		registered.removed = true
		delete(slot.registrations, registered.ID)
		if slot.temporary == nil && !slot.tearingDown {
			if lower := m.chooseRegistrationLocked(slot); lower != nil {
				if publishErr := m.publishOwnershipLocked(m.ownershipCombinationsLocked(nil)); publishErr != nil {
					err = publishErr
				} else {
					replacement, acquireErr := m.acquireCaptureLocked(lower)
					if acquireErr != nil {
						err = acquireErr
					} else {
						slot.capture = replacement
						startCapture := replacement
						defer func() { startCaptureListener(startCapture) }()
					}
				}
			} else if len(slot.registrations) == 0 {
				delete(m.slots, slot.combination)
				m.generation++
				err = m.publishOwnershipLocked(m.ownershipCombinationsLocked(nil))
			}
		}
	}
	completeUnregisterLocked(registered, err)
	m.mu.Unlock()

	if err == nil {
		logging.Infof(context.Background(), "hotkey.hotkey", "Hotkey removido: ID=%d", registered.ID)
	} else if nativeErr == nil {
		logging.Warnf(context.Background(), "hotkey.hotkey", "Hotkey %d removido, mas a publicação de ownership falhou: %v", registered.ID, err)
	} else {
		logging.Warnf(context.Background(), "hotkey.hotkey", "Hotkey %d permanece reservado após falha de remoção nativa", registered.ID)
	}
	return err
}

// UnregisterAll remove todos os hotkeys
func (m *Manager) UnregisterAll() {
	m.teardownMu.Lock()
	defer m.teardownMu.Unlock()

	m.mu.Lock()
	slots := make([]*hotkeySlot, 0, len(m.slots))
	captures := make([]*hotkeyCapture, 0, len(m.slots))
	pending := make([]<-chan struct{}, 0, len(m.hotkeys))
	for _, slot := range m.slots {
		slot.tearingDown = true
		slots = append(slots, slot)
		if slot.capture != nil {
			stopCapture(slot.capture)
			if slot.capture.binding.ID != 0 && slot.capture.binding.unregistering {
				pending = append(pending, slot.capture.binding.unregisterDone)
			} else {
				captures = append(captures, slot.capture)
			}
		}
		for _, registration := range slot.registrations {
			wasUnregistering := registration.unregistering
			if !wasUnregistering {
				registration.unregistering = true
			} else if !registration.unregisterDoneSet {
				pending = append(pending, registration.unregisterDone)
			}
			registration.active.Store(false)
		}
	}
	m.mu.Unlock()

	// Teardown cancela todos os listeners antes de qualquer I/O nativo. Ele
	// não passa por releaseTemporary: não restaura inferiores nem depende de
	// ACK, inclusive quando a barreira já foi fechada pelo App.
	nativeErrors := make(map[*hotkeyCapture]error, len(captures))
	for _, capture := range captures {
		nativeErrors[capture] = capture.native.Unregister()
	}
	for _, done := range pending {
		<-done
	}

	m.mu.Lock()
	for _, slot := range slots {
		if m.slots[slot.combination] != slot {
			continue
		}
		capture := slot.capture
		var nativeErr error
		failed := false
		if capture != nil {
			if err, attempted := nativeErrors[capture]; attempted {
				nativeErr = err
				failed = err != nil
			} else if capture.binding.unregisterDoneSet {
				nativeErr = capture.binding.unregisterErr
				failed = nativeErr != nil
			}
		}
		if failed {
			// Preserve the native owner and every logical registration after an
			// uncertain removal. All callbacks remain disabled.
			for _, registration := range slot.registrations {
				completeUnregisterLocked(registration, nativeErr)
			}
			slot.tearingDown = false
			continue
		}

		for _, registration := range slot.registrations {
			delete(m.hotkeys, registration.ID)
			registration.removed = true
			completeUnregisterLocked(registration, nil)
		}
		slot.registrations = make(map[int]*RegisteredHotkey)
		slot.capture = nil
		slot.temporary = nil
		delete(m.slots, slot.combination)
		m.generation++
		_ = m.publishOwnershipLocked(m.ownershipCombinationsLocked(nil))
	}
	m.mu.Unlock()
}

// SnapshotGlobalOwnership retorna uma cópia ordenada das combinações que o
// manager possui nativamente. A cópia não compartilha memória com o Manager;
// sua identidade não é uma promessa de equivalência com o DOM.
func (m *Manager) SnapshotGlobalOwnership() NativeOwnershipSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	combinations := make([]NativeCombination, 0, len(m.slots))
	for combination := range m.slots {
		combinations = append(combinations, NativeCombination{
			Modifiers: combination.modifiers,
			Key:       combination.key,
		})
	}
	sort.Slice(combinations, func(i, j int) bool {
		if combinations[i].Key != combinations[j].Key {
			return combinations[i].Key < combinations[j].Key
		}
		return combinations[i].Modifiers < combinations[j].Modifiers
	})
	return NativeOwnershipSnapshot{Generation: m.generation, Combinations: combinations}
}

func listenHotkey(ctx context.Context, registered *RegisteredHotkey) {
	// A hotkey.Hotkey exposes independent down/up queues. They are not a
	// single ordered stream: an up from a previous press can be observed before
	// an older down. Preserve the established one-callback-per-KeyDown
	// semantics; do not infer a pressed state here or silently drop quick taps.
	// Windows usa o backend próprio com MOD_NOREPEAT; outras plataformas recusam
	// registro. Não inventamos KeyUp nem deduplicamos filas independentes.
	down := registered.down

	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-down:
			if !ok {
				return
			}
			// Cancelamento não remove um callback já admitido antes desta checagem;
			// o manager não faz join sob seu mutex e, portanto, esse caso pode
			// concluir concorrentemente com Unregister.
			if registered.active.Load() && ctx.Err() == nil {
				registered.Callback()
			}
		}
	}
}

// Start não faz nada nesta implementação (listeners já iniciam no Register)
func (m *Manager) Start() {}

// Stop para todos os hotkeys
func (m *Manager) Stop() {
	m.UnregisterAll()
}

// IsSupported verifica se hotkeys globais são suportados
// Somente Windows possui o adapter qualificado de ownership e no-repeat.
func IsSupported() bool {
	return runtime.GOOS == "windows"
}

// parseModifiersUint converte uint32 para slice de Modifier
func parseModifiersUint(mods uint32) []hotkey.Modifier {
	var result []hotkey.Modifier

	// Mapeamento baseado nas constantes
	if mods&0x0002 != 0 { // ModControl
		result = append(result, ModCtrl)
	}
	if mods&0x0004 != 0 { // ModShift
		result = append(result, ModShift)
	}
	if mods&0x0001 != 0 { // ModAlt
		result = append(result, ModAlt)
	}
	if mods&0x0008 != 0 { // ModWin
		result = append(result, ModWin)
	}

	return result
}

// ParseModifiersString converte string para slice de Modifier
func ParseModifiersString(mods string) []hotkey.Modifier {
	var result []hotkey.Modifier
	modsLower := strings.ToLower(mods)

	if strings.Contains(modsLower, "ctrl") || strings.Contains(modsLower, "control") {
		result = append(result, ModCtrl)
	}
	if strings.Contains(modsLower, "shift") {
		result = append(result, ModShift)
	}
	if strings.Contains(modsLower, "alt") || strings.Contains(modsLower, "option") {
		result = append(result, ModAlt)
	}
	if strings.Contains(modsLower, "win") || strings.Contains(modsLower, "super") || strings.Contains(modsLower, "cmd") {
		result = append(result, ModWin)
	}

	return result
}

// ParseKeyString converte string para Key
// Implementação específica por plataforma definida em hotkey_windows.go e hotkey_darwin.go
func ParseKeyString(key string) (hotkey.Key, error) {
	return parseKeyStringImpl(strings.ToUpper(key))
}

// ParseCombination converte uma string de combinação para modifiers e key
// Exemplo: "Ctrl+Shift+A" -> ([]Modifier{ModCtrl, ModShift}, KeyA)
func ParseCombination(combination string) ([]hotkey.Modifier, hotkey.Key, error) {
	combination = strings.TrimSpace(combination)
	if combination == "" {
		return nil, 0, fmt.Errorf("combination string is empty")
	}

	parts := strings.Split(combination, "+")
	if len(parts) == 0 {
		return nil, 0, fmt.Errorf("invalid combination: %s", combination)
	}

	// O último elemento é a tecla, os anteriores são modificadores
	keyPart := strings.TrimSpace(parts[len(parts)-1])
	modParts := parts[:len(parts)-1]

	for i, part := range parts {
		if strings.TrimSpace(part) == "" {
			return nil, 0, fmt.Errorf("invalid combination %q: empty token at position %d", combination, i)
		}
	}

	// Parse key
	key, err := ParseKeyString(keyPart)
	if err != nil {
		return nil, 0, err
	}

	// Parse modifiers
	var modifiers []hotkey.Modifier
	seen := make(map[hotkey.Modifier]struct{}, len(modParts))
	for _, mod := range modParts {
		modLower := strings.ToLower(strings.TrimSpace(mod))
		var parsed hotkey.Modifier
		switch modLower {
		case "ctrl", "control":
			parsed = ModCtrl
		case "shift":
			parsed = ModShift
		case "alt", "option":
			parsed = ModAlt
		case "win", "super", "cmd", "command", "meta":
			parsed = ModWin
		default:
			return nil, 0, fmt.Errorf("unknown modifier %q", strings.TrimSpace(mod))
		}
		if _, ok := seen[parsed]; ok {
			return nil, 0, fmt.Errorf("duplicate modifier %q", strings.TrimSpace(mod))
		}
		seen[parsed] = struct{}{}
		modifiers = append(modifiers, parsed)
	}

	return modifiers, key, nil
}

// RegisteredProfileHotkey representa um hotkey registrado para um perfil de interação
type RegisteredProfileHotkey struct {
	ProfileID    int
	IsPrimary    bool   // true para hotkey principal, false para secundário
	Combination  string // A combinação original (ex: "Ctrl+Shift+A")
	BringToFront bool   // Se deve trazer janela para frente
	HotkeyID     int    // ID do hotkey registrado no Manager
}

// profileHotkeys guarda o mapeamento de perfis para hotkeys
var profileHotkeys = make(map[int][]*RegisteredProfileHotkey)
var profileHotkeysMu sync.Mutex

// RegisterProfileHotkey registra um hotkey para um perfil de interação
func (m *Manager) RegisterProfileHotkey(profileID int, combination string, isPrimary bool, bringToFront bool, callback HotkeyCallback) (int, error) {
	if combination == "" {
		return 0, fmt.Errorf("combination is empty")
	}

	modifiers, key, err := ParseCombination(combination)
	if err != nil {
		return 0, fmt.Errorf("invalid combination %s: %w", combination, err)
	}

	// Registra o hotkey
	hotkeyID, err := m.Register(modifiers, key, callback)
	if err != nil {
		return 0, err
	}

	// Guarda referência para o perfil
	profileHotkeysMu.Lock()
	profileHotkeys[profileID] = append(profileHotkeys[profileID], &RegisteredProfileHotkey{
		ProfileID:    profileID,
		IsPrimary:    isPrimary,
		Combination:  combination,
		BringToFront: bringToFront,
		HotkeyID:     hotkeyID,
	})
	profileHotkeysMu.Unlock()

	logging.Infof(context.Background(), "hotkey.hotkey", "Profile hotkey registrado: ProfileID=%d, Combination=%s, Primary=%v, BringToFront=%v, HotkeyID=%d",
		profileID, combination, isPrimary, bringToFront, hotkeyID)

	return hotkeyID, nil
}

// UnregisterProfileHotkeys remove todos os hotkeys de um perfil
func (m *Manager) UnregisterProfileHotkeys(profileID int) error {
	profileHotkeysMu.Lock()
	hotkeys, exists := profileHotkeys[profileID]
	if !exists {
		profileHotkeysMu.Unlock()
		return nil
	}
	delete(profileHotkeys, profileID)
	profileHotkeysMu.Unlock()

	var lastErr error
	for _, hk := range hotkeys {
		if err := m.Unregister(hk.HotkeyID); err != nil {
			logging.Warnf(context.Background(), "hotkey.hotkey", "Warning: failed to unregister hotkey %d for profile %d: %v", hk.HotkeyID, profileID, err)
			lastErr = err
		}
	}

	return lastErr
}

// GetProfileHotkeys retorna os hotkeys registrados para um perfil
func GetProfileHotkeys(profileID int) []*RegisteredProfileHotkey {
	profileHotkeysMu.Lock()
	defer profileHotkeysMu.Unlock()

	hotkeys, exists := profileHotkeys[profileID]
	if !exists {
		return nil
	}

	// Retorna cópia para evitar race conditions
	result := make([]*RegisteredProfileHotkey, len(hotkeys))
	copy(result, hotkeys)
	return result
} // UnregisterAllProfileHotkeys remove todos os hotkeys de todos os perfis
func (m *Manager) UnregisterAllProfileHotkeys() {
	profileHotkeysMu.Lock()
	allProfiles := make([]int, 0, len(profileHotkeys))
	for pid := range profileHotkeys {
		allProfiles = append(allProfiles, pid)
	}
	profileHotkeysMu.Unlock()

	for _, pid := range allProfiles {
		_ = m.UnregisterProfileHotkeys(pid)
	}
}
