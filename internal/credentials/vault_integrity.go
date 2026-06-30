package credentials

import (
	"assistente/internal/logging"
	"context"
	"errors"
	"sync"
)

// VaultIntegrityStatus descreve a relação entre a DEK que o Manager
// está usando em runtime (`m.encKey`) e a DEK embrulhada no
// `master_wrap` do banco. É consultado pela UI para informar o
// usuário quando há credenciais "ilegíveis" e oferecer recuperação
// (ver AEP-0061).
type VaultIntegrityStatus struct {
	// HasKeyringDEK indica se foi possível ler uma DEK do keychain
	// no boot. Quando false, normalmente HasMasterWrap também é
	// false (cofre pré-Setup); se for true, sinaliza um keychain
	// danificado/limpo manualmente — `OK` reflete isso.
	HasKeyringDEK bool `json:"hasKeyringDEK"`

	// HasMasterWrap indica se há master_wrap persistido no banco.
	HasMasterWrap bool `json:"hasMasterWrap"`

	// KeychainDekID é `DEKIdentity(DEK_keychain)` ou string vazia.
	KeychainDekID string `json:"keychainDekId"`

	// WrapsDekID é o `dek_id` registrado no master_wrap (ou vazio se
	// pré-AEP-0061 e ainda não migrado).
	WrapsDekID string `json:"wrapsDekId"`

	// OK é true quando keychain e wraps representam a mesma DEK ou
	// quando o estado é "vazio" coerente (sem DEK e sem wraps).
	OK bool `json:"ok"`

	// Reason descreve por que `OK == false` em formato amigável para
	// UI/log. Vazio quando OK.
	Reason string `json:"reason,omitempty"`

	// UnreadableCredentialIDs lista os IDs de credential_entries que
	// NÃO decifraram com a DEK do keychain. A UI usa para mostrar a
	// lista exata e oferecer "remover" / "recuperar".
	UnreadableCredentialIDs []string `json:"unreadableCredentialIds,omitempty"`
}

// vaultIntegrity guarda o último status calculado para que API / UI
// possam consultar sem refazer o trabalho. É preenchido em
// `verifyDEKConsistency` e atualizado em cada Load/Reset.
type vaultIntegrity struct {
	mu     sync.RWMutex
	status VaultIntegrityStatus
}

func (v *vaultIntegrity) get() VaultIntegrityStatus {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.status
}

func (v *vaultIntegrity) set(status VaultIntegrityStatus) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.status = status
}

// IntegrityStatus expõe o último status calculado da consistência
// DEK_keychain ↔ DEK_wraps para callers (App API / UI).
func (m *Manager) IntegrityStatus() VaultIntegrityStatus {
	if m == nil {
		return VaultIntegrityStatus{}
	}
	return m.integrity.get()
}

// verifyDEKConsistency é chamado pelo Manager no boot (antes de
// qualquer escrita persistida) e em qualquer Reset/configure que troque
// `m.encKey`. Atualiza `m.integrity` e, se houver divergência, REVOGA
// `m.persist` (bloqueia novas gravações que cifrariam com a DEK errada
// e perpetuariam o estado divergente).
//
// O estado sempre fica consultável via `IntegrityStatus()` mesmo se
// retornar erro — assim a UI consegue mostrar diagnóstico mesmo
// quando o boot continua (a sessão ainda funciona com creds que
// decifram).
func (m *Manager) verifyDEKConsistency(ctx context.Context) error {
	if m == nil || m.store == nil {
		return nil
	}

	status := VaultIntegrityStatus{
		KeychainDekID: DEKIdentity(m.encKey),
		HasKeyringDEK: len(m.encKey) > 0,
	}

	wrap, err := m.store.GetKeyWrap(ctx, KeyWrapKindMaster)
	if err != nil {
		status.OK = false
		status.Reason = "erro ao ler master_wrap: " + err.Error()
		m.integrity.set(status)
		return err
	}
	status.HasMasterWrap = wrap != nil
	if wrap != nil {
		status.WrapsDekID = wrap.DekID
	}

	switch {
	case wrap == nil && !status.HasKeyringDEK:
		// Pré-Setup: sem DEK e sem wrap; estado "vazio coerente".
		status.OK = true
	case wrap == nil && status.HasKeyringDEK:
		// Tem DEK em runtime mas nenhum wrap persistido. Caminho
		// legítimo só em testes (NewManager com chave aleatória).
		// Em produção, app_credentials só seta encKey != nil quando
		// LoadDEKFromKeychain teve sucesso, então nesse cenário o
		// wizard de Setup ainda vai rodar e criar os wraps.
		status.OK = true
	case wrap != nil && !status.HasKeyringDEK:
		status.OK = false
		status.Reason = "wrap existe mas a DEK não está no keychain — desbloqueie com a senha mestre ou recovery key"
	case wrap.DekID == "":
		// Wrap pré-AEP-0061 ainda sem dek_id. Adotamos a DEK do
		// keychain como autoritativa neste boot e populamos. Se a
		// instalação tiver creds órfãs (cifradas com DEK que sumiu),
		// elas vão aparecer em `UnreadableCredentialIDs` no scan
		// abaixo.
		logging.Errorf(ctx, "credentials.vault-integrity", "[Credentials] master_wrap sem dek_id — adotando DEK do keychain (%s) como referência", status.KeychainDekID)
		if err := adoptDekIDInWrapsIfMissing(ctx, m.store, status.KeychainDekID); err != nil {
			status.OK = false
			status.Reason = "falha ao popular dek_id em wraps legados: " + err.Error()
			m.integrity.set(status)
			return err
		}
		status.WrapsDekID = status.KeychainDekID
		status.OK = true
	case wrap.DekID == status.KeychainDekID:
		status.OK = true
	default:
		// DIVERGÊNCIA: keychain tem uma DEK, wraps embrulham outra.
		// Bloqueia novas gravações para não perpetuar dois universos.
		logging.Infof(ctx, "credentials.vault-integrity", "[Credentials] CRITICAL: divergência DEK detectada — keychain=%s wraps=%s; bloqueando persistência até resolver", status.KeychainDekID, wrap.DekID)
		m.persist = false
		status.OK = false
		status.Reason = "divergência: keychain tem uma DEK e os wraps embrulham outra; use a senha mestre/recovery para reconciliar (UnlockOverwriteKeychain) ou reemita as credenciais ilegíveis"
	}

	// Scan de credenciais ilegíveis com a DEK atual. Roda em qualquer
	// estado (mesmo OK) para refletir creds órfãs herdadas de antes.
	if status.HasKeyringDEK {
		ids, scanErr := m.scanUnreadableCredentialIDs(ctx)
		if scanErr != nil {
			logging.Infof(ctx, "credentials.vault-integrity", "[Credentials] scan de creds ilegíveis falhou: %v", scanErr)
		}
		status.UnreadableCredentialIDs = ids
	}

	m.integrity.set(status)
	return nil
}

