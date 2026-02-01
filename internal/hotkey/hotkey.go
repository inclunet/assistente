// Package hotkey fornece hotkeys globais cross-platform
// Usa golang.design/x/hotkey para suporte a Windows, Linux (X11) e macOS
package hotkey

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"golang.design/x/hotkey"
)

// Modificadores exportados para conveniência
var (
	ModCtrl  = hotkey.ModCtrl
	ModShift = hotkey.ModShift
	ModAlt   = hotkey.ModAlt
	ModWin   = hotkey.ModWin // Win no Windows, Cmd no macOS
)

// Teclas comuns
const (
	KeyA     = hotkey.KeyA
	KeyB     = hotkey.KeyB
	KeyC     = hotkey.KeyC
	KeyD     = hotkey.KeyD
	KeyE     = hotkey.KeyE
	KeyF     = hotkey.KeyF
	KeyG     = hotkey.KeyG
	KeyH     = hotkey.KeyH
	KeyI     = hotkey.KeyI
	KeyJ     = hotkey.KeyJ
	KeyK     = hotkey.KeyK
	KeyL     = hotkey.KeyL
	KeyM     = hotkey.KeyM
	KeyN     = hotkey.KeyN
	KeyO     = hotkey.KeyO
	KeyP     = hotkey.KeyP
	KeyQ     = hotkey.KeyQ
	KeyR     = hotkey.KeyR
	KeyS     = hotkey.KeyS
	KeyT     = hotkey.KeyT
	KeyU     = hotkey.KeyU
	KeyV     = hotkey.KeyV
	KeyW     = hotkey.KeyW
	KeyX     = hotkey.KeyX
	KeyY     = hotkey.KeyY
	KeyZ     = hotkey.KeyZ
	KeySpace = hotkey.KeySpace
	KeyF1    = hotkey.KeyF1
	KeyF2    = hotkey.KeyF2
	KeyF3    = hotkey.KeyF3
	KeyF4    = hotkey.KeyF4
	KeyF5    = hotkey.KeyF5
	KeyF6    = hotkey.KeyF6
	KeyF7    = hotkey.KeyF7
	KeyF8    = hotkey.KeyF8
	KeyF9    = hotkey.KeyF9
	KeyF10   = hotkey.KeyF10
	KeyF11   = hotkey.KeyF11
	KeyF12   = hotkey.KeyF12
)

// HotkeyCallback função chamada quando hotkey é pressionado
type HotkeyCallback func()

// RegisteredHotkey representa um hotkey registrado
type RegisteredHotkey struct {
	ID        int
	Modifiers []hotkey.Modifier
	Key       hotkey.Key
	Callback  HotkeyCallback
	hotkey    *hotkey.Hotkey
	cancel    context.CancelFunc
}

// Manager gerencia hotkeys globais
type Manager struct {
	hotkeys map[int]*RegisteredHotkey
	nextID  int
	mu      sync.RWMutex
}

// singleton
var (
	globalManager *Manager
	managerOnce   sync.Once
)

// GetManager retorna o manager singleton
func GetManager() *Manager {
	managerOnce.Do(func() {
		globalManager = &Manager{
			hotkeys: make(map[int]*RegisteredHotkey),
			nextID:  1,
		}
	})
	return globalManager
}

// Register registra um novo hotkey global
func (m *Manager) Register(modifiers []hotkey.Modifier, key hotkey.Key, callback HotkeyCallback) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := m.nextID
	m.nextID++

	// Cria o hotkey
	hk := hotkey.New(modifiers, key)

	// Tenta registrar
	if err := hk.Register(); err != nil {
		return 0, fmt.Errorf("failed to register hotkey: %w", err)
	}

	// Contexto para cancelar o listener
	ctx, cancel := context.WithCancel(context.Background())

	// Inicia listener em goroutine
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-hk.Keydown():
				callback()
			}
		}
	}()

	m.hotkeys[id] = &RegisteredHotkey{
		ID:        id,
		Modifiers: modifiers,
		Key:       key,
		Callback:  callback,
		hotkey:    hk,
		cancel:    cancel,
	}

	log.Printf("Hotkey registrado: ID=%d, Modifiers=%v, Key=%v", id, modifiers, key)
	return id, nil
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
	defer m.mu.Unlock()

	hk, exists := m.hotkeys[id]
	if !exists {
		return fmt.Errorf("hotkey ID %d not found", id)
	}

	// Cancela o listener
	if hk.cancel != nil {
		hk.cancel()
	}

	// Desregistra o hotkey
	if err := hk.hotkey.Unregister(); err != nil {
		log.Printf("Warning: failed to unregister hotkey %d: %v", id, err)
	}

	delete(m.hotkeys, id)
	log.Printf("Hotkey removido: ID=%d", id)
	return nil
}

// UnregisterAll remove todos os hotkeys
func (m *Manager) UnregisterAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, hk := range m.hotkeys {
		if hk.cancel != nil {
			hk.cancel()
		}
		if hk.hotkey != nil {
			hk.hotkey.Unregister()
		}
		log.Printf("Hotkey removido: ID=%d", id)
	}
	m.hotkeys = make(map[int]*RegisteredHotkey)
}

// Start não faz nada nesta implementação (listeners já iniciam no Register)
func (m *Manager) Start() {}

// Stop para todos os hotkeys
func (m *Manager) Stop() {
	m.UnregisterAll()
}

// IsSupported verifica se hotkeys globais são suportados
// Retorna true para Windows, Linux X11 e macOS
func IsSupported() bool {
	return true
}

