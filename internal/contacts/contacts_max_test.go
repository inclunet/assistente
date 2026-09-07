package contacts

import (
	"errors"
	"path/filepath"
	"testing"

	"assistente/internal/channels"
	"assistente/internal/configdir"
	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTempHome(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	configdir.ResetForTests()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Cleanup(configdir.ResetForTests)
}

func setupMaxContactsDB(t *testing.T) {
	t.Helper()
	setupTempHome(t)
	resetContactsStoreForTests()

	path := filepath.Join(t.TempDir(), "contacts_max.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(
		&database.Channel{},
		&database.ChannelContact{},
		&database.ChannelContactConversation{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	channels.UseDatabase(db)
	UseDatabase(db)
	t.Cleanup(func() {
		resetContactsStoreForTests()
		channels.UseDatabase(nil)
		_ = sqlDB.Close()
	})

	if err := channels.Save("telegram", &channels.ChannelConfig{
		Enabled:     true,
		OwnerUserID: "user-1",
		Type:        "telegram",
		MaxContacts: -1,
	}); err != nil {
		t.Fatalf("Save channel: %v", err)
	}
}

func TestAuthorize_MaxContactsUnlimited(t *testing.T) {
	setupMaxContactsDB(t)

	if err := Authorize("telegram", "1", "A", "", -1); err != nil {
		t.Fatalf("authorize first: %v", err)
	}
	if err := Authorize("telegram", "2", "B", "", -1); err != nil {
		t.Fatalf("authorize second with unlimited max: %v", err)
	}

	has, allowed := IsAuthorized("telegram", -1, "2")
	if !has || !allowed {
		t.Fatalf("expected contact 2 authorized with unlimited max, has=%v allowed=%v", has, allowed)
	}
}

func TestAuthorize_MaxContactsZeroDefaultsToOne(t *testing.T) {
	setupMaxContactsDB(t)

	if err := Authorize("telegram", "1", "A", "", 0); err != nil {
		t.Fatalf("authorize first with max=0 (default 1): %v", err)
	}
	if err := Authorize("telegram", "2", "B", "", 0); err == nil {
		t.Fatal("expected error when max_contacts defaults to 1 already filled")
	}
}

func TestAuthorize_MaxContactsLimited(t *testing.T) {
	setupMaxContactsDB(t)

	if err := Authorize("telegram", "1", "A", "", 1); err != nil {
		t.Fatalf("authorize first: %v", err)
	}
	if err := Authorize("telegram", "2", "B", "", 1); err == nil {
		t.Fatal("expected error when max_contacts=1 already filled")
	}

	has, allowed := IsAuthorized("telegram", 1, "2")
	if !has || allowed {
		t.Fatalf("expected contact 2 rejected at limit, has=%v allowed=%v", has, allowed)
	}
}

func TestRuntimeAPIs_RequireDatabase(t *testing.T) {
	resetContactsStoreForTests()
	t.Cleanup(resetContactsStoreForTests)

	if err := Authorize("telegram", "1", "A", "", 1); !errors.Is(err, ErrDBNotEnabled) {
		t.Fatalf("Authorize sem DB: got %v, want ErrDBNotEnabled", err)
	}
	if _, err := Load(); !errors.Is(err, ErrDBNotEnabled) {
		t.Fatalf("Load sem DB: got %v, want ErrDBNotEnabled", err)
	}
	has, allowed := IsAuthorized("telegram", 1, "1")
	if !has || allowed {
		t.Fatalf("IsAuthorized sem DB deve rejeitar (true,false); got (%v,%v)", has, allowed)
	}
}
