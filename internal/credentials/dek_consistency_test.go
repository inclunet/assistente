package credentials

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

// TestDEKIdentity_DeterministicAndDistinct prova as garantias mínimas
// que o resto da arquitetura confia (ver AEP-0061): mesma DEK gera
// mesmo ID, DEKs distintas geram IDs distintos, DEK vazia → string vazia.
func TestDEKIdentity_DeterministicAndDistinct(t *testing.T) {
	dek1 := []byte("01234567890123456789012345678901")
	dek2 := []byte("abcdefghijabcdefghijabcdefghij01")

	if got := DEKIdentity(nil); got != "" {
		t.Fatalf("DEKIdentity(nil) = %q, want empty string", got)
	}
	id1a := DEKIdentity(dek1)
	id1b := DEKIdentity(dek1)
	if id1a == "" || id1a != id1b {
		t.Fatalf("DEKIdentity não-determinístico ou vazio: %q vs %q", id1a, id1b)
	}
	if len(id1a) != 32 {
		t.Fatalf("DEKIdentity deveria ser hex 32 chars (16 bytes), got %d: %q", len(id1a), id1a)
	}
	if got := DEKIdentity(dek2); got == id1a {
		t.Fatalf("DEKs distintas geraram MESMO id: %q (colisão grave)", got)
	}
}

// memoryConsistencyStore é um Store in-memory dedicado a testes do
// caminho de consistência (não pode reutilizar memoryCredentialStore
// do pacote auth porque está noutro pacote).
type memoryConsistencyStore struct {
	wraps       map[string]KeyWrap
	credentials []StoredCredential
}

func newMemoryConsistencyStore() *memoryConsistencyStore {
	return &memoryConsistencyStore{wraps: map[string]KeyWrap{}}
}

func (s *memoryConsistencyStore) SaveCredential(_ context.Context, cred StoredCredential) error {
	if cred.ID == "" {
		cred.ID = cred.Pattern + ":" + cred.UserID
	}
	for i, existing := range s.credentials {
		if existing.ID == cred.ID || (existing.Pattern == cred.Pattern && existing.UserID == cred.UserID) {
			s.credentials[i] = cred
			return nil
		}
	}
	s.credentials = append(s.credentials, cred)
	return nil
}

func (s *memoryConsistencyStore) ListCredentials(_ context.Context) ([]StoredCredential, error) {
	return s.credentials, nil
}

func (s *memoryConsistencyStore) DeleteCredential(_ context.Context, pattern string) error {
	out := s.credentials[:0]
	for _, c := range s.credentials {
		if c.Pattern != pattern {
			out = append(out, c)
		}
	}
	s.credentials = out
	return nil
}

func (s *memoryConsistencyStore) SaveKeyWrap(_ context.Context, wrap KeyWrap) error {
	s.wraps[wrap.Kind] = wrap
	return nil
}

func (s *memoryConsistencyStore) GetKeyWrap(_ context.Context, kind string) (*KeyWrap, error) {
	if wrap, ok := s.wraps[kind]; ok {
		copy := wrap
		return &copy, nil
	}
	return nil, nil
}

func (s *memoryConsistencyStore) HasKeyWrap(_ context.Context, kind string) (bool, error) {
	_, ok := s.wraps[kind]
	return ok, nil
}

func (s *memoryConsistencyStore) ListAllCredentialsIgnoringScope(_ context.Context) ([]StoredCredential, error) {
	return s.credentials, nil
}

func (s *memoryConsistencyStore) DeleteCredentialsByID(_ context.Context, ids []string) (int, error) {
	out := s.credentials[:0]
	removed := 0
	idSet := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		idSet[id] = struct{}{}
	}
	for _, c := range s.credentials {
		if _, drop := idSet[c.ID]; drop {
			removed++
			continue
		}
		out = append(out, c)
	}
	s.credentials = out
	return removed, nil
}