// scanUnreadableCredentialIDs varre as credenciais persistidas (em
// todos os escopos) e identifica as que não decifram com `m.encKey`.
// Retorna apenas IDs (não bytes) para que o caller possa removê-las
// ou marcá-las como `unreadable` na UI sem expor ciphertext.
func (m *Manager) scanUnreadableCredentialIDs(ctx context.Context) ([]string, error) {
	if m.store == nil {
		return nil, nil
	}
	allLister, ok := m.store.(allCredentialsLister)
	if !ok {
		return nil, nil
	}
	all, err := allLister.ListAllCredentialsIgnoringScope(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0)
	for _, entry := range all {
		if entry.Auth == nil {
			continue
		}
		if !m.isAuthDecryptable(entry.Auth) {
			ids = append(ids, entry.ID)
		}
	}
	return ids, nil
}

// allCredentialsLister é uma interface opcional implementada por
// stores que conseguem listar TODAS as credenciais sem aplicar o
// filtro user-scope. É usado APENAS para diagnóstico de integridade
// (scanUnreadableCredentialIDs) — nunca para servir credenciais a
// requests do app.
type allCredentialsLister interface {
	ListAllCredentialsIgnoringScope(ctx context.Context) ([]StoredCredential, error)
}

// isAuthDecryptable testa se conseguimos decifrar pelo menos um campo
// sensível com `m.encKey`. Suficiente para identificar wraps órfãos:
// se o token cifrado com DEK_X não bate o GCM tag de DEK_atual, o
// `gcm.Open` retorna `cipher: message authentication failed` e
// reportamos como ilegível.
func (m *Manager) isAuthDecryptable(auth *AuthConfig) bool {
	probes := []string{auth.Token, auth.Password, auth.RefreshURL, auth.ClientSecret, auth.ClientID}
	for _, v := range auth.Headers {
		probes = append(probes, v)
	}
	hasAny := false
	for _, p := range probes {
		if p == "" {
			continue
		}
		hasAny = true
		if _, err := m.decrypt(p); err == nil {
			return true
		}
	}
	if !hasAny {
		// Sem campos sensíveis (ex.: client_id em cleartext), nada
		// para decifrar — não conta como ilegível.
		return true
	}
	return false
}

// PurgeUnreadableCredentials remove do store todas as credenciais
// listadas em `IntegrityStatus().UnreadableCredentialIDs`. Idempotente:
// se nenhuma cred estiver marcada, retorna 0 e nil.
//
// É a contraparte da UI "remover credenciais ilegíveis após
// confirmação"; também pode ser chamada em rotina de manutenção
// administrativa quando o usuário decide reemitir tudo.
func (m *Manager) PurgeUnreadableCredentials(ctx context.Context) (int, error) {
	if m == nil || m.store == nil {
		return 0, nil
	}
	purger, ok := m.store.(unreadableCredentialPurger)
	if !ok {
		return 0, errors.New("store atual não suporta remoção por ID; reemita as credenciais manualmente")
	}
	status := m.IntegrityStatus()
	if len(status.UnreadableCredentialIDs) == 0 {
		return 0, nil
	}
	removed, err := purger.DeleteCredentialsByID(ctx, status.UnreadableCredentialIDs)
	if err != nil {
		return removed, err
	}
	// Limpa do cache em memória qualquer entry com ID purgado.
	m.mu.Lock()
	if removed > 0 {
		toKeep := m.credentials[:0]
		removedSet := make(map[string]struct{}, len(status.UnreadableCredentialIDs))
		for _, id := range status.UnreadableCredentialIDs {
			removedSet[id] = struct{}{}
		}
		for _, c := range m.credentials {
			if _, drop := removedSet[c.ID]; drop {
				continue
			}
			toKeep = append(toKeep, c)
		}
		m.credentials = toKeep
	}
	m.mu.Unlock()

	// Atualiza status zerando IDs purgados.
	status.UnreadableCredentialIDs = nil
	m.integrity.set(status)
	return removed, nil
}

type unreadableCredentialPurger interface {
	DeleteCredentialsByID(ctx context.Context, ids []string) (int, error)
}
