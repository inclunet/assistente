package acp

import (
	"context"
	"testing"

	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupStoreTest(t *testing.T) (SessionStore, *gorm.DB, context.Context, context.Context) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("abrir banco: %v", err)
	}
	if err := db.AutoMigrate(&database.User{}, &database.ACPSession{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return NewDBSessionStore(db), db,
		database.WithUserID(context.Background(), "user-ana"),
		database.WithUserID(context.Background(), "user-leo")
}

func contaSessoes(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var total int64
	if err := db.Model(&database.ACPSession{}).Count(&total).Error; err != nil {
		t.Fatalf("contar sessões: %v", err)
	}
	return total
}

func TestSemBancoNaoSobraPonteiroNuloDisfarcadoDeStore(t *testing.T) {
	// Devolver (*DBSessionStore)(nil) passaria pelo teste de nulidade do
	// manager e estouraria na primeira gravação.
	if store := NewDBSessionStore(nil); store != nil {
		t.Fatalf("store sem banco = %#v, esperado nil", store)
	}
	m := NewManager(ManagerConfig{
		Store:   NewDBSessionStore(nil),
		WorkDir: func() (string, error) { return "/projeto", nil },
		Dial: func(Config, RequestHandler) (Client, error) {
			return newFakeManagedClient(), nil
		},
	})
	conv, err := m.Conversation(context.Background(), testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("conversa sem banco: %v", err)
	}
	if conv.Session() == nil {
		t.Fatal("conversa sem banco ficou sem sessão")
	}
}

func TestVinculoDaSessaoSobreviveAoIdaEVoltaDoBanco(t *testing.T) {
	store, db, ana, _ := setupStoreTest(t)

	rec := StoredSession{
		ConversationID: "conv-1",
		ProviderID:     "cursor",
		SessionID:      "sess-abc",
		PrefixHash:     "hash-persona",
		WorkDir:        "/projeto",
	}
	if err := store.Save(ana, rec); err != nil {
		t.Fatalf("gravar: %v", err)
	}

	got, err := store.Load(ana, "conv-1", "cursor")
	if err != nil {
		t.Fatalf("ler: %v", err)
	}
	if got == nil {
		t.Fatal("vínculo gravado não foi encontrado")
	}
	if *got != rec {
		t.Fatalf("vínculo lido = %+v, esperado %+v", *got, rec)
	}

	// Trocar de sessão na mesma conversa substitui o registro; duas linhas
	// deixariam o app sem saber qual sessão é a viva.
	rec.SessionID = "sess-def"
	rec.PrefixHash = ""
	if err := store.Save(ana, rec); err != nil {
		t.Fatalf("regravar: %v", err)
	}
	if total := contaSessoes(t, db); total != 1 {
		t.Fatalf("linhas = %d, esperado 1 por conversa e provider", total)
	}
	got, err = store.Load(ana, "conv-1", "cursor")
	if err != nil || got == nil {
		t.Fatalf("ler após regravar: %v", err)
	}
	if got.SessionID != "sess-def" || got.PrefixHash != "" {
		t.Fatalf("registro não foi substituído: %+v", *got)
	}
}

func TestConversaSemSessaoRegistradaNaoEhErro(t *testing.T) {
	store, _, ana, _ := setupStoreTest(t)

	got, err := store.Load(ana, "conv-nova", "cursor")
	if err != nil {
		t.Fatalf("ler conversa sem sessão: %v", err)
	}
	if got != nil {
		t.Fatalf("conversa sem sessão devolveu %+v", *got)
	}
}

func TestSessaoDeUmUsuarioNaoApareceParaOutro(t *testing.T) {
	store, _, ana, leo := setupStoreTest(t)

	if err := store.Save(ana, StoredSession{
		ConversationID: "conv-1",
		ProviderID:     "cursor",
		SessionID:      "sess-da-ana",
		WorkDir:        "/projeto",
	}); err != nil {
		t.Fatalf("gravar ana: %v", err)
	}

	got, err := store.Load(leo, "conv-1", "cursor")
	if err != nil {
		t.Fatalf("ler leo: %v", err)
	}
	if got != nil {
		t.Fatalf("sessão da ana vazou para o leo: %+v", *got)
	}

	// E o mesmo par conversa+provider pode existir para os dois.
	if err := store.Save(leo, StoredSession{
		ConversationID: "conv-1",
		ProviderID:     "cursor",
		SessionID:      "sess-do-leo",
		WorkDir:        "/projeto",
	}); err != nil {
		t.Fatalf("gravar leo: %v", err)
	}
	daAna, err := store.Load(ana, "conv-1", "cursor")
	if err != nil || daAna == nil {
		t.Fatalf("ler ana após gravar leo: %v", err)
	}
	if daAna.SessionID != "sess-da-ana" {
		t.Fatalf("sessão da ana foi sobrescrita: %+v", *daAna)
	}
}

func TestSemUsuarioNoContextoOStoreFalhaFechado(t *testing.T) {
	store, _, _, _ := setupStoreTest(t)
	semUsuario := context.Background()

	if _, err := store.Load(semUsuario, "conv-1", "cursor"); err == nil {
		t.Fatal("leitura sem usuário no contexto passou")
	}
	if err := store.Save(semUsuario, StoredSession{
		ConversationID: "conv-1",
		ProviderID:     "cursor",
		SessionID:      "sess-abc",
	}); err == nil {
		t.Fatal("escrita sem usuário no contexto passou")
	}
	if err := store.SavePrefixHash(semUsuario, "conv-1", "cursor", "hash"); err == nil {
		t.Fatal("anotação de prefixo sem usuário no contexto passou")
	}
	if err := store.Delete(semUsuario, "conv-1"); err == nil {
		t.Fatal("exclusão sem usuário no contexto passou")
	}
	if err := store.DeleteAll(semUsuario); err == nil {
		t.Fatal("limpeza geral sem usuário no contexto passou")
	}
}

func TestLimparTudoNaoPassaPorCimaDasSessoesDeOutroUsuario(t *testing.T) {
	store, _, ana, leo := setupStoreTest(t)

	for _, conversa := range []string{"conv-1", "conv-2"} {
		if err := store.Save(ana, StoredSession{
			ConversationID: conversa,
			ProviderID:     "cursor",
			SessionID:      "sess-" + conversa,
			WorkDir:        "/projeto",
		}); err != nil {
			t.Fatalf("gravar %s: %v", conversa, err)
		}
	}
	if err := store.Save(leo, StoredSession{
		ConversationID: "conv-1",
		ProviderID:     "cursor",
		SessionID:      "sess-do-leo",
		WorkDir:        "/projeto",
	}); err != nil {
		t.Fatalf("gravar leo: %v", err)
	}

	if err := store.DeleteAll(ana); err != nil {
		t.Fatalf("limpar tudo: %v", err)
	}

	for _, conversa := range []string{"conv-1", "conv-2"} {
		got, err := store.Load(ana, conversa, "cursor")
		if err != nil {
			t.Fatalf("ler %s após limpar: %v", conversa, err)
		}
		if got != nil {
			t.Fatalf("sessão de %s sobreviveu à limpeza geral", conversa)
		}
	}
	doLeo, err := store.Load(leo, "conv-1", "cursor")
	if err != nil || doLeo == nil {
		t.Fatalf("a limpeza de uma pessoa levou junto a sessão da outra: %v", err)
	}
}

func TestApagarConversaLevaAsSessoesDeTodosOsProviders(t *testing.T) {
	store, _, ana, _ := setupStoreTest(t)

	for _, provider := range []string{"cursor", "claude-code"} {
		if err := store.Save(ana, StoredSession{
			ConversationID: "conv-1",
			ProviderID:     provider,
			SessionID:      "sess-" + provider,
			WorkDir:        "/projeto",
		}); err != nil {
			t.Fatalf("gravar %s: %v", provider, err)
		}
	}
	if err := store.Save(ana, StoredSession{
		ConversationID: "conv-2",
		ProviderID:     "cursor",
		SessionID:      "sess-outra",
		WorkDir:        "/projeto",
	}); err != nil {
		t.Fatalf("gravar outra conversa: %v", err)
	}

	if err := store.Delete(ana, "conv-1"); err != nil {
		t.Fatalf("apagar conversa: %v", err)
	}
	for _, provider := range []string{"cursor", "claude-code"} {
		got, err := store.Load(ana, "conv-1", provider)
		if err != nil {
			t.Fatalf("ler %s após apagar: %v", provider, err)
		}
		if got != nil {
			t.Fatalf("sessão de %s sobreviveu à exclusão da conversa", provider)
		}
	}
	sobrou, err := store.Load(ana, "conv-2", "cursor")
	if err != nil || sobrou == nil {
		t.Fatalf("apagar uma conversa levou junto a sessão da outra: %v", err)
	}
}

func TestAnotarPrefixoExigeSessaoRegistrada(t *testing.T) {
	store, _, ana, _ := setupStoreTest(t)

	if err := store.SavePrefixHash(ana, "conv-1", "cursor", "hash-persona"); err == nil {
		t.Fatal("prefixo anotado para uma sessão que não existe")
	}

	if err := store.Save(ana, StoredSession{
		ConversationID: "conv-1",
		ProviderID:     "cursor",
		SessionID:      "sess-abc",
		WorkDir:        "/projeto",
	}); err != nil {
		t.Fatalf("gravar: %v", err)
	}
	if err := store.SavePrefixHash(ana, "conv-1", "cursor", "hash-persona"); err != nil {
		t.Fatalf("anotar prefixo: %v", err)
	}

	got, err := store.Load(ana, "conv-1", "cursor")
	if err != nil || got == nil {
		t.Fatalf("ler após anotar: %v", err)
	}
	if got.PrefixHash != "hash-persona" {
		t.Fatalf("prefixo anotado = %q", got.PrefixHash)
	}
	if got.SessionID != "sess-abc" {
		t.Fatalf("anotar o prefixo mexeu na sessão: %q", got.SessionID)
	}
}