// TestPersistDEKConsistent_RecusaSobrescreverDEKDivergente cobre o
// caminho central da defesa AEP-0061: tentar gravar uma DEK diferente
// da que está no keychain DEVE retornar `ErrDEKWouldOverwrite` em vez
// de sobrescrever silenciosamente. Era o caminho que historicamente
// destruiu credenciais quando código antigo chamava SaveDEKToKeychain
// sem verificar o estado prévio.
func TestPersistDEKConsistent_RecusaSobrescreverDEKDivergente(t *testing.T) {
	store := newMemoryConsistencyStore()
	dekX := []byte("XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX")
	dekY := []byte("YYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYY")

	loaded := dekX
	saved := []byte(nil)
	loadKeyring := func() ([]byte, error) { return loaded, nil }
	saveKeyring := func(d []byte) error {
		saved = append([]byte(nil), d...)
		return nil
	}

	err := PersistDEKConsistent(context.Background(), store, dekY, loadKeyring, saveKeyring)
	if !errors.Is(err, ErrDEKWouldOverwrite) {
		t.Fatalf("esperava ErrDEKWouldOverwrite, got %v", err)
	}
	if saved != nil {
		t.Fatalf("saveKeyring foi chamado mesmo com DEK divergente — defesa ineficaz")
	}
}

// TestPersistDEKConsistent_NoOpQuandoMesmaDEK garante a idempotência
// do helper: chamar com a DEK que já está no keychain não recusa nem
// re-grava, mas adota o `dek_id` em wraps que ainda estejam vazios
// (caminho de migração para wraps pré-AEP-0061).
func TestPersistDEKConsistent_NoOpQuandoMesmaDEK(t *testing.T) {
	store := newMemoryConsistencyStore()
	store.wraps[KeyWrapKindMaster] = KeyWrap{Kind: KeyWrapKindMaster, DekID: ""}

	dek := []byte("11111111111111111111111111111111")
	loaded := dek
	saveCalls := 0
	loadKeyring := func() ([]byte, error) { return loaded, nil }
	saveKeyring := func([]byte) error { saveCalls++; return nil }

	if err := PersistDEKConsistent(context.Background(), store, dek, loadKeyring, saveKeyring); err != nil {
		t.Fatalf("PersistDEKConsistent: %v", err)
	}
	if saveCalls != 0 {
		t.Fatalf("não deveria reescrever keychain quando DEKs batem; calls=%d", saveCalls)
	}
	if got := store.wraps[KeyWrapKindMaster].DekID; got != DEKIdentity(dek) {
		t.Fatalf("dek_id do wrap não foi populado: got=%q want=%q", got, DEKIdentity(dek))
	}
}

// TestPersistDEKConsistent_GravaQuandoKeychainVazio cobre o caminho
// de "Setup inicial" / "primeira execução em nova máquina": keychain
// retorna ErrNotFound, helper grava normalmente e popula dek_id em
// wraps existentes.
func TestPersistDEKConsistent_GravaQuandoKeychainVazio(t *testing.T) {
	store := newMemoryConsistencyStore()
	store.wraps[KeyWrapKindMaster] = KeyWrap{Kind: KeyWrapKindMaster, DekID: ""}

	dek := []byte("22222222222222222222222222222222")
	saveCalls := 0
	loadKeyring := func() ([]byte, error) { return nil, keyring.ErrNotFound }
	saveKeyring := func(d []byte) error { saveCalls++; return nil }

	if err := PersistDEKConsistent(context.Background(), store, dek, loadKeyring, saveKeyring); err != nil {
		t.Fatalf("PersistDEKConsistent: %v", err)
	}
	if saveCalls != 1 {
		t.Fatalf("esperava 1 chamada a saveKeyring, got %d", saveCalls)
	}
	if got := store.wraps[KeyWrapKindMaster].DekID; got != DEKIdentity(dek) {
		t.Fatalf("dek_id não populado: got=%q want=%q", got, DEKIdentity(dek))
	}
}

// TestOverwriteKeychainDEK_SobrescreveSemValidar prova que a rota de
// recovery deliberada de fato grava sem validar a DEK existente.
// É a contraparte explícita de PersistDEKConsistent — só deve ser
// usada com confirmação do usuário.
func TestOverwriteKeychainDEK_SobrescreveSemValidar(t *testing.T) {
	store := newMemoryConsistencyStore()
	dekX := []byte("XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX")
	dekY := []byte("YYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYY")

	saved := []byte(nil)
	saveKeyring := func(d []byte) error { saved = append([]byte(nil), d...); return nil }

	// Mesmo havendo um wrap com dek_id divergente, OverwriteKeychainDEK
	// grava normalmente.
	store.wraps[KeyWrapKindMaster] = KeyWrap{Kind: KeyWrapKindMaster, DekID: DEKIdentity(dekX)}

	if err := OverwriteKeychainDEK(context.Background(), store, dekY, saveKeyring); err != nil {
		t.Fatalf("OverwriteKeychainDEK: %v", err)
	}
	if string(saved) != string(dekY) {
		t.Fatalf("keychain não foi sobrescrito com a nova DEK")
	}
}

