package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"assistente/internal/profiles"
)

var (
	// ErrInvalidProfileHotkeyOccurrence indica ocorrência vazia ou criada fora
	// do callback nativo confiável.
	ErrInvalidProfileHotkeyOccurrence = errors.New("ocorrência de hotkey de perfil inválida")
	// ErrStaleProfileHotkeyOccurrence indica uma ocorrência aposentada ou que
	// não corresponde mais à configuração autoritativa do perfil ativo.
	ErrStaleProfileHotkeyOccurrence = errors.New("ocorrência de hotkey de perfil obsoleta")
)

// ProfileHotkeyBinding é a projeção autoritativa de um trigger de hotkey do
// perfil ativo. Fingerprint identifica a configuração exata que foi projetada.
type ProfileHotkeyBinding struct {
	ProfileSlug  string `json:"profileSlug"`
	Hotkey       string `json:"hotkey"`
	TriggerType  string `json:"triggerType"`
	Fingerprint  string `json:"fingerprint"`
	BringToFront bool   `json:"bringToFront"`
	Global       bool   `json:"global"`
}

// ProfileHotkeyOccurrence é opaca: somente o callback criado pelo registro
// nativo recebe uma instância válida.
type ProfileHotkeyOccurrence struct {
	controller   *HotkeysController
	registration *profileHotkeyRegistration
	binding      ProfileHotkeyBinding
}

// Binding devolve uma cópia do binding capturado pela ocorrência.
func (o ProfileHotkeyOccurrence) Binding() ProfileHotkeyBinding {
	return o.binding
}

// Current informa somente o estado volátil em memória da geração. Não consulta
// perfis, disco, banco ou qualquer outra autoridade externa.
func (o ProfileHotkeyOccurrence) Current() bool {
	if o.controller == nil || o.registration == nil || o.registration.ctx == nil || o.registration.ctx.Err() != nil {
		return false
	}
	o.controller.mu.Lock()
	defer o.controller.mu.Unlock()
	return o.controller.registration == o.registration && o.controller.generation == o.registration.generation
}

// Context devolve o contexto de vida da geração que originou a ocorrência.
// Ele é cancelado em reload ou Stop; o chamador deve mesclá-lo com o contexto
// do App antes de executar o comando.
func (o ProfileHotkeyOccurrence) Context() context.Context {
	if o.registration == nil {
		return nil
	}
	return o.registration.ctx
}

// Validate revalida a ocorrência contra a configuração autoritativa atual do
// perfil. A checagem de Current permanece apenas em memória; esta etapa relê o
// perfil ativo e compara input habilitado, trigger exato e fingerprint.
func (o ProfileHotkeyOccurrence) Validate(ctx context.Context) error {
	if ctx == nil || o.controller == nil || o.registration == nil || o.registration.ctx == nil || o.binding == (ProfileHotkeyBinding{}) {
		return ErrInvalidProfileHotkeyOccurrence
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !o.Current() {
		return ErrStaleProfileHotkeyOccurrence
	}
	if err := o.registration.ctx.Err(); err != nil {
		return err
	}
	if o.controller.profileMgr == nil {
		return ErrStaleProfileHotkeyOccurrence
	}
	active, err := o.controller.profileMgr.GetActiveAndSlug()
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if active == nil || active.Profile == nil || active.Slug != o.binding.ProfileSlug || !active.Profile.Input.Enabled {
		return ErrStaleProfileHotkeyOccurrence
	}
	for _, trigger := range active.Profile.Input.Triggers {
		if !trigger.Enabled || trigger.Hotkey != o.binding.Hotkey || trigger.Type != o.binding.TriggerType ||
			trigger.HotkeyGlobal != o.binding.Global || trigger.HotkeyBringToFront != o.binding.BringToFront {
			continue
		}
		fingerprint, err := profileHotkeyFingerprint(active.Slug, active.Profile.Input, trigger)
		if err != nil {
			return err
		}
		if fingerprint == o.binding.Fingerprint {
			return nil
		}
	}
	return ErrStaleProfileHotkeyOccurrence
}

// CommandHotkeyBindings devolve o snapshot autoritativo dos hotkeys do perfil
// ativo. A consulta não registra, executa nem autoriza ocorrências.
func (c *HotkeysController) CommandHotkeyBindings() ([]ProfileHotkeyBinding, error) {
	if c == nil || c.profileMgr == nil {
		return nil, ErrStaleProfileHotkeyOccurrence
	}
	active, err := c.profileMgr.GetActiveAndSlug()
	if err != nil {
		return nil, err
	}
	if active == nil || active.Profile == nil || !active.Profile.Input.Enabled {
		return []ProfileHotkeyBinding{}, nil
	}
	return buildProfileHotkeyBindings(active.Slug, active.Profile)
}

func buildProfileHotkeyBindings(slug string, profile *profiles.Profile) ([]ProfileHotkeyBinding, error) {
	if profile == nil {
		return nil, ErrStaleProfileHotkeyOccurrence
	}
	bindings := make([]ProfileHotkeyBinding, 0, len(profile.Input.Triggers))
	for _, trigger := range profile.Input.Triggers {
		if !trigger.Enabled || trigger.Hotkey == "" {
			continue
		}
		fingerprint, err := profileHotkeyFingerprint(slug, profile.Input, trigger)
		if err != nil {
			return nil, err
		}
		bindings = append(bindings, ProfileHotkeyBinding{
			ProfileSlug:  slug,
			Hotkey:       trigger.Hotkey,
			TriggerType:  trigger.Type,
			Fingerprint:  fingerprint,
			BringToFront: trigger.HotkeyBringToFront,
			Global:       trigger.HotkeyGlobal,
		})
	}
	return bindings, nil
}

func profileHotkeyFingerprint(slug string, input profiles.InputConfig, trigger profiles.TriggerConfig) (string, error) {
	payload := struct {
		Version     int                    `json:"version"`
		ProfileSlug string                 `json:"profileSlug"`
		Input       profiles.InputConfig   `json:"input"`
		Trigger     profiles.TriggerConfig `json:"trigger"`
	}{Version: 1, ProfileSlug: slug, Input: input, Trigger: trigger}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(append([]byte("assistente.profile-hotkey.v1\x00"), encoded...))
	return hex.EncodeToString(digest[:]), nil
}
