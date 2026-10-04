package controllers

import (
	"assistente/internal/credentials"
	"assistente/internal/database"
	"context"
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
