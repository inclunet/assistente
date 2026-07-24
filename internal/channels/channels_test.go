package channels

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestGetMaxContacts(t *testing.T) {
	t.Parallel()

	if got := (*ChannelConfig)(nil).GetMaxContacts(); got != 1 {
		t.Fatalf("nil config: got %d, want 1", got)
	}
	if got := (&ChannelConfig{}).GetMaxContacts(); got != 1 {
		t.Fatalf("omitido/zero: got %d, want 1 (legado single-contact)", got)
	}
	if got := (&ChannelConfig{MaxContacts: 3}).GetMaxContacts(); got != 3 {
		t.Fatalf("positivo: got %d, want 3", got)
	}
	if got := (&ChannelConfig{MaxContacts: -1}).GetMaxContacts(); got != -1 {
		t.Fatalf("negativo: got %d, want -1 (ilimitado)", got)
	}
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

// TestSave_FileMode0600 valida o vetor B8 do review original e P1-3
// do re-review: configs de canal podem conter tokens em texto plano
// (BotToken, AppToken, APIToken) quando o credential manager está
// indisponível. Em hosts POSIX shared (containers, multi-user), 0644
// deixaria os tokens world-readable. Save() deve persistir o arquivo
// com 0600 e o diretório com 0700.
//
// Pulamos no Windows porque os.WriteFile não traduz POSIX modes para
// ACLs nativas — o teste serve como gate para Linux/macOS, onde a
// regressão importa.
func TestSave_FileMode0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissões POSIX 0600/0700 não traduzem direto para ACLs do Windows")
	}
	setupTempHome(t)

	if err := Save("telegram", &ChannelConfig{
		Enabled:     true,
		OwnerUserID: "u1",
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	dir := channelsHomeDir()
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0700 {
		t.Fatalf("dir mode: want 0700, got %#o", got)
	}

	path := filepath.Join(dir, "telegram.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("file mode: want 0600, got %#o", got)
	}
}

// TestLoad_CorruptedJSONReturnsError valida o vetor M9 do review
// original e P1-3 do re-review: antes do fix, JSON corrompido fazia
// o canal "sumir" da lista — combinado com AdoptOrphans/gateway
// virava um disabled invisível, sem feedback ao operador. Agora
// Load() propaga o erro de parse com a substring "corrompido" para
// que healthchecks possam sinalizar na UI.
func TestLoad_CorruptedJSONReturnsError(t *testing.T) {
	setupTempHome(t)

	dir := channelsHomeDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "telegram.json")
	if err := os.WriteFile(path, []byte("{ invalid json"), 0600); err != nil {
		t.Fatalf("write corrupt: %v", err)
	}

	cfg, err := Load("telegram")
	if err == nil {
		t.Fatalf("Load deveria propagar JSON corrompido, got cfg=%+v", cfg)
	}
	if !strings.Contains(err.Error(), "corrompido") {
		t.Fatalf("erro deveria mencionar 'corrompido', got %q", err.Error())
	}
	if cfg != nil {
		t.Fatalf("Load corrompido deveria devolver cfg=nil, got %+v", cfg)
	}
}
