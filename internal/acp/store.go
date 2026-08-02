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

	// DeleteAll esquece as sessões de todas as conversas de quem pediu. É o
	// "limpar tudo": as conversas que essas sessões descrevem sumiram, e um
	// vínculo sem conversa nunca mais seria reencontrado nem apagado.
	DeleteAll(ctx context.Context) error
}

// storeTimeout limita a escrita do vínculo. Ela roda fora do cancelamento de
// quem pediu o turno (perder o registro deixaria sessão órfã no agente), e sem
// prazo próprio um banco travado penduraria o turno inteiro.
const storeTimeout = 5 * time.Second

// DBSessionStore implementa SessionStore no banco do app, escopado por usuário
// (AEP-0052): a sessão de um agente carrega a conversa de quem a abriu.
type DBSessionStore struct {
	// resolve busca o banco a cada uso em vez de guardar a conexão. Resetar o
	// banco fecha a conexão e abre outra; um ponteiro guardado no início
	// continuaria apontando para a fechada, e toda gravação de sessão falharia
	// até reiniciar o app.
	resolve func() *gorm.DB
}

// NewDBSessionStore devolve o store persistente. Sem jeito de achar o banco
// devolve nil de verdade — a interface, não um ponteiro nulo dentro dela, que
// passaria pelo teste de nulidade do manager e estouraria no primeiro uso. O
// manager sem store funciona; é o caso de um app ainda sem banco aberto.
func NewDBSessionStore(resolve func() *gorm.DB) SessionStore {
	if resolve == nil {
		return nil
	}
	return &DBSessionStore{resolve: resolve}
}

// db devolve a conexão do momento. Banco fechado é falha de verdade: fingir que
// deu certo deixaria a sessão viva no agente sem registro que a reencontre.
func (s *DBSessionStore) db(ctx context.Context) (*gorm.DB, error) {
	db := s.resolve()
	if db == nil {
		return nil, errors.New("banco indisponível para as sessões ACP")
	}
	return db.WithContext(ctx), nil
}

func (s *DBSessionStore) Load(ctx context.Context, conversationID, providerID string) (*StoredSession, error) {
	if _, err := database.RequireUserID(ctx); err != nil {
		return nil, err
	}
	conversationID, providerID = strings.TrimSpace(conversationID), strings.TrimSpace(providerID)
	if conversationID == "" || providerID == "" {
		return nil, errors.New("conversa ou provider vazio ao buscar sessão ACP")
	}
	db, err := s.db(ctx)
	if err != nil {
		return nil, err
	}
	var row database.ACPSession
	err = database.ScopeByUser(ctx, db, "user_id").
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
	db, err := s.db(ctx)
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
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
	db, err := s.db(ctx)
	if err != nil {
		return err
	}
	res := db.Model(&database.ACPSession{}).
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
	db, err := s.db(ctx)
	if err != nil {
		return err
	}
	return database.ScopeByUser(ctx, db, "user_id").
		Where("conversation_id = ?", conversationID).
		Delete(&database.ACPSession{}).Error
}

func (s *DBSessionStore) DeleteAll(ctx context.Context) error {
	if _, err := database.RequireUserID(ctx); err != nil {
		return err
	}
	db, err := s.db(ctx)
	if err != nil {
		return err
	}
	return database.ScopeByUser(ctx, db, "user_id").
		Delete(&database.ACPSession{}).Error
}
