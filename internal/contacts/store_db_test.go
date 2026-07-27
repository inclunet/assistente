package contacts

import (
	"path/filepath"
	"testing"

	"assistente/internal/channels"
	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func resetContactsStoreForTests() {
	mu.Lock()
	defer mu.Unlock()
	storeDB = nil
}

func setupContactsDB(t *testing.T) {
	t.Helper()
	resetContactsStoreForTests()

	path := filepath.Join(t.TempDir(), "contacts.db")
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
		DisplayName: "Telegram",
		MaxContacts: 2,
	}); err != nil {
		t.Fatalf("Save channel: %v", err)
	}
}

func TestContactsDB_AuthorizeIsAuthorizedRemove(t *testing.T) {
	setupContactsDB(t)

	if err := Authorize("telegram", "42", "Fulano", "fulano", 2); err != nil {
		t.Fatalf("Authorize: %v", err)
	}

	has, ok := IsAuthorized("telegram", 2, "42")
	if !has || !ok {
		t.Fatalf("IsAuthorized esperado (true,true); got (%v,%v)", has, ok)
	}

	list, err := GetForChannel("telegram")
	if err != nil || len(list) != 1 {
		t.Fatalf("GetForChannel: err=%v len=%d", err, len(list))
	}
	if list[0].ID != "42" || list[0].DisplayName != "Fulano" {
		t.Fatalf("contato inesperado: %+v", list[0])
	}

	if err := Authorize("telegram", "99", "Beltrano", "", 2); err != nil {
		t.Fatalf("Authorize 2: %v", err)
	}
	if err := Authorize("telegram", "100", "Excesso", "", 2); err == nil {
		t.Fatal("Authorize além do maxContacts deveria falhar")
	}

	has, ok = IsAuthorized("telegram", 2, "desconhecido")
	if !has || ok {
		t.Fatalf("no limite, desconhecido deve ser (true,false); got (%v,%v)", has, ok)
	}

	if err := Remove("telegram", "42"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	list, err = GetForChannel("telegram")
	if err != nil || len(list) != 1 || list[0].ID != "99" {
		t.Fatalf("após Remove: err=%v list=%v", err, list)
	}

	if got := Count("telegram"); got != 1 {
		t.Fatalf("Count: got=%d", got)
	}
}

func TestContactsDB_EmptyChannel(t *testing.T) {
	setupContactsDB(t)
	list, err := GetForChannel("signal")
	if err != nil || len(list) != 0 {
		t.Fatalf("canal inexistente deve retornar lista vazia; err=%v list=%v", err, list)
	}
	has, ok := IsAuthorized("signal", 1, "x")
	if has || ok {
		t.Fatalf("sem canal: esperado (false,false); got (%v,%v)", has, ok)
	}
}
