package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"assistente/internal/database"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrExternalIdentityNotMapped     = errors.New("identidade externa não mapeada")
	ErrExternalIdentityAlreadyMapped = errors.New("identidade externa já mapeada")
	ErrExternalIdentityRevoked       = errors.New("identidade externa revogada")
	ErrExternalIdentityNotReady      = errors.New("mapeamento de identidade externa não está pronto")
	ErrExternalAdministratorRequired = errors.New("administração externa autenticada obrigatória")
	ErrExternalTargetUserRequired    = errors.New("usuário local ativo obrigatório")
)

// ExternalIdentityMapping é a linha administrativa canônica (issuer, subject)
// -> users.id. A migração que cria a FK para users pertence ao host central e
// não é executada por este pacote.
type ExternalIdentityMapping struct {
	ID        string         `gorm:"type:text;primaryKey" json:"id"`
	Issuer    string         `gorm:"not null;uniqueIndex:ux_external_identity_issuer_subject,priority:1" json:"issuer"`
	Subject   string         `gorm:"not null;uniqueIndex:ux_external_identity_issuer_subject,priority:2" json:"subject"`
	UserID    string         `gorm:"not null;index" json:"userId"`
	Enabled   bool           `gorm:"not null;default:true;index" json:"enabled"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	User      *database.User `json:"-" gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (ExternalIdentityMapping) TableName() string { return "external_identity_mappings" }

// ExternalIdentityMappingParams contém apenas a escolha administrativa. Ele
// nunca é usado para autenticar quem faz a operação.
type ExternalIdentityMappingParams struct {
	Issuer  string
	Subject string
	UserID  string
}

// ExternalIdentityRepository acessa exclusivamente o armazenamento do mapa.
// Ele não cria a tabela nem concede prontidão: isso é responsabilidade da
// migração/host, evitando que uma chamada de runtime faça DDL ou JIT.
type ExternalIdentityRepository struct {
	db  *gorm.DB
	now func() time.Time
}

func NewExternalIdentityRepository(db *gorm.DB) *ExternalIdentityRepository {
	return &ExternalIdentityRepository{db: db, now: time.Now}
}

// CheckReadiness apenas verifica, de forma não mutante, que a migração central
// já publicou a tabela. A presença da tabela não habilita execução por si só:
// o host deve instalar uma flag de readiness explícita no autenticador depois
// de concluir bootstrap e adoção pelo middleware.
func (r *ExternalIdentityRepository) CheckReadiness(ctx context.Context) error {
	if r == nil || r.db == nil || ctx == nil {
		return ErrExternalIdentityNotReady
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !r.db.Migrator().HasTable(&ExternalIdentityMapping{}) {
		return ErrExternalIdentityNotReady
	}
	return nil
}

// CheckIssuerReadiness confirma que o issuer pode usar a identidade externa:
// ambas as tabelas centrais devem existir e o ledger v31 deve provar o
// bootstrap exato desse issuer. Esta verificação é somente leitura; não cria
// schema nem exige que o usuário que fez bootstrap continue ativo.
func (r *ExternalIdentityRepository) CheckIssuerReadiness(ctx context.Context, issuer string) error {
	if r == nil || r.db == nil || ctx == nil || !validExternalIdentityPart(issuer) {
		return ErrExternalIdentityNotReady
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	db := r.db.WithContext(ctx)
	if !db.Migrator().HasTable(&ExternalIdentityMapping{}) || !db.Migrator().HasTable(&database.ExternalIdentityAdminAudit{}) {
		return ErrExternalIdentityNotReady
	}
	var bootstrapCount int64
	if err := db.Model(&database.ExternalIdentityAdminAudit{}).
		Where("issuer = ? AND action = ?", issuer, "bootstrap").
		Count(&bootstrapCount).Error; err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return err
	}
	if bootstrapCount != 1 {
		return ErrExternalIdentityNotReady
	}
	return nil
}

// Create persiste um vínculo novo, sem atualizar silenciosamente um vínculo
// existente. A unicidade é exatamente (issuer, subject), independente do
// usuário local escolhido.
func (r *ExternalIdentityRepository) Create(ctx context.Context, params ExternalIdentityMappingParams) (*ExternalIdentityMapping, error) {
	if err := r.CheckReadiness(ctx); err != nil {
		return nil, err
	}
	issuer, subject, userID, err := normalizeExternalMapping(params)
	if err != nil {
		return nil, err
	}
	var user database.User
	if err := r.db.WithContext(ctx).Where("id = ? AND is_active = ?", userID, true).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrExternalTargetUserRequired
		}
		return nil, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	now := r.now()
	if now.IsZero() {
		return nil, ErrExternalIdentityNotReady
	}
	row := &ExternalIdentityMapping{ID: id.String(), Issuer: issuer, Subject: subject, UserID: userID, Enabled: true, CreatedAt: now, UpdatedAt: now}
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(row)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrExternalIdentityAlreadyMapped
	}
	return row, nil
}

