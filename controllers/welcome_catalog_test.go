package controllers

import (
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
	"context"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"testing"
	"time"
)

func welcomeVaultDB(t *testing.T) *gorm.DB {
	t.Helper()
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
	return db
}

func TestWelcomePreparedVaultDoesNotOpenAnotherQuestionnaire(t *testing.T) {
	db := welcomeVaultDB(t)
	if err := db.Create(&database.CredentialKeyWrap{Kind: credentials.KeyWrapKindMaster}).Error; err != nil {
		t.Fatal(err)
	}
	mgr := questionnaire.NewManager(func(string, any) { t.Error("vault already prepared") })
	ctrl := NewWelcomeController(WelcomeControllerConfig{QuestionnaireMgr: mgr})
	ready, err := ctrl.RunWelcomeWizard(context.Background())
	if err != nil || !ready {
		t.Fatalf("ready=%v err=%v", ready, err)
	}
}

func TestWelcomePasswordMismatchStaysInVaultStepAndCanCancel(t *testing.T) {
	db := welcomeVaultDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	requests := 0
	var mgr *questionnaire.Manager
	mgr = questionnaire.NewManager(func(event string, data any) {
		if event != questionnaire.EventQuestionnaire {
			return
		}
		payload := data.(map[string]any)
		requests++
		if requests == 2 && payload["description"].(questionnaire.Text).Key != welcomeTextKey("passwordMismatch") {
			t.Error("missing localized mismatch")
		}
		if err := mgr.Respond(payload["id"].(string), map[string]any{"masterPassword": "different", "confirmPassword": "password"}, requests >= 2); err != nil {
			t.Error(err)
		}
	})
	ctrl := NewWelcomeController(WelcomeControllerConfig{QuestionnaireMgr: mgr})
	ready, err := ctrl.RunWelcomeWizard(ctx)
	if err != nil || ready || requests != 2 {
		t.Fatalf("ready=%v requests=%d err=%v", ready, requests, err)
	}
	var count int64
	if err := db.Model(&database.CredentialKeyWrap{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("cancelled preparation changed vault")
	}
}

func TestWelcomeVaultReadFailureDoesNotOfferToOverwriteIt(t *testing.T) {
	db := welcomeVaultDB(t)
	if err := db.Migrator().DropTable(&database.CredentialKeyWrap{}); err != nil {
		t.Fatal(err)
	}
	ctrl := NewWelcomeController(WelcomeControllerConfig{QuestionnaireMgr: questionnaire.NewManager(func(string, any) { t.Error("storage failure must not initiate setup") })})
	ready, err := ctrl.RunWelcomeWizard(context.Background())
	if err == nil || ready {
		t.Fatalf("ready=%v err=%v", ready, err)
	}
}
