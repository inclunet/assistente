package controllers

import (
	"context"
	"time"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/llmcatalog"
	"assistente/internal/providers"
	"assistente/internal/questionnaire"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
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

func TestWelcomeCatalogErrorAllowsAnotherProvider(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	previous := database.DB()
	database.SetDB(db)
	t.Cleanup(func() {
		database.SetDB(previous)
		if err := sqlDB.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := db.AutoMigrate(&database.CredentialKeyWrap{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.CredentialKeyWrap{Kind: credentials.KeyWrapKindMaster}).Error; err != nil {
		t.Fatal(err)
	}
	for _, choice := range []string{"OpenAI", "Google (Gemini)", wizardProviderOllama} {
		t.Run(choice, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			requests := 0
			var manager *questionnaire.Manager
			manager = questionnaire.NewManager(func(event string, data any) {
				if event != questionnaire.EventQuestionnaire {
					return
				}
				payload := data.(map[string]any)
				questions := payload["questions"].([]questionnaire.Question)
				description := payload["description"].(questionnaire.Text)
				requests++
				answers := map[string]any{}
				cancelled := false
				switch requests {
				case 1:
					answers["provider"] = choice
				case 2:
					if questions[0].ID != "provider" || questions[0].Default != choice || description.Key != welcomeTextKey("providerUnavailable") {
						t.Errorf("provider selection did not retain choice and localized error: %+v", payload)
					}
					answers["provider"] = wizardProviderOther
				case 3:
					if questions[0].ID != "baseURL" {
						t.Errorf("manual configuration not reached: %+v", payload)
					}
					cancelled = true
				case 4:
					if questions[0].ID != "provider" || description.Key != welcomeTextKey("providerDescription") {
						t.Errorf("previous catalog error was not cleared: %+v", payload)
					}
					cancelled = true
				default:
					t.Error("unexpected wizard step")
					cancelled = true
				}
				if err := manager.Respond(payload["id"].(string), answers, cancelled); err != nil {
					t.Error(err)
				}
			})
			svc := providers.NewService(providers.ServiceConfig{})
			controller := NewWelcomeController(WelcomeControllerConfig{ProviderSvc: svc, QuestionnaireMgr: manager})
			completed, err := controller.RunWelcomeWizard(ctx)
			if err != nil || completed || requests != 4 {
				t.Fatalf("wizard ended unexpectedly: completed=%v requests=%d err=%v", completed, requests, err)
			}
		})
	}
}