// Resolve é a única leitura usada pelo autenticador de comandos. Ela relê o
// estado da tabela e do usuário a cada autenticação; nenhum mapa é cacheado.
func (r *ExternalIdentityRepository) Resolve(ctx context.Context, issuer, subject string) (*ExternalIdentityMapping, error) {
	if err := r.CheckReadiness(ctx); err != nil {
		return nil, err
	}
	if !validExternalIdentityPart(issuer) || !validExternalIdentityPart(subject) {
		return nil, ErrExternalIdentityNotMapped
	}
	var row ExternalIdentityMapping
	if err := r.db.WithContext(ctx).Where("issuer = ? AND subject = ?", issuer, subject).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrExternalIdentityNotMapped
		}
		return nil, err
	}
	if !row.Enabled {
		return nil, ErrExternalIdentityRevoked
	}
	var user database.User
	if err := r.db.WithContext(ctx).Where("id = ? AND is_active = ?", row.UserID, true).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrExternalIdentityNotMapped
		}
		return nil, err
	}
	return &row, nil
}

// ActiveUserRole relê o role atual do usuário mapeado. O role do JWT e o
// conteúdo de qualquer projeção do chamador não participam desta decisão.
func (r *ExternalIdentityRepository) ActiveUserRole(ctx context.Context, userID string) (string, error) {
	if err := r.CheckReadiness(ctx); err != nil {
		return "", err
	}
	if userID == "" || strings.TrimSpace(userID) != userID || !canonicalSessionUUID(userID) {
		return "", ErrExternalTargetUserRequired
	}
	var user database.User
	if err := r.db.WithContext(ctx).Where("id = ? AND is_active = ?", userID, true).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrExternalTargetUserRequired
		}
		return "", err
	}
	return user.Role, nil
}

func (r *ExternalIdentityRepository) Revoke(ctx context.Context, issuer, subject string) error {
	if err := r.CheckReadiness(ctx); err != nil {
		return err
	}
	if !validExternalIdentityPart(issuer) || !validExternalIdentityPart(subject) {
		return ErrExternalIdentityNotMapped
	}
	now := r.now()
	result := r.db.WithContext(ctx).Model(&ExternalIdentityMapping{}).
		Where("issuer = ? AND subject = ? AND enabled = ?", issuer, subject, true).
		Updates(map[string]any{"enabled": false, "updated_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrExternalIdentityNotMapped
	}
	return nil
}

// Enable só é exposto para uma nova decisão administrativa explícita; não é
// usado pelo caminho de autenticação e não aceita upsert de uma identidade.
func (r *ExternalIdentityRepository) Enable(ctx context.Context, issuer, subject string) error {
	if err := r.CheckReadiness(ctx); err != nil {
		return err
	}
	if !validExternalIdentityPart(issuer) || !validExternalIdentityPart(subject) {
		return ErrExternalIdentityNotMapped
	}
	result := r.db.WithContext(ctx).Model(&ExternalIdentityMapping{}).
		Where("issuer = ? AND subject = ?", issuer, subject).
		Updates(map[string]any{"enabled": true, "updated_at": r.now()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrExternalIdentityNotMapped
	}
	return nil
}

func normalizeExternalMapping(params ExternalIdentityMappingParams) (string, string, string, error) {
	issuer := params.Issuer
	subject := params.Subject
	userID := params.UserID
	if !validExternalIdentityPart(issuer) || !validExternalIdentityPart(subject) {
		return "", "", "", ErrExternalIdentityNotMapped
	}
	if userID == "" || strings.TrimSpace(userID) != userID || !canonicalSessionUUID(userID) {
		return "", "", "", ErrExternalTargetUserRequired
	}
	return issuer, subject, userID, nil
}

func validExternalIdentityPart(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && !strings.ContainsRune(value, '\x00')
}
