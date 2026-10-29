package controllers

import (
	"assistente/internal/llmcatalog"
	"assistente/internal/providers"
	"testing"
)

func TestWelcomeCatalogRejectsIncompatibleNativeRoute(t *testing.T) {
	for _, test := range []struct {
		choice, kind, id, family, format, url string
		accepted                              bool
	}{
		{"OpenAI", "openai", "openai", "openai", "openai_responses", "https://api.openai.com/v1", true},
		{"Google (Gemini)", "google", "google-gemini", "google", "google", "https://generativelanguage.googleapis.com/v1beta", false},
	} {
		t.Run(test.kind, func(t *testing.T) {
			server := llmcatalog.Server{ID: test.id, ProviderFamily: test.family, Name: test.choice, APIFormat: test.format, BaseURL: test.url, Status: "active", Auth: llmcatalog.Auth{Mode: "bearer_api_key"}}
			svc := providers.NewService(providers.ServiceConfig{CatalogSnapshot: func() llmcatalog.Snapshot {
				return llmcatalog.Snapshot{Document: llmcatalog.Document{Servers: []llmcatalog.Server{server}}}
			}})
			controller := NewWelcomeController(WelcomeControllerConfig{ProviderSvc: svc})
			got, err := controller.wizardProviderTemplate(test.choice, test.kind)
			if test.accepted {
				if err != nil || got.BaseURL != test.url {
					t.Fatalf("catalog URL not used: %v", err)
				}
			} else if err == nil || got != nil {
				t.Fatal("native route accepted by legacy OpenAI wizard")
			}
		})
	}
}
