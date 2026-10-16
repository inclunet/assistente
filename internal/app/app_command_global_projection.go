package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"assistente/internal/commandjson"
	"assistente/internal/database"
	"assistente/internal/hotkey"
	"assistente/internal/logging"
)

type commandGlobalBinding struct {
	ID, CommandID, Identity, Fingerprint string
	Arguments, Trigger                   json.RawMessage
}

// The legacy OS configuration names logical keys (A..Z, Space, F1..F12).
// Its global identity is not a conversion from VK to a physical DOM position.
func commandGlobalTrigger(keys string) (json.RawMessage, string, error) {
	modifiers, _, err := hotkey.ParseCombination(keys)
	if err != nil {
		return nil, "", err
	}
	parts := strings.Split(keys, "+")
	code := strings.ToUpper(strings.TrimSpace(parts[len(parts)-1]))
	if len(code) == 1 && code[0] >= 'A' && code[0] <= 'Z' {
		code = "Key" + code
	}
	if code == "SPACE" {
		code = "Space"
	}
	names := []string{}
	for _, modifier := range modifiers {
		switch modifier {
		case hotkey.ModCtrl:
			names = append(names, "Control")
		case hotkey.ModShift:
			names = append(names, "Shift")
		case hotkey.ModAlt:
			names = append(names, "Alt")
		case hotkey.ModWin:
			names = append(names, "Meta")
		default:
			return nil, "", commandexecution.ErrInvalidConfiguration
		}
	}
	sort.Strings(names)
	raw, err := commandjson.Marshal(map[string]any{"version": 1, "code": code, "modifiers": names})
	if err != nil {
		return nil, "", err
	}
	identity, err := (commandconfig.KeyboardGlobalTriggerPort{}).Normalize(context.Background(), raw)
	return raw, identity, err
}

func newCommandGlobalBinding(commandID, keys, originFingerprint string, args any) (commandGlobalBinding, error) {
	raw, identity, err := commandGlobalTrigger(keys)
	if err != nil {
		return commandGlobalBinding{}, err
	}
	arguments, err := commandjson.Marshal(args)
	if err != nil || originFingerprint == "" {
		return commandGlobalBinding{}, commandexecution.ErrInvalidConfiguration
	}
	canonical, err := commandjson.Marshal([]string{"global-binding-v1", commandID, identity, originFingerprint, string(arguments)})
	if err != nil {
		return commandGlobalBinding{}, err
	}
	digest := sha256.Sum256(canonical)
	fingerprint := hex.EncodeToString(digest[:])
	return commandGlobalBinding{ID: "builtin.global.binding_" + fingerprint, CommandID: commandID, Identity: identity, Fingerprint: fingerprint, Arguments: arguments, Trigger: raw}, nil
}

func (a *App) commandGlobalBindings(ctx context.Context) ([]commandGlobalBinding, error) {
	var bindings []commandGlobalBinding
	if a.hotkeyCtrl != nil {
		profiles, err := a.hotkeyCtrl.CommandHotkeyBindings()
		if err != nil {
			return nil, err
		}
		for _, b := range profiles {
			binding, err := newCommandGlobalBinding(commandGlobalVoiceID, b.Hotkey, b.Fingerprint, map[string]any{
				"profile_slug": b.ProfileSlug, "trigger_type": b.TriggerType, "bring_to_front": b.BringToFront,
			})
			if err != nil {
				logging.Println(ctx, "commands.global", "Ignorando hotkey de voz inválida; o registro nativo também a recusa")
				continue
			}
			bindings = append(bindings, binding)
		}
	}
	if a.jobMgr != nil {
		principal, err := a.currentCommandPrincipal()
		if err != nil {
			return nil, err
		}
		ctx = database.WithUserID(ctx, principal.UserID)
		jobs, err := a.jobMgr.CommandHotkeyBindings(ctx)
		if err != nil {
			return nil, err
		}
		for _, b := range jobs {
			binding, err := newCommandGlobalBinding(commandGlobalJobID, b.Keys, b.BindingFingerprint, map[string]string{"job_id": b.JobDatabaseID})
			if err != nil {
				logging.Println(ctx, "commands.global", "Ignorando hotkey de job inválida; o registro nativo também a recusa")
				continue
			}
			bindings = append(bindings, binding)
		}
	}
	sort.Slice(bindings, func(i, j int) bool { return bindings[i].ID < bindings[j].ID })
	return bindings, nil
}

func (a *App) commandProductGlobalProjection(ctx context.Context, registry *commandcatalog.Registry, active []string) (commandconfig.CompleteProjection, error) {
	projection, err := commandProductProjection(registry, active)
	if err != nil {
		return projection, err
	}
	bindings, err := a.commandGlobalBindings(ctx)
	if err != nil {
		return projection, err
	}
	layer := commandconfig.BuiltinLayer{ID: commandGlobalLayerID, Active: true}
	seen := map[string]bool{}
	for _, b := range bindings {
		if seen[b.ID] {
			continue
		}
		seen[b.ID] = true
		layer.Defaults = append(layer.Defaults, commandbindings.Default{Version: "1", Fingerprint: b.Fingerprint,
			Candidate: commandbindings.Candidate{ID: b.ID, Trigger: b.Identity, CommandID: b.CommandID, ArgumentsKey: string(b.Arguments),
				ExecutionScopeKey: "global", Scope: commandbindings.Application, Enabled: true, LayerActive: true},
		})
	}
	if len(layer.Defaults) > 0 {
		projection.BuiltinLayers = append(projection.BuiltinLayers, layer)
	}
	return projection, nil
}
