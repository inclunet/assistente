package chat

import (
	"context"
	"strings"
	"testing"

	"assistente/internal/profiles"
)

func TestResolvePromptCacheHintKeyRequiresEnabledProviderHints(t *testing.T) {
	profile := profiles.DefaultProfile()
	profile.Chat.LLMProvider = "openai-default"
	profile.Chat.Model = "gpt-4o-mini"

	if got := ResolvePromptCacheHintKey(profile, "dev", "conv-1", "gpt-4o-mini"); got != "" {
		t.Fatalf("key with cache disabled = %q, want empty", got)
	}

	profile.Chat.PromptCache.Enabled = true
	if got := ResolvePromptCacheHintKey(profile, "dev", "conv-1", "gpt-4o-mini"); got != "" {
		t.Fatalf("key with hints disabled = %q, want empty", got)
	}

	profile.Chat.PromptCache.ProviderHints = true
	got := ResolvePromptCacheHintKey(profile, "dev", "conv-1", "gpt-4o-mini")
	if got == "" {
		t.Fatal("key with prompt cache + hints enabled is empty")
	}
	if !strings.HasPrefix(got, "asst-") {
		t.Fatalf("key = %q, want asst-*", got)
	}
	for _, forbidden := range []string{"openai-default", "gpt-4o-mini", "dev", "conv-1"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("key %q leaked %q", got, forbidden)
		}
	}
}

func TestPrepareContextPromptCacheKeyUsesEffectiveModel(t *testing.T) {
	spy := &spyEmitter{}
	profileMgr := setupProfileTestEnv(t)

	profile := profiles.DefaultProfile()
	profile.Name = "Cache"
	profile.Active = true
	profile.Chat.LLMProvider = "openai-default"
	profile.Chat.Model = ""
	profile.Chat.PromptCache.Enabled = true
	profile.Chat.PromptCache.ProviderHints = true
	slug, err := profileMgr.Create(profile)
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if err := profileMgr.SetActive(slug); err != nil {
		t.Fatalf("set active: %v", err)
	}

	inter := NewInteractor(InteractorConfig{
		Emitter:    spy,
		ConvRepo:   noopConvRepo{},
		ProfileMgr: profileMgr,
	})

	resp, err := inter.PrepareContext(context.Background(), PrepareContextRequest{
		ConversationID: "conv-1",
		UserContent:    "oi",
		DefaultModel:   "fallback-model",
	})
	if err != nil {
		t.Fatalf("PrepareContext fallback: %v", err)
	}
	wantFallbackKey := ResolvePromptCacheHintKey(resp.ActiveProfile, slug, "conv-1", resp.Params.Model)
	if resp.Params.PromptCacheKey != wantFallbackKey {
		t.Fatalf("fallback PromptCacheKey = %q, want %q", resp.Params.PromptCacheKey, wantFallbackKey)
	}
	if resp.Params.Model == "" {
		t.Fatal("expected effective model to be resolved")
	}

	resp, err = inter.PrepareContext(context.Background(), PrepareContextRequest{
		ConversationID: "conv-1",
		UserContent:    "oi",
		DefaultModel:   "fallback-model",
		Params: ChatParams{
			Model: "override-model",
		},
	})
	if err != nil {
		t.Fatalf("PrepareContext override: %v", err)
	}
	wantOverrideKey := ResolvePromptCacheHintKey(resp.ActiveProfile, slug, "conv-1", "override-model")
	if resp.Params.PromptCacheKey != wantOverrideKey {
		t.Fatalf("override PromptCacheKey = %q, want %q", resp.Params.PromptCacheKey, wantOverrideKey)
	}
	if wantFallbackKey == wantOverrideKey {
		t.Fatal("fallback and override keys should differ")
	}
}

func TestPrepareContextPromptCacheKeyUsesFallbackActiveProfileSlug(t *testing.T) {
	spy := &spyEmitter{}
	profileMgr := setupProfileTestEnv(t)

	profile := profiles.DefaultProfile()
	profile.Name = "Ativo"
	profile.Active = true
	profile.Chat.LLMProvider = "openai-default"
	profile.Chat.Model = "gpt-4o-mini"
	profile.Chat.PromptCache.Enabled = true
	profile.Chat.PromptCache.ProviderHints = true
	activeSlug, err := profileMgr.Create(profile)
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if err := profileMgr.SetActive(activeSlug); err != nil {
		t.Fatalf("set active: %v", err)
	}

	inter := NewInteractor(InteractorConfig{
		Emitter:    spy,
		ConvRepo:   noopConvRepo{},
		ProfileMgr: profileMgr,
	})

	resp, err := inter.PrepareContext(context.Background(), PrepareContextRequest{
		ConversationID: "conv-1",
		UserContent:    "oi",
		Params: ChatParams{
			ProfileSlug: "slug-inexistente",
		},
	})
	if err != nil {
		t.Fatalf("PrepareContext: %v", err)
	}
	if resp.Params.ProfileSlug != activeSlug {
		t.Fatalf("ProfileSlug = %q, want fallback active slug %q", resp.Params.ProfileSlug, activeSlug)
	}
	wantKey := ResolvePromptCacheHintKey(resp.ActiveProfile, activeSlug, "conv-1", resp.Params.Model)
	if resp.Params.PromptCacheKey != wantKey {
		t.Fatalf("PromptCacheKey = %q, want %q", resp.Params.PromptCacheKey, wantKey)
	}
}

