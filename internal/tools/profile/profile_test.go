package profile

import (
	"context"
	"encoding/json"
	"testing"

	"assistente/internal/profileaccess"
	"assistente/internal/tools/invocationctx"
)

type fakeAccess struct {
	profiles  []profileaccess.ProfileSummary
	allowed   bool
	authErr   error
	authCalls int
	lastAuth  profileaccess.AuthorizationRequest
}

func (f *fakeAccess) List(context.Context, string) ([]profileaccess.ProfileSummary, error) {
	return f.profiles, nil
}

func (f *fakeAccess) Authorize(_ context.Context, request profileaccess.AuthorizationRequest) (bool, error) {
	f.authCalls++
	f.lastAuth = request
	return f.allowed, f.authErr
}

type fakeSwitcher struct {
	tabID          string
	conversationID string
	profileSlug    string
	resetID        string
	err            error
}

func (f *fakeSwitcher) SwitchTabProfile(tabID, conversationID, profileSlug string) error {
	f.tabID, f.conversationID, f.profileSlug = tabID, conversationID, profileSlug
	return f.err
}

func (f *fakeSwitcher) ResetConversationTools(conversationID string) {
	f.resetID = conversationID
}

func profileToolContext(source string) context.Context {
	return invocationctx.With(context.Background(), invocationctx.InvocationContext{
		ConversationID: "conversation-1",
		ProfileSlug:    "geral",
		Source:         source,
		SurfaceTabID:   "tab-1",
	})
}

func TestListProfilesDefaultsAction(t *testing.T) {
	access := &fakeAccess{profiles: []profileaccess.ProfileSummary{{
		Slug: "custom", Name: "Custom", Description: "Especializado", Available: true,
	}}}
	tool := New(access, &fakeSwitcher{})

	result, err := tool.Execute(profileToolContext("wails"), json.RawMessage(`{}`))
	if err != nil || result.IsError {
		t.Fatalf("resultado inesperado: %#v err=%v", result, err)
	}
	var payload response
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Action != ActionList || payload.CurrentSlug != "geral" || len(payload.Profiles) != 1 {
		t.Fatalf("payload inesperado: %#v", payload)
	}
}

func TestSwitchApprovedPersistsAndResetsTools(t *testing.T) {
	access := &fakeAccess{allowed: true}
	switcher := &fakeSwitcher{}
	tool := New(access, switcher)

	result, err := tool.Execute(profileToolContext("wails"), json.RawMessage(
		`{"action":"switch","slug":"custom","reason":"preciso deste especialista"}`,
	))
	if err != nil || result.IsError {
		t.Fatalf("resultado inesperado: %#v err=%v", result, err)
	}
	var payload response
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Changed || payload.AppliesFrom != "next_turn" || switcher.profileSlug != "custom" ||
		switcher.tabID != "tab-1" || switcher.resetID != "conversation-1" {
		t.Fatalf("troca inesperada: payload=%#v switcher=%#v", payload, switcher)
	}
	if access.lastAuth.CurrentSlug != "geral" || !access.lastAuth.PersistentSwitch {
		t.Fatalf("autorização inesperada: %#v", access.lastAuth)
	}
}

func TestSwitchDeniedDoesNotMutate(t *testing.T) {
	access := &fakeAccess{allowed: false}
	switcher := &fakeSwitcher{}
	tool := New(access, switcher)

	result, err := tool.Execute(profileToolContext("wails"), json.RawMessage(
		`{"action":"switch","slug":"custom","reason":"motivo"}`,
	))
	if err != nil || result.IsError {
		t.Fatalf("recusa é resultado normal: %#v err=%v", result, err)
	}
	var payload response
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Changed || payload.Authorized == nil || *payload.Authorized || switcher.profileSlug != "" {
		t.Fatalf("recusa mutou estado: payload=%#v switcher=%#v", payload, switcher)
	}
}

func TestSwitchRejectsNonDesktopSource(t *testing.T) {
	access := &fakeAccess{allowed: true}
	switcher := &fakeSwitcher{}
	result, err := New(access, switcher).Execute(profileToolContext("telegram"), json.RawMessage(
		`{"action":"switch","slug":"custom","reason":"motivo"}`,
	))
	if err != nil || !result.IsError || access.authCalls != 0 || switcher.profileSlug != "" {
		t.Fatalf("origem remota não deveria trocar: %#v err=%v", result, err)
	}
}