// TestVerifyDEKConsistency_DetectaDivergenciaERevogaPersistencia é o
// teste de REGRESSÃO direto do incidente do usuário (AEP-0061):
// ambiente onde o keychain tem DEK_Y mas as creds existentes foram
// cifradas com DEK_X (que continua embrulhada no master_wrap).
//
// Comportamento esperado:
//   - boot detecta divergência
//   - `m.persist` vai para false (nenhuma cred nova vai sair cifrada
//     com a DEK errada e perpetuar o estado)
//   - status reporta `OK=false` com `Reason` claro
//   - credenciais cifradas com DEK_X aparecem em
//     `UnreadableCredentialIDs`
func TestVerifyDEKConsistency_DetectaDivergenciaERevogaPersistencia(t *testing.T) {
	store := newMemoryConsistencyStore()
	dekX := []byte("XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX")
	dekY := []byte("YYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYY")

	// Cred cifrada com DEK_X, persistida no store
	encoder := NewManager(dekX)
	encAuth, err := encoder.encryptAuth(&AuthConfig{Type: "bearer", Token: "sk-orphan"})
	if err != nil {
		t.Fatalf("encrypt auth: %v", err)
	}
	store.credentials = []StoredCredential{{ID: "orphan-1", UserID: "user-1", Pattern: "api.x.com", Auth: encAuth}}

	// master_wrap embrulha DEK_X (estado herdado pré-incidente)
	store.wraps[KeyWrapKindMaster] = KeyWrap{Kind: KeyWrapKindMaster, DekID: DEKIdentity(dekX)}

	// Manager boota com DEK_Y (a do keychain "atual" do incidente)
	mgr := NewManagerWithStore(dekY, store, true)
	if err := mgr.LoadInstanceSecrets(context.Background()); err != nil {
		t.Fatalf("LoadInstanceSecrets: %v", err)
	}

	status := mgr.IntegrityStatus()
	if status.OK {
		t.Fatalf("esperava OK=false (DEKs divergentes), got status=%+v", status)
	}
	if status.KeychainDekID != DEKIdentity(dekY) {
		t.Fatalf("KeychainDekID errado: %q", status.KeychainDekID)
	}
	if status.WrapsDekID != DEKIdentity(dekX) {
		t.Fatalf("WrapsDekID errado: %q", status.WrapsDekID)
	}
	if !strings.Contains(status.Reason, "diverg") {
		t.Fatalf("Reason não menciona divergência: %q", status.Reason)
	}
	if mgr.CanPersist() {
		t.Fatal("Manager continuou persistindo após detectar divergência — escritas novas vão perpetuar o problema")
	}
	found := false
	for _, id := range status.UnreadableCredentialIDs {
		if id == "orphan-1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("cred órfã 'orphan-1' não foi marcada como unreadable; ids=%v", status.UnreadableCredentialIDs)
	}
}

// TestVerifyDEKConsistency_AdotaDekIDLegado garante que wraps
// pré-AEP-0061 (com `dek_id == ""`) são tratados como "instalações
// legadas a adotar": o boot popula `dek_id` com a DEK do keychain e
// considera o estado consistente. Sem isso, todos os instalações
// existentes regredíriam para `OK=false` no upgrade.
func TestVerifyDEKConsistency_AdotaDekIDLegado(t *testing.T) {
	store := newMemoryConsistencyStore()
	dek := []byte("33333333333333333333333333333333")
	store.wraps[KeyWrapKindMaster] = KeyWrap{Kind: KeyWrapKindMaster, DekID: ""}

	mgr := NewManagerWithStore(dek, store, true)
	if err := mgr.LoadInstanceSecrets(context.Background()); err != nil {
		t.Fatalf("LoadInstanceSecrets: %v", err)
	}

	status := mgr.IntegrityStatus()
	if !status.OK {
		t.Fatalf("esperava OK=true após adoção; status=%+v", status)
	}
	if got := store.wraps[KeyWrapKindMaster].DekID; got != DEKIdentity(dek) {
		t.Fatalf("dek_id não populado: got=%q want=%q", got, DEKIdentity(dek))
	}
}

