package channels

import (
	"testing"

	"assistente/internal/configdir"
)

func setupTempHome(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	configdir.ResetForTests()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Cleanup(configdir.ResetForTests)
}

// TestAdoptOrphans valida o fix do Blocker D do re-review do AEP-0052:
// canais sem OwnerUserID (configs pré-AEP-0052) recebem o userID do
// primeiro usuário durante AdoptLegacyData. Canais já com dono não são
// sobrescritos, mesmo se o dono atual for outro usuário.
func TestAdoptOrphans(t *testing.T) {
	setupTempHome(t)

	if err := Save("telegram", &ChannelConfig{Enabled: true, MaxContacts: 1}); err != nil {
		t.Fatalf("save telegram: %v", err)
	}
	if err := Save("signal", &ChannelConfig{Enabled: true, MaxContacts: 1, OwnerUserID: "user-leo"}); err != nil {
		t.Fatalf("save signal: %v", err)
	}
	if err := Save("slack", &ChannelConfig{Enabled: false}); err != nil {
		t.Fatalf("save slack: %v", err)
	}

	migrated, err := AdoptOrphans("user-ana")
	if err != nil {
		t.Fatalf("adopt orphans: %v", err)
	}
	if len(migrated) != 2 {
		t.Fatalf("esperava 2 canais migrados (telegram, slack), got %v", migrated)
	}

	tg, err := Load("telegram")
	if err != nil || tg == nil {
		t.Fatalf("load telegram: %v", err)
	}
	if tg.OwnerUserID != "user-ana" {
		t.Fatalf("telegram OwnerUserID = %q, esperava user-ana", tg.OwnerUserID)
	}

	sl, err := Load("slack")
	if err != nil || sl == nil {
		t.Fatalf("load slack: %v", err)
	}
	if sl.OwnerUserID != "user-ana" {
		t.Fatalf("slack OwnerUserID = %q, esperava user-ana", sl.OwnerUserID)
	}

	sg, err := Load("signal")
	if err != nil || sg == nil {
		t.Fatalf("load signal: %v", err)
	}
	if sg.OwnerUserID != "user-leo" {
		t.Fatalf("signal OwnerUserID foi sobrescrito de user-leo para %q (AdoptOrphans não pode tocar canais com dono já definido)", sg.OwnerUserID)
	}
}

func TestAdoptOrphans_RequiresUserID(t *testing.T) {
	setupTempHome(t)

	if _, err := AdoptOrphans(""); err == nil {
		t.Fatal("esperava erro ao chamar AdoptOrphans com userID vazio")
	}
}

func TestAdoptOrphans_Idempotent(t *testing.T) {
	setupTempHome(t)

	if err := Save("telegram", &ChannelConfig{Enabled: true}); err != nil {
		t.Fatalf("save: %v", err)
	}

	first, err := AdoptOrphans("user-ana")
	if err != nil {
		t.Fatalf("first adopt: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("first adopt esperava 1 migrado, got %v", first)
	}

	second, err := AdoptOrphans("user-ana")
	if err != nil {
		t.Fatalf("second adopt: %v", err)
	}
	if len(second) != 0 {
		t.Fatalf("segunda chamada não deveria migrar nada (canal já tem dono), got %v", second)
	}
}
