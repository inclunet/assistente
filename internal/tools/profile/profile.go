// Package profile expõe o control-plane de profiles ao modelo (AEP-0101).
package profile

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"assistente/internal/profileaccess"
	"assistente/internal/tools"
	"assistente/internal/tools/invocationctx"
)

const (
	ActionList   = "list"
	ActionSwitch = "switch"
	maxReasonLen = 500
)

type Access interface {
	List(context.Context, string) ([]profileaccess.ProfileSummary, error)
	Authorize(context.Context, profileaccess.AuthorizationRequest) (bool, error)
	ValidateTarget(context.Context, string) error
}

// Switcher persiste o override somente se a aba ainda pertencer à conversa.
type Switcher interface {
	ValidateTabConversation(tabID, conversationID string) error
	SwitchTabProfile(tabID, conversationID, profileSlug string) error
	ResetConversationTools(conversationID string)
}

type Tool struct {
	access   Access
	switcher Switcher
}

func New(access Access, switcher Switcher) *Tool {
	return &Tool{access: access, switcher: switcher}
}

func (t *Tool) Name() string { return "profile" }

func (t *Tool) CatalogMetadata() tools.CatalogMetadata {
	return tools.CatalogMetadata{
		Category: "agents",
		Class:    "profile_control",
		Package:  "agents",
		Risk:     "write",
	}
}

func (t *Tool) Description() string {
	return "Discover interaction profiles and request a persistent profile change. Use action='list' (default) to inspect installed profile slugs, names, descriptions, current status, and availability before choosing a profile for the subagent tool. Use action='switch' only when the main conversation should keep using another profile in future turns; it always asks the user for confirmation and takes effect on the next turn. For immediate specialized work, prefer subagent with the selected profile."
}