// parseModifiersUint converte uint32 para slice de Modifier
func parseModifiersUint(mods uint32) []hotkey.Modifier {
	var result []hotkey.Modifier

	// Mapeamento baseado nas constantes
	if mods&0x0002 != 0 { // ModControl
		result = append(result, hotkey.ModCtrl)
	}
	if mods&0x0004 != 0 { // ModShift
		result = append(result, hotkey.ModShift)
	}
	if mods&0x0001 != 0 { // ModAlt
		result = append(result, hotkey.ModAlt)
	}
	if mods&0x0008 != 0 { // ModWin
		result = append(result, hotkey.ModWin)
	}

	return result
}

// ParseModifiersString converte string para slice de Modifier
func ParseModifiersString(mods string) []hotkey.Modifier {
	var result []hotkey.Modifier
	modsLower := strings.ToLower(mods)

	if strings.Contains(modsLower, "ctrl") || strings.Contains(modsLower, "control") {
		result = append(result, hotkey.ModCtrl)
	}
	if strings.Contains(modsLower, "shift") {
		result = append(result, hotkey.ModShift)
	}
	if strings.Contains(modsLower, "alt") || strings.Contains(modsLower, "option") {
		result = append(result, hotkey.ModAlt)
	}
	if strings.Contains(modsLower, "win") || strings.Contains(modsLower, "super") || strings.Contains(modsLower, "cmd") {
		result = append(result, hotkey.ModWin)
	}

	return result
}

// ParseKeyString converte string para Key
func ParseKeyString(key string) (hotkey.Key, error) {
	keyMap := map[string]hotkey.Key{
		"A": hotkey.KeyA, "B": hotkey.KeyB, "C": hotkey.KeyC, "D": hotkey.KeyD,
		"E": hotkey.KeyE, "F": hotkey.KeyF, "G": hotkey.KeyG, "H": hotkey.KeyH,
		"I": hotkey.KeyI, "J": hotkey.KeyJ, "K": hotkey.KeyK, "L": hotkey.KeyL,
		"M": hotkey.KeyM, "N": hotkey.KeyN, "O": hotkey.KeyO, "P": hotkey.KeyP,
		"Q": hotkey.KeyQ, "R": hotkey.KeyR, "S": hotkey.KeyS, "T": hotkey.KeyT,
		"U": hotkey.KeyU, "V": hotkey.KeyV, "W": hotkey.KeyW, "X": hotkey.KeyX,
		"Y": hotkey.KeyY, "Z": hotkey.KeyZ,
		"SPACE": hotkey.KeySpace,
		"F1":    hotkey.KeyF1, "F2": hotkey.KeyF2, "F3": hotkey.KeyF3, "F4": hotkey.KeyF4,
		"F5": hotkey.KeyF5, "F6": hotkey.KeyF6, "F7": hotkey.KeyF7, "F8": hotkey.KeyF8,
		"F9": hotkey.KeyF9, "F10": hotkey.KeyF10, "F11": hotkey.KeyF11, "F12": hotkey.KeyF12,
	}

	if k, ok := keyMap[strings.ToUpper(key)]; ok {
		return k, nil
	}
	return 0, fmt.Errorf("unknown key: %s", key)
}

// ParseCombination converte uma string de combinação para modifiers e key
// Exemplo: "Ctrl+Shift+A" -> ([]Modifier{ModCtrl, ModShift}, KeyA)
func ParseCombination(combination string) ([]hotkey.Modifier, hotkey.Key, error) {
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

	// Parse key
	key, err := ParseKeyString(keyPart)
	if err != nil {
		return nil, 0, err
	}

	// Parse modifiers
	var modifiers []hotkey.Modifier
	for _, mod := range modParts {
		modLower := strings.ToLower(strings.TrimSpace(mod))
		switch modLower {
		case "ctrl", "control":
			modifiers = append(modifiers, hotkey.ModCtrl)
		case "shift":
			modifiers = append(modifiers, hotkey.ModShift)
		case "alt", "option":
			modifiers = append(modifiers, hotkey.ModAlt)
		case "win", "super", "cmd", "command", "meta":
			modifiers = append(modifiers, hotkey.ModWin)
		}
	}

	return modifiers, key, nil
}

// RegisteredProfileHotkey representa um hotkey registrado para um perfil de interação
type RegisteredProfileHotkey struct {
	ProfileID     int
	IsPrimary     bool   // true para hotkey principal, false para secundário
	Combination   string // A combinação original (ex: "Ctrl+Shift+A")
	BringToFront  bool   // Se deve trazer janela para frente
	HotkeyID      int    // ID do hotkey registrado no Manager
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

	log.Printf("Profile hotkey registrado: ProfileID=%d, Combination=%s, Primary=%v, BringToFront=%v, HotkeyID=%d",
		profileID, combination, isPrimary, bringToFront, hotkeyID)

	return hotkeyID, nil
}// UnregisterProfileHotkeys remove todos os hotkeys de um perfil
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
			log.Printf("Warning: failed to unregister hotkey %d for profile %d: %v", hk.HotkeyID, profileID, err)
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
}

// UnregisterAllProfileHotkeys remove todos os hotkeys de todos os perfis
func (m *Manager) UnregisterAllProfileHotkeys() {
	profileHotkeysMu.Lock()
	allProfiles := make([]int, 0, len(profileHotkeys))
	for pid := range profileHotkeys {
		allProfiles = append(allProfiles, pid)
	}
	profileHotkeysMu.Unlock()

	for _, pid := range allProfiles {
		m.UnregisterProfileHotkeys(pid)
	}
}