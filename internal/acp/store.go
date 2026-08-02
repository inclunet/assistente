package acp

import (
	"context"
	"errors"
	"strings"
	"time"

	"assistente/internal/database"

	"gorm.io/gorm"
)

// StoredSession é o vínculo entre uma conversa do app e a sessão do agente,
// como ele fica guardado entre execuções (AEP-0084 D4).
type StoredSession struct {
	ConversationID string
	ProviderID     string
	// SessionID é o identificador que o agente atribuiu. Volta para ele no
	// session/load, então é guardado como veio.
	SessionID string
	// PrefixHash resume o prefixo estável do perfil que esta sessão já ouviu.
	PrefixHash string
	// WorkDir é o diretório com que a sessão foi aberta.
	WorkDir string
}

// SessionStore guarda o vínculo conversa↔sessão. Um manager sem store funciona,
// mas esquece tudo ao fechar o app: toda conversa recomeça com um agente que
// não lembra do que foi dito.
type SessionStore interface {
	// Load devolve o vínculo registrado, ou nil quando ainda não há nenhum.
	Load(ctx context.Context, conversationID, providerID string) (*StoredSession, error)

	// Save grava o vínculo, substituindo o anterior da mesma conversa com o
	// mesmo provider.
	Save(ctx context.Context, rec StoredSession) error

	// SavePrefixHash anota o prefixo já entregue sem tocar no resto.
	SavePrefixHash(ctx context.Context, conversationID, providerID, hash string) error

	// Delete esquece todas as sessões da conversa, de todos os providers. É o
	// que acontece quando a conversa é limpa ou excluída.
	Delete(ctx context.Context, conversationID string) error
}

// storeTimeout limita a escrita do vínculo. Ela roda fora do cancelamento de
// quem pediu o turno (perder o registro deixaria sessão órfã no agente), e sem
// prazo próprio um banco travado penduraria o turno inteiro.
const storeTimeout = 5 * time.Second

// DBSessionStore implementa SessionStore no banco do app, escopado por usuário
// (AEP-0052): a sessão de um agente carrega a conversa de quem a abriu.
type DBSessionStore struct {
	db *gorm.DB
}

// NewDBSessionStore devolve o store persistente. Sem banco devolve nil de
// verdade — a interface, não um ponteiro nulo dentro dela, que passaria pelo
// teste de nulidade do manager e estouraria no primeiro uso. O manager sem
// store funciona; é o caso de um app ainda sem banco aberto.
func NewDBSessionStore(db *gorm.DB) SessionStore {
	if db == nil {
		return nil
	}
	return &DBSessionStore{db: db}
}

func (s *DBSessionStore) Load(ctx context.Context, conversationID, providerID string) (*StoredSession, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	conversationID, providerID = strings.TrimSpace(conversationID), strings.TrimSpace(providerID)
	if conversationID == "" || providerID == "" {
		return nil, errors.New("conversa ou provider vazio ao buscar sessão ACP")
	}
	var row database.ACPSession
	err := database.ScopeByUser(ctx, s.db.WithContext(ctx), "user_id").
		Where("conversation_id = ? AND provider_id = ?", conversationID, providerID).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &StoredSession{
		ConversationID: row.ConversationID,
		ProviderID:     row.ProviderID,
		SessionID:      row.SessionID,
		PrefixHash:     row.PromptPrefixHash,
		WorkDir:        row.Cwd,
	}, nil
}

func (s *DBSessionStore) Save(ctx context.Context, rec StoredSession) error {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	rec.ConversationID = strings.TrimSpace(rec.ConversationID)
	rec.ProviderID = strings.TrimSpace(rec.ProviderID)
	if rec.ConversationID == "" || rec.ProviderID == "" {
		return errors.New("conversa ou provider vazio ao gravar sessão ACP")
	}
	if strings.TrimSpace(rec.SessionID) == "" {
		return errors.New("sessão ACP sem identificador")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing database.ACPSession
		err := tx.Where("user_id = ? AND conversation_id = ? AND provider_id = ?", userID, rec.ConversationID, rec.ProviderID).
			First(&existing).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return tx.Create(&database.ACPSession{
				UserID:           userID,
				ConversationID:   rec.ConversationID,
				ProviderID:       rec.ProviderID,
				SessionID:        rec.SessionID,
				PromptPrefixHash: rec.PrefixHash,
				Cwd:              rec.WorkDir,
			}).Error
		case err != nil:
			return err
		default:
			return tx.Model(&existing).Updates(map[string]any{
				"session_id":         rec.SessionID,
				"prompt_prefix_hash": rec.PrefixHash,
				"cwd":                rec.WorkDir,
			}).Error
		}
	})
}

func (s *DBSessionStore) SavePrefixHash(ctx context.Context, conversationID, providerID, hash string) error {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	conversationID, providerID = strings.TrimSpace(conversationID), strings.TrimSpace(providerID)
	if conversationID == "" || providerID == "" {
		return errors.New("conversa ou provider vazio ao anotar prefixo da sessão ACP")
	}
	res := s.db.WithContext(ctx).Model(&database.ACPSession{}).
		Where("user_id = ? AND conversation_id = ? AND provider_id = ?", userID, conversationID, providerID).
		Update("prompt_prefix_hash", hash)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("sessão ACP não encontrada para anotar o prefixo")
	}
	return nil
}

func (s *DBSessionStore) Delete(ctx context.Context, conversationID string) error {
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return nil
	}
	return database.ScopeByUser(ctx, s.db.WithContext(ctx), "user_id").
		Where("conversation_id = ?", conversationID).
		Delete(&database.ACPSession{}).Error
}