func TestPrepareContextAppliesPromptCacheKeyFromProfile(t *testing.T) {
	spy := &spyEmitter{}
	profileMgr := setupProfileTestEnv(t)

	profile := profiles.DefaultProfile()
	profile.Name = "Cache"
	profile.Active = true
	profile.Chat.LLMProvider = "openai-default"
	profile.Chat.Model = "gpt-4o-mini"
	profile.Chat.PromptCache.Enabled = true
	profile.Chat.PromptCache.ProviderHints = true
	slug, err := profileMgr.Create(profile)
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if err := profileMgr.SetActive(slug); err != nil {
		t.Fatalf("set active: %v", err)
	}

	inter := NewInteractor(InteractorConfig{
		Emitter:    spy,
		ConvRepo:   noopConvRepo{},
		ProfileMgr: profileMgr,
	})

	resp, err := inter.PrepareContext(context.Background(), PrepareContextRequest{
		ConversationID: "conv-1",
		UserContent:    "oi",
	})
	if err != nil {
		t.Fatalf("PrepareContext: %v", err)
	}
	if resp.Params.PromptCacheKey == "" {
		t.Fatal("PromptCacheKey is empty")
	}

	profile.Chat.PromptCache.ProviderHints = false
	if err := profileMgr.Update(slug, profile); err != nil {
		t.Fatalf("update profile: %v", err)
	}
	resp, err = inter.PrepareContext(context.Background(), PrepareContextRequest{
		ConversationID: "conv-1",
		UserContent:    "oi",
	})
	if err != nil {
		t.Fatalf("PrepareContext after disabling hints: %v", err)
	}
	if resp.Params.PromptCacheKey != "" {
		t.Fatalf("PromptCacheKey = %q, want empty when provider_hints=false", resp.Params.PromptCacheKey)
	}
}

func TestHandlePromptCacheHintUnsupportedPersistsProviderHintsFalse(t *testing.T) {
	mgr := setupProfileTestEnv(t)
	profile := profiles.DefaultProfile()
	profile.Name = "Cache Hints"
	profile.Active = true
	profile.Chat.PromptCache.Enabled = true
	profile.Chat.PromptCache.ProviderHints = true
	slug, err := mgr.Create(profile)
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	inter := newAutoAdjustInteractor(mgr)

	inter.HandlePromptCacheHintUnsupported("", "gpt-4o-mini")

	got, err := mgr.Get(slug)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Chat.PromptCache.ProviderHints {
		t.Fatal("ProviderHints = true, want false")
	}
	if !got.Chat.PromptCache.Enabled {
		t.Fatal("Enabled was changed to false; only provider_hints should be disabled")
	}
}

func TestHandlePromptCacheHintUnsupportedUsesExplicitProfileSlug(t *testing.T) {
	mgr := setupProfileTestEnv(t)

	active := profiles.DefaultProfile()
	active.Name = "Ativo"
	active.Active = true
	active.Chat.PromptCache.Enabled = true
	active.Chat.PromptCache.ProviderHints = true
	activeSlug, err := mgr.Create(active)
	if err != nil {
		t.Fatalf("create active: %v", err)
	}

	sub := profiles.DefaultProfile()
	sub.Name = "Sub"
	sub.Active = false
	sub.Chat.PromptCache.Enabled = true
	sub.Chat.PromptCache.ProviderHints = true
	subSlug, err := mgr.Create(sub)
	if err != nil {
		t.Fatalf("create sub: %v", err)
	}

	inter := newAutoAdjustInteractor(mgr)
	inter.HandlePromptCacheHintUnsupported(subSlug, "modelo-sub")

	gotSub, err := mgr.Get(subSlug)
	if err != nil {
		t.Fatalf("get sub: %v", err)
	}
	if gotSub.Chat.PromptCache.ProviderHints {
		t.Fatal("sub ProviderHints = true, want false")
	}
	gotActive, err := mgr.Get(activeSlug)
	if err != nil {
		t.Fatalf("get active: %v", err)
	}
	if !gotActive.Chat.PromptCache.ProviderHints {
		t.Fatal("active ProviderHints changed unexpectedly")
	}
}