// TestPurgeUnreadableCredentials_RemoveOrfãsEDeixaResto valida que
// `PurgeUnreadableCredentials` remove apenas as credenciais marcadas
// como ilegíveis (cifradas com DEK divergente) e NÃO toca nas
// legíveis. Cobre a política `auto_purge` da decisão do usuário em
// AEP-0061.
func TestPurgeUnreadableCredentials_RemoveOrfãsEDeixaResto(t *testing.T) {
	store := newMemoryConsistencyStore()
	dekKeychain := []byte("44444444444444444444444444444444")
	dekOrfa := []byte("OROROROROROROROROROROROROROROROR")

	encoderOrfa := NewManager(dekOrfa)
	encOrfa, _ := encoderOrfa.encryptAuth(&AuthConfig{Type: "bearer", Token: "sk-orphan"})

	encoderOK := NewManager(dekKeychain)
	encOK, _ := encoderOK.encryptAuth(&AuthConfig{Type: "bearer", Token: "sk-ok"})

	store.credentials = []StoredCredential{
		{ID: "orphan-1", UserID: "user-1", Pattern: "old.example.com", Auth: encOrfa},
		{ID: "good-1", UserID: "user-1", Pattern: "new.example.com", Auth: encOK},
	}
	store.wraps[KeyWrapKindMaster] = KeyWrap{Kind: KeyWrapKindMaster, DekID: DEKIdentity(dekKeychain)}

	mgr := NewManagerWithStore(dekKeychain, store, true)
	if err := mgr.LoadInstanceSecrets(context.Background()); err != nil {
		t.Fatalf("LoadInstanceSecrets: %v", err)
	}

	status := mgr.IntegrityStatus()
	if len(status.UnreadableCredentialIDs) != 1 || status.UnreadableCredentialIDs[0] != "orphan-1" {
		t.Fatalf("esperava 1 unreadable=orphan-1, got %v", status.UnreadableCredentialIDs)
	}

	removed, err := mgr.PurgeUnreadableCredentials(context.Background())
	if err != nil {
		t.Fatalf("PurgeUnreadableCredentials: %v", err)
	}
	if removed != 1 {
		t.Fatalf("esperava remover 1, got %d", removed)
	}
	if len(store.credentials) != 1 || store.credentials[0].ID != "good-1" {
		t.Fatalf("cred legível foi removida indevidamente: %+v", store.credentials)
	}
	if got := mgr.IntegrityStatus().UnreadableCredentialIDs; len(got) != 0 {
		t.Fatalf("status ainda lista unreadable após purge: %v", got)
	}
}

// TestSetupMasterKey_RecusaSobrescreverDEKExistente prova que o
// caminho de Setup (que historicamente sobrescrevia silenciosamente a
// DEK existente quando chamado por engano em install já configurado)
// agora REJEITA a operação com `ErrDEKWouldOverwrite`. É a defesa
// direta da causa raiz do incidente em AEP-0061.
func TestSetupMasterKey_RecusaSobrescreverDEKExistente(t *testing.T) {
	store := newMemoryConsistencyStore()
	existing := []byte("PRE-EXISTING-DEK-LENGTH-32-BYTES")
	saveCalls := 0
	persister := func(dek []byte) error {
		// Simula PersistDEKConsistent contra um keychain que já tem
		// `existing`: rejeita por divergência.
		return PersistDEKConsistent(context.Background(), store, dek,
			func() ([]byte, error) { return existing, nil },
			func([]byte) error { saveCalls++; return nil })
	}

	_, err := setupMasterKey(store, "master-pwd", persister)
	if !errors.Is(err, ErrDEKWouldOverwrite) {
		t.Fatalf("esperava ErrDEKWouldOverwrite, got %v", err)
	}
	if saveCalls != 0 {
		t.Fatalf("saveKeyring foi chamado mesmo recusando — defesa ineficaz")
	}
}
