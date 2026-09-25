package app

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"assistente/internal/hotkey"
	"github.com/google/uuid"
)

const decisionRepeatLease = 30 * time.Second
const decisionRepeatMaxRevision = uint64(9007199254740991)

var errDecisionRepeatStale = errors.New("decision repeat reservation is stale or unavailable")

// A capacidade só repete o anúncio da decisão topmost; não aceita combinações,
// comandos ou argumentos configuráveis e nunca autoriza uma decisão.
type decisionRepeatEvent struct {
	SessionID string `json:"sessionId"`
	Revision  uint64 `json:"revision"`
	DialogID  string `json:"dialogId"`
}

type decisionRepeatReservation struct {
	event   decisionRepeatEvent
	expires atomic.Int64
}

type decisionRepeatHotkeys struct {
	mu       sync.Mutex
	active   atomic.Pointer[decisionRepeatReservation]
	reserve  func(hotkey.HotkeyCallback) (func() error, error)
	emit     func(decisionRepeatEvent)
	allowed  func() bool
	session  string
	revision uint64
	dialog   string
	release  func() error
	timer    *time.Timer
	closed   bool
	fault    error
}

func newDecisionRepeatHotkeys(reserve func(hotkey.HotkeyCallback) (func() error, error), emit func(decisionRepeatEvent), allowed func() bool) *decisionRepeatHotkeys {
	return &decisionRepeatHotkeys{reserve: reserve, emit: emit, allowed: allowed}
}

// OpenDecisionRepeatHotkeySession troca a identidade da conexão do renderer.
// Um renderer anterior não pode renovar nem remover a reserva do atual.
// String vazia anuncia indisponibilidade; o caminho local continua existindo.
func (a *App) OpenDecisionRepeatHotkeySession() string {
	if a == nil || a.decisionRepeatHotkeys == nil {
		return ""
	}
	return a.decisionRepeatHotkeys.open()
}

func (a *App) SetDecisionRepeatHotkey(sessionID string, revision uint64, dialogID string) error {
	if a == nil || a.decisionRepeatHotkeys == nil {
		return errDecisionRepeatStale
	}
	return a.decisionRepeatHotkeys.set(sessionID, revision, dialogID)
}

func (a *App) CloseDecisionRepeatHotkeySession(sessionID string) error {
	if a == nil || a.decisionRepeatHotkeys == nil {
		return nil
	}
	s := a.decisionRepeatHotkeys
	s.mu.Lock()
	defer s.mu.Unlock()
	if sessionID == "" || sessionID != s.session {
		return nil
	}
	s.session = ""
	return s.clearLocked()
}

func (s *decisionRepeatHotkeys) open() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.fault != nil || s.reserve == nil || s.emit == nil || s.allowed == nil {
		return ""
	}
	s.session = ""
	if s.clearLocked() != nil {
		return ""
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return ""
	}
	s.session, s.revision, s.dialog = id.String(), 0, ""
	return s.session
}

func (s *decisionRepeatHotkeys) clearLocked() error {
	s.active.Store(nil)
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	if s.release != nil {
		release := s.release
		s.release = nil
		if err := release(); err != nil {
			// Sem confirmação de teardown nativo não tentamos outra reserva.
			s.fault = err
		}
	}
	return s.fault
}

func (s *decisionRepeatHotkeys) set(session string, revision uint64, dialog string) error {
	if revision == 0 || revision > decisionRepeatMaxRevision || len(dialog) > 256 || strings.TrimSpace(dialog) != dialog {
		return errDecisionRepeatStale
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.fault != nil || session == "" || session != s.session || revision < s.revision || revision == s.revision && dialog != s.dialog {
		return errDecisionRepeatStale
	}
	if entry := s.active.Load(); revision == s.revision && entry != nil {
		s.renewLocked(entry)
		return nil
	}
	if err := s.clearLocked(); err != nil {
		return err
	}
	s.revision, s.dialog = revision, dialog
	if dialog == "" {
		return nil
	}
	entry := &decisionRepeatReservation{event: decisionRepeatEvent{session, revision, dialog}}
	entry.expires.Store(time.Now().Add(decisionRepeatLease).UnixNano())
	s.active.Store(entry)
	release, err := s.reserve(func() {
		if s.active.Load() != entry || time.Now().UnixNano() >= entry.expires.Load() || !s.allowed() {
			return
		}
		if s.active.Load() == entry {
			s.emit(entry.event)
		}
	})
	if err != nil || release == nil {
		s.active.Store(nil)
		if err != nil {
			return err
		}
		return errDecisionRepeatStale
	}
	s.release = release
	s.renewLocked(entry)
	return nil
}

func (s *decisionRepeatHotkeys) renewLocked(entry *decisionRepeatReservation) {
	entry.expires.Store(time.Now().Add(decisionRepeatLease).UnixNano())
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(decisionRepeatLease, func() { s.expire(entry) })
}

func (s *decisionRepeatHotkeys) expire(entry *decisionRepeatReservation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active.Load() != entry || time.Now().UnixNano() < entry.expires.Load() {
		return
	}
	s.session = ""
	_ = s.clearLocked()
}

func (s *decisionRepeatHotkeys) shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.session = ""
	_ = s.clearLocked()
}