func (t *Tool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {
				"type": "string",
				"enum": ["list", "switch"],
				"description": "Operation. Defaults to list."
			},
			"slug": {
				"type": "string",
				"description": "Target profile slug. Required for switch."
			},
			"reason": {
				"type": "string",
				"description": "Short user-facing reason for changing the main conversation profile. Required for switch."
			}
		},
		"additionalProperties": false
	}`)
}

type args struct {
	Action string `json:"action"`
	Slug   string `json:"slug"`
	Reason string `json:"reason"`
}

type response struct {
	Action      string                         `json:"action"`
	CurrentSlug string                         `json:"current_slug,omitempty"`
	Profiles    []profileaccess.ProfileSummary `json:"profiles,omitempty"`
	Changed     bool                           `json:"changed,omitempty"`
	TargetSlug  string                         `json:"target_slug,omitempty"`
	AppliesFrom string                         `json:"applies_from,omitempty"`
	Authorized  *bool                          `json:"authorized,omitempty"`
}

func (t *Tool) Execute(ctx context.Context, raw json.RawMessage) (tools.ToolResult, error) {
	var input args
	if len(raw) > 0 {
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			return errorResult("invalid_arguments", fmt.Sprintf("argumentos inválidos: %v", err)), nil
		}
	}
	action := strings.TrimSpace(input.Action)
	if action == "" {
		action = ActionList
	}
	inv, _ := invocationctx.Get(ctx)
	currentSlug := strings.TrimSpace(inv.ProfileSlug)

	switch action {
	case ActionList:
		if t == nil || t.access == nil {
			return errorResult("catalog_unavailable", "catálogo de profiles indisponível"), nil
		}
		profiles, err := t.access.List(ctx, currentSlug)
		if err != nil {
			return errorResult("list_failed", fmt.Sprintf("erro ao listar profiles: %v", err)), nil
		}
		return jsonResult(response{
			Action:      ActionList,
			CurrentSlug: currentSlug,
			Profiles:    profiles,
		})

	case ActionSwitch:
		return t.executeSwitch(ctx, inv, currentSlug, input)

	default:
		return errorResult("invalid_action", "action inválida: use 'list' ou 'switch'"), nil
	}
}

func (t *Tool) executeSwitch(ctx context.Context, inv invocationctx.InvocationContext, currentSlug string, input args) (tools.ToolResult, error) {
	targetSlug := strings.TrimSpace(input.Slug)
	reason := strings.TrimSpace(input.Reason)
	if targetSlug == "" {
		return errorResult("target_required", "'slug' é obrigatório para switch"), nil
	}
	if reason == "" {
		return errorResult("reason_required", "'reason' é obrigatório para switch"), nil
	}
	if len([]rune(reason)) > maxReasonLen {
		return errorResult("reason_too_long", fmt.Sprintf("'reason' excede %d caracteres", maxReasonLen)), nil
	}
	if targetSlug == currentSlug {
		authorized := true
		return jsonResult(response{
			Action:      ActionSwitch,
			CurrentSlug: currentSlug,
			Changed:     false,
			TargetSlug:  targetSlug,
			Authorized:  &authorized,
		})
	}
	if strings.TrimSpace(inv.Source) != "wails" || strings.TrimSpace(inv.SurfaceTabID) == "" || strings.TrimSpace(inv.ConversationID) == "" {
		return errorResult("desktop_tab_required", "troca de profile requer uma conversa aberta em aba do aplicativo"), nil
	}
	if t == nil || t.access == nil || t.switcher == nil {
		return errorResult("switch_unavailable", "troca de profile indisponível"), nil
	}
	if err := t.switcher.ValidateTabConversation(inv.SurfaceTabID, inv.ConversationID); err != nil {
		return errorResult("invalid_tab_conversation", fmt.Sprintf("aba de origem inválida para troca de profile: %v", err)), nil
	}
	allowed, err := t.access.Authorize(ctx, profileaccess.AuthorizationRequest{
		Source:           inv.Source,
		ConversationID:   inv.ConversationID,
		CurrentSlug:      currentSlug,
		TargetSlug:       targetSlug,
		TaskTitle:        reason,
		PersistentSwitch: true,
	})
	if err != nil {
		return errorResult("authorization_failed", fmt.Sprintf("não foi possível autorizar a troca de profile: %v", err)), nil
	}
	if !allowed {
		authorized := false
		return jsonResult(response{
			Action:      ActionSwitch,
			CurrentSlug: currentSlug,
			Changed:     false,
			TargetSlug:  targetSlug,
			Authorized:  &authorized,
		})
	}
	if err := t.access.ValidateTarget(ctx, targetSlug); err != nil {
		return errorResult("target_unavailable", fmt.Sprintf("profile alvo indisponível após autorização: %v", err)), nil
	}
	if err := t.switcher.SwitchTabProfile(inv.SurfaceTabID, inv.ConversationID, targetSlug); err != nil {
		return errorResult("persistence_failed", fmt.Sprintf("erro ao trocar profile: %v", err)), nil
	}
	t.switcher.ResetConversationTools(inv.ConversationID)
	authorized := true
	return jsonResult(response{
		Action:      ActionSwitch,
		CurrentSlug: currentSlug,
		Changed:     true,
		TargetSlug:  targetSlug,
		AppliesFrom: "next_turn",
		Authorized:  &authorized,
	})
}

func errorResult(code, message string) tools.ToolResult {
	payload, _ := json.Marshal(map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
	return tools.ToolResult{
		Content: string(payload),
		IsError: true,
		Metadata: map[string]any{
			"error_code": code,
		},
	}
}

func jsonResult(value response) (tools.ToolResult, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return errorResult("serialization_failed", fmt.Sprintf("erro ao serializar resposta de profile: %v", err)), nil
	}
	return tools.ToolResult{
		Content: string(payload),
		Metadata: map[string]any{
			"action":       value.Action,
			"changed":      value.Changed,
			"target_slug":  value.TargetSlug,
			"applies_from": value.AppliesFrom,
		},
	}, nil
}
