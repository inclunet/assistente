package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"assistente/internal/llmcapabilities"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrLLMModelNotFound                     = errors.New("modelo LLM não encontrado")
	ErrStaleCompatibility                   = errors.New("gravação pertence a uma revisão antiga do provedor")
	ErrSystemProviderWriteRequiresBootstrap = errors.New("escrita em provedor de sistema exige contexto interno de bootstrap")
	ErrInvalidCapabilityField               = errors.New("capability ou campo canônico inválido")
	ErrInvalidRecognizerID                  = errors.New("identificador de reconhecedor inválido")
)

type LLMModelCapabilitiesRepository struct {
	db *gorm.DB
}

func NewLLMModelCapabilitiesRepository(db *gorm.DB) *LLMModelCapabilitiesRepository {
	return &LLMModelCapabilitiesRepository{db: db}
}

// UnsupportedField é uma restrição ativa para a revisão atual da conexão.
type UnsupportedField struct {
	CapabilityCode        string
	FieldCode             string
	CompatibilityRevision int
	RecognizerID          string
	UpdatedAt             time.Time
}

// SaveModel cria a identidade remota dentro do provedor. Chamadas repetidas
// atualizam apenas o nome de exibição, sem transferir estado entre modelos.
func (r *LLMModelCapabilitiesRepository) SaveModel(ctx context.Context, providerID, remoteID, displayName string) (*LLMModel, error) {
	if err := RequireUserIDOrBootstrap(ctx); err != nil {
		return nil, err
	}
	if providerID == "" || remoteID == "" || strings.TrimSpace(remoteID) != remoteID || len(remoteID) > 512 ||
		strings.ContainsRune(remoteID, 0) || strings.ContainsRune(displayName, 0) ||
		len(displayName) > 512 {
		return nil, errors.New("identidade de modelo inválida")
	}

	var model LLMModel
	err := WithSQLiteImmediateTransactionOnce(ctx, time.Now().Add(sqliteBusyRetryMaxWait), r.db, "llm_models.save", func(tx *gorm.DB) error {
		if _, err := r.providerRevisionForWrite(ctx, tx, providerID); err != nil {
			return err
		}
		err := tx.Where("provider_id = ? AND remote_id = ?", providerID, remoteID).Take(&model).Error
		switch {
		case err == nil:
			return tx.Model(&model).Update("display_name", displayName).Error
		case !errors.Is(err, gorm.ErrRecordNotFound):
			return err
		}
		model = LLMModel{
			ProviderID:  providerID,
			RemoteID:    remoteID,
			DisplayName: displayName,
		}
		return tx.Create(&model).Error
	})
	if err != nil {
		return nil, err
	}
	return &model, nil
}

// ListModels lista modelos somente quando o chamador pode ler o provedor.
func (r *LLMModelCapabilitiesRepository) ListModels(ctx context.Context, providerID string) ([]LLMModel, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var models []LLMModel
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := r.providerRevisionForUser(ctx, tx, providerID); err != nil {
			return err
		}
		return tx.Where("provider_id = ?", providerID).Order("remote_id COLLATE BINARY").Find(&models).Error
	})
	return models, err
}

// RecordUnsupportedField grava ou atualiza uma única restrição. A revisão é a
// capturada pela requisição; se a identidade do provedor mudou, a gravação é
// recusada em vez de contaminar a conexão nova.
func (r *LLMModelCapabilitiesRepository) RecordUnsupportedField(
	ctx context.Context,
	modelID string,
	capability llmcapabilities.Capability,
	field llmcapabilities.FieldKey,
	compatibilityRevision int,
	recognizerID string,
) error {
	if err := RequireUserIDOrBootstrap(ctx); err != nil {
		return err
	}
	if !llmcapabilities.HasField(capability, field) {
		return ErrInvalidCapabilityField
	}
	if compatibilityRevision < 1 {
		return ErrStaleCompatibility
	}
	if !validRecognizerID(recognizerID) {
		return ErrInvalidRecognizerID
	}

	now := time.Now().UTC()
	return WithSQLiteImmediateTransactionOnce(ctx, now.Add(sqliteBusyRetryMaxWait), r.db, "llm_models.record_unsupported_field", func(tx *gorm.DB) error {
		revision, model, err := r.modelAndProviderRevision(ctx, tx, modelID)
		if err != nil {
			return err
		}
		if _, err := r.providerRevisionForWrite(ctx, tx, model.ProviderID); err != nil {
			return err
		}
		if revision != compatibilityRevision {
			return ErrStaleCompatibility
		}

		capabilityRow := LLMModelCapability{
			ModelID:        model.ID,
			CapabilityCode: string(capability),
		}
		err = tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "model_id"}, {Name: "capability_code"}},
			DoNothing: true,
		}).Create(&capabilityRow).Error
		if err != nil {
			return err
		}
		capabilityRow.ID = ""
		if err := tx.Where("model_id = ? AND capability_code = ?", model.ID, capability).Take(&capabilityRow).Error; err != nil {
			return err
		}

		fieldRow := LLMModelCapabilityField{
			CapabilityID:          capabilityRow.ID,
			FieldCode:             string(field),
			CompatibilityRevision: compatibilityRevision,
			RecognizerID:          recognizerID,
			UpdatedAt:             now,
		}
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "capability_id"}, {Name: "field_code"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"compatibility_revision", "recognizer_id", "updated_at",
			}),
		}).Create(&fieldRow).Error
	})
}

