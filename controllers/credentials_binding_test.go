package controllers

import (
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/providers"
	"bytes"
	"context"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCredentialForURLPreservesEffectivePatternWithoutMaterializing(t *testing.T) {
	for _, pattern := range []string{"EXAMPLE.COM", "*.example.com", "::1"} {
		t.Run(pattern, func(t *testing.T) {
			manager := credentials.NewManager(nil)
			ctx := database.WithUserID(context.Background(), "user")
			auth := &credentials.AuthConfig{Source: "command", Type: "bearer", SourceConfig: &credentials.SourceConfig{Command: "nonexistent-command-must-not-run", TimeoutSeconds: 30}}
			if err := manager.RegisterPatternWithContext(ctx, pattern, auth); err != nil {
				t.Fatal(err)
			}
			resource := "https://example.com/mcp"
			if pattern == "*.example.com" {
				resource = "https://api.example.com/mcp"
				if err := manager.RegisterPatternWithContext(ctx, "api.example.com", auth); err != nil {
					t.Fatal(err)
				}
			}
			if pattern == "::1" {
				resource = "http://[::1]:3000/mcp"
			}
			ctrl := NewCredentialsController(CredentialsControllerConfig{CredMgr: manager})
			summary, err := ctrl.GetCredentialForURL(ctx, resource)
			if err != nil || summary == nil || summary.Pattern != pattern || summary.Source != "command" {
				t.Fatalf("binding: %+v %v", summary, err)
			}
			if _, err = ctrl.GetCredentialForURL(context.Background(), resource); err == nil {
				t.Fatal("unscoped access")
			}
			summary, err = ctrl.GetCredentialForURL(database.WithUserID(context.Background(), "other"), resource)
			if err != nil || summary == nil || summary.Source != "static" || summary.SourceConfig != nil {
				t.Fatalf("cross-user access: %+v %v", summary, err)
			}
		})
	}
}

func TestCredentialForURLRejectsAmbiguousPattern(t *testing.T) {
	manager := credentials.NewManager(nil)
	ctx := database.WithUserID(context.Background(), "user")
	for _, pattern := range []string{"EXAMPLE.COM", "example.com"} {
		if err := manager.RegisterPatternWithContext(ctx, pattern, &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "private"}); err != nil {
			t.Fatal(err)
		}
	}
	ctrl := NewCredentialsController(CredentialsControllerConfig{CredMgr: manager})
	if summary, err := ctrl.GetCredentialForURL(ctx, "https://example.com"); err == nil || summary != nil {
		t.Fatalf("ambiguous lookup: %+v %v", summary, err)
	}
}

func TestCredentialForURLNewDraftUsesRuntimeHostname(t *testing.T) {
	ctrl := NewCredentialsController(CredentialsControllerConfig{CredMgr: credentials.NewManager(nil)})
	ctx := database.WithUserID(context.Background(), "user")
	for resource, pattern := range map[string]string{"https://café.example/mcp": "café.example", "http://[::1]:3000/mcp": "::1"} {
		summary, err := ctrl.GetCredentialForURL(ctx, resource)
		if err != nil || summary == nil || summary.Pattern != pattern || summary.Source != "static" {
			t.Fatalf("new draft: %+v %v", summary, err)
		}
	}
	entries, err := ctrl.ListCredentials(ctx)
	if err != nil || len(entries) != 0 {
		t.Fatal("draft lookup created credential")
	}
}

func TestCredentialMetadataFlowsThroughProviderPreviewCreateAndDuplicate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&database.LLMProvider{}, &database.CredentialEntry{}); err != nil {
		t.Fatal(err)
	}
	previous := database.DB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(previous); sqlDB, _ := db.DB(); _ = sqlDB.Close() })
	ctx := database.WithUserID(context.Background(), "owner")
	mgr := credentials.NewManagerWithStore(bytes.Repeat([]byte{4}, 32), credentials.NewDBStore(), true)
	registry := llm.NewProviderRegistry()
	store := providers.NewDBStore()
	service := providers.NewService(providers.ServiceConfig{Registry: registry, CredMgr: mgr, Store: store})
	consumer := NewLLMController(LLMControllerConfig{LLMRegistry: registry, ProviderSvc: service})
	vault := NewCredentialsController(CredentialsControllerConfig{CredMgr: mgr})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Error("missing credential")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"model"}]}`))
	}))
	defer server.Close()
	resource := strings.Replace(server.URL, "127.0.0.1", "LOCALHOST", 1)
	summary, err := vault.GetCredentialForURL(ctx, resource)
	if err != nil || summary.Pattern != "localhost" {
		t.Fatalf("metadata: %v %v", summary, err)
	}
	input := &CredentialInput{Pattern: summary.Pattern, Type: summary.Type, Source: summary.Source, Token: "token"}
	models, err := consumer.ListModelsRaw(ctx, TestLLMProviderRequest{Type: "custom", APIFormat: "openai", BaseURL: resource, Credential: input})
	if err != nil || len(models) != 1 {
		t.Fatalf("preview: %v %v", models, err)
	}
	created, err := consumer.CreateLLMProvider(ctx, CreateLLMProviderRequest{ID: "original", Name: "Original", Type: "custom", APIFormat: "openai", BaseURL: resource, Credential: input})
	if err != nil || created["credential_pattern"] != "localhost" {
		t.Fatalf("create: %v %v", created, err)
	}
	// Move the fixture to a non-hostname alias, then duplicate through the DTO/controller.
	original, err := store.Get(ctx, "original")
	if err != nil {
		t.Fatal(err)
	}
	original.CredentialPattern = "shared-alias"
	if err := mgr.RegisterPatternWithContext(ctx, "shared-alias", &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "alias-secret"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, []*llm.ProviderConfig{original}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(original); err != nil {
		t.Fatal(err)
	}
	copied, err := consumer.CreateLLMProvider(ctx, CreateLLMProviderRequest{ID: "copy", Name: "Copy", Type: "custom", APIFormat: "openai", BaseURL: resource, CredentialFromProviderID: "original"})
	if err != nil || copied["credential_pattern"] != "shared-alias" {
		t.Fatalf("copy: %v %v", copied, err)
	}
	auth, err := mgr.GetByPatternWithContext(ctx, "shared-alias")
	if err != nil || auth.Token != "alias-secret" {
		t.Fatal("duplicate rewrote credential")
	}
}