// ListUnsupportedFields retorna só as restrições da revisão efetiva. Campos
// sem linha permanecem desconhecidos e não são interpretados como suportados.
func (r *LLMModelCapabilitiesRepository) ListUnsupportedFields(ctx context.Context, modelID string) ([]UnsupportedField, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var fields []UnsupportedField
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		revision, model, err := r.modelAndProviderRevision(ctx, tx, modelID)
		if err != nil {
			return err
		}
		return tx.Table("llm_model_capability_fields AS f").
			Select("c.capability_code, f.field_code, f.compatibility_revision, f.recognizer_id, f.updated_at").
			Joins("JOIN llm_model_capabilities AS c ON c.id = f.capability_id").
			Where("c.model_id = ? AND f.compatibility_revision = ?", model.ID, revision).
			Order("c.capability_code COLLATE BINARY, f.field_code COLLATE BINARY").
			Scan(&fields).Error
	})
	return fields, err
}

func validRecognizerID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, char := range id {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') ||
			char == '.' || char == '_' || char == '-' || char == ':' {
			continue
		}
		return false
	}
	return true
}

func (r *LLMModelCapabilitiesRepository) modelAndProviderRevision(ctx context.Context, db *gorm.DB, modelID string) (int, *LLMModel, error) {
	if err := RequireUserIDOrBootstrap(ctx); err != nil {
		return 0, nil, err
	}
	var row struct {
		ModelID               string
		ProviderID            string
		UserID                string
		CompatibilityRevision int
		CreatedAt             time.Time
		UpdatedAt             time.Time
		RemoteID              string
		DisplayName           string
	}
	query := db.Table("llm_models AS m").
		Select("m.id AS model_id, m.provider_id, p.user_id, p.compatibility_revision, m.created_at, m.updated_at, m.remote_id, m.display_name").
		Joins("JOIN llm_providers AS p ON p.id = m.provider_id").
		Where("m.id = ?", modelID)
	if userID, ok := UserIDFromContext(ctx); ok {
		query = query.Where("(p.user_id = ? OR p.user_id = '')", userID)
	} else {
		query = query.Where("p.user_id = ''")
	}
	err := query.Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil, ErrLLMModelNotFound
	}
	if err != nil {
		return 0, nil, err
	}
	if row.CompatibilityRevision < 1 {
		return 0, nil, fmt.Errorf("revisão de compatibilidade inválida no provedor %q", row.ProviderID)
	}
	model := LLMModel{
		UUIDModel:  UUIDModel{ID: row.ModelID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt},
		ProviderID: row.ProviderID, RemoteID: row.RemoteID, DisplayName: row.DisplayName,
	}
	return row.CompatibilityRevision, &model, nil
}

func (r *LLMModelCapabilitiesRepository) providerRevisionForUser(ctx context.Context, db *gorm.DB, providerID string) (int, error) {
	userID, err := RequireUserID(ctx)
	if err != nil {
		return 0, err
	}
	var revision int
	err = db.Model(&LLMProvider{}).
		Select("compatibility_revision").
		Where("id = ? AND (user_id = ? OR user_id = '')", providerID, userID).
		Take(&revision).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, gorm.ErrRecordNotFound
	}
	if err != nil {
		return 0, err
	}
	if revision < 1 {
		return 0, errors.New("revisão de compatibilidade inválida no provedor")
	}
	return revision, nil
}

func (r *LLMModelCapabilitiesRepository) providerRevisionForWrite(ctx context.Context, db *gorm.DB, providerID string) (int, error) {
	if err := RequireUserIDOrBootstrap(ctx); err != nil {
		return 0, err
	}
	var provider LLMProvider
	if err := db.Select("user_id, compatibility_revision").Where("id = ?", providerID).Take(&provider).Error; err != nil {
		return 0, err
	}
	if provider.UserID == "" {
		if !IsBootstrap(ctx) {
			return 0, ErrSystemProviderWriteRequiresBootstrap
		}
	} else if userID, ok := UserIDFromContext(ctx); !ok || userID != provider.UserID {
		return 0, gorm.ErrRecordNotFound
	}
	if provider.CompatibilityRevision < 1 {
		return 0, errors.New("revisão de compatibilidade inválida no provedor")
	}
	return provider.CompatibilityRevision, nil
}
