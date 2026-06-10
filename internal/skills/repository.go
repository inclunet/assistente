package skills

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"assistente/internal/database"

	"gorm.io/gorm"
)

// ErrSkillNotFound é retornado quando um skill não existe no repositório.
var ErrSkillNotFound = errors.New("skill not found")

// Repository abstrai a persistência de skills (AEP-0051).
//
// Skills são instância-wide (sem escopo por usuário): o catálogo é compartilhado,
// como os builtins do tool_catalog. Por isso os métodos não exigem user scope.
// O ctx é mantido em todos os métodos por consistência com Jobs/MCP (cancelamento,
// transações) e para uso futuro pela AEP-0072.
type Repository interface {
	List(ctx context.Context) ([]SkillInfo, error)
	Get(ctx context.Context, slug string) (*Skill, error)
	GetByID(ctx context.Context, id string) (*Skill, error)
	Create(ctx context.Context, skill *Skill) (string, error)
	Update(ctx context.Context, slug string, skill *Skill) error
	Delete(ctx context.Context, slug string) error
	Duplicate(ctx context.Context, slug string) (string, error)
	ExistsBySlug(ctx context.Context, slug string) (bool, error)

	GetAutoSkills(ctx context.Context) ([]Skill, error)
	GetAvailableSkills(ctx context.Context) ([]Skill, error)
	GetAllSkillsFull(ctx context.Context) ([]Skill, error)
	GetUserInvocableSkills(ctx context.Context) ([]SkillInfo, error)

	SeedBuiltin(ctx context.Context, skill *Skill, version string) error

	// Catálogo compacto persistido (AEP-0072 D1). RebuildCatalog aceita um
	// materializador opcional para pré-gravar o corpo das skills em disco e
	// persistir o caminho no catálogo (Nível 1 servível direto do catálogo).
	ListCatalog(ctx context.Context) ([]SkillCatalogEntry, error)
	RebuildCatalog(ctx context.Context, materialize CatalogMaterializer) error
}

// DBRepository implementa Repository usando GORM.
type DBRepository struct {
	db  *gorm.DB
	now func() time.Time
}

// NewDBRepository cria um repositório de skills baseado em banco.
func NewDBRepository(db *gorm.DB) *DBRepository {
	return &DBRepository{db: db, now: time.Now}
}

// preserveOnUpdate são colunas gerenciadas internamente (não vêm do domínio Skill)
// e que devem ser preservadas durante um Update vindo do CRUD do usuário.
var preserveOnUpdate = []string{"id", "created_at", "slug", "is_builtin", "builtin_version", "is_customized"}

func (r *DBRepository) List(ctx context.Context) ([]SkillInfo, error) {
	var rows []database.Skill
	if err := r.db.WithContext(ctx).Order("name ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	infos := make([]SkillInfo, 0, len(rows))
	for i := range rows {
		info, err := skillInfoFromModel(&rows[i])
		if err != nil {
			return nil, err
		}
		infos = append(infos, info)
	}
	return infos, nil
}

func (r *DBRepository) Get(ctx context.Context, slug string) (*Skill, error) {
	var row database.Skill
	err := r.db.WithContext(ctx).Where("slug = ?", strings.TrimSpace(slug)).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSkillNotFound
	}
	if err != nil {
		return nil, err
	}
	return skillFromModel(&row)
}

func (r *DBRepository) GetByID(ctx context.Context, id string) (*Skill, error) {
	var row database.Skill
	err := r.db.WithContext(ctx).Where("id = ?", strings.TrimSpace(id)).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSkillNotFound
	}
	if err != nil {
		return nil, err
	}
	return skillFromModel(&row)
}

func (r *DBRepository) ExistsBySlug(ctx context.Context, slug string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&database.Skill{}).
		Where("slug = ?", strings.TrimSpace(slug)).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *DBRepository) Create(ctx context.Context, skill *Skill) (string, error) {
	if skill == nil {
		return "", fmt.Errorf("skill nil")
	}
	if strings.TrimSpace(skill.Slug) == "" {
		skill.Slug = Slugify(skill.Name)
	}
	model, err := skillToModel(skill)
	if err != nil {
		return "", err
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&database.Skill{}).Where("slug = ?", model.Slug).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("skill already exists: %s", model.Slug)
		}
		return tx.Create(model).Error
	})
	if err != nil {
		return "", err
	}
	return model.Slug, nil
}

func (r *DBRepository) Update(ctx context.Context, slug string, skill *Skill) error {
	if skill == nil {
		return fmt.Errorf("skill nil")
	}
	model, err := skillToModel(skill)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing database.Skill
		err := tx.Where("slug = ?", strings.TrimSpace(slug)).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrSkillNotFound
		}
		if err != nil {
			return err
		}
		model.ID = existing.ID
		return r.replaceSkill(tx, &existing, model, preserveOnUpdate)
	})
}

func (r *DBRepository) Delete(ctx context.Context, slug string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing database.Skill
		err := tx.Where("slug = ?", strings.TrimSpace(slug)).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrSkillNotFound
		}
		if err != nil {
			return err
		}
		if err := tx.Where("skill_id = ?", existing.ID).Delete(&database.SkillTool{}).Error; err != nil {
			return err
		}
		return tx.Delete(&existing).Error
	})
}

func (r *DBRepository) Duplicate(ctx context.Context, slug string) (string, error) {
	skill, err := r.Get(ctx, slug)
	if err != nil {
		return "", err
	}

	var rows []database.Skill
	if err := r.db.WithContext(ctx).Select("slug").Find(&rows).Error; err != nil {
		return "", err
	}
	existing := make(map[string]bool, len(rows))
	for _, row := range rows {
		existing[row.Slug] = true
	}

	newName := nextCopyName(Slugify(skill.Name), existing)
	dup := *skill
	dup.Name = newName
	dup.Slug = newName
	if dup.DisplayName == "" || dup.DisplayName == skill.Name {
		dup.DisplayName = newName
	}
	return r.Create(ctx, &dup)
}

func (r *DBRepository) GetAutoSkills(ctx context.Context) ([]Skill, error) {
	return r.querySkills(ctx, func(q *gorm.DB) *gorm.DB {
		return q.Where("auto_load = ? AND disable_model_invocation = ?", true, false)
	})
}

func (r *DBRepository) GetAvailableSkills(ctx context.Context) ([]Skill, error) {
	// Espelha a semântica do Manager filesystem: disponível = NOT IsAutoLoad(),
	// isto é, auto_load=false OU disable_model_invocation=true. Assim um skill
	// auto_load=true com disable_model_invocation=true não fica órfão entre os
	// filtros (não é auto-injetado, mas continua disponível sob demanda).
	return r.querySkills(ctx, func(q *gorm.DB) *gorm.DB {
		return q.Where("auto_load = ? OR disable_model_invocation = ?", false, true)
	})
}

func (r *DBRepository) GetAllSkillsFull(ctx context.Context) ([]Skill, error) {
	return r.querySkills(ctx, nil)
}

func (r *DBRepository) GetUserInvocableSkills(ctx context.Context) ([]SkillInfo, error) {
	var rows []database.Skill
	if err := r.db.WithContext(ctx).
		Where("user_invocable IS NULL OR user_invocable = ?", true).
		Order("name ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	infos := make([]SkillInfo, 0, len(rows))
	for i := range rows {
		info, err := skillInfoFromModel(&rows[i])
		if err != nil {
			return nil, err
		}
		infos = append(infos, info)
	}
	return infos, nil
}

// SeedBuiltin insere/atualiza um skill builtin com versionamento (AEP-0051 D5):
//   - não existe → insere com is_builtin=true e builtin_version=version
//   - existe + is_customized=true → skip (preserva customização do usuário)
//   - existe + builtin_version < version → atualiza conteúdo/metadados
//   - caso contrário → no-op (já atualizado)
func (r *DBRepository) SeedBuiltin(ctx context.Context, skill *Skill, version string) error {
	if skill == nil {
		return fmt.Errorf("skill nil")
	}
	if strings.TrimSpace(skill.Slug) == "" {
		skill.Slug = Slugify(skill.Name)
	}
	model, err := skillToModel(skill)
	if err != nil {
		return err
	}
	model.IsBuiltin = true
	model.BuiltinVersion = version

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing database.Skill
		err := tx.Where("slug = ?", model.Slug).First(&existing).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return tx.Create(model).Error
		case err != nil:
			return err
		default:
			if existing.IsCustomized {
				return nil
			}
			if !builtinVersionNewer(version, existing.BuiltinVersion) {
				return nil
			}
			model.ID = existing.ID
			// Preserva customização e timestamps; sobrescreve o resto.
			return r.replaceSkill(tx, &existing, model, []string{"id", "created_at", "is_customized"})
		}
	})
}

// querySkills carrega skills completos aplicando um scope opcional.
func (r *DBRepository) querySkills(ctx context.Context, scope func(*gorm.DB) *gorm.DB) ([]Skill, error) {
	q := r.db.WithContext(ctx).Order("name ASC")
	if scope != nil {
		q = scope(q)
	}
	var rows []database.Skill
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]Skill, 0, len(rows))
	for i := range rows {
		s, err := skillFromModel(&rows[i])
		if err != nil {
			return nil, err
		}
		result = append(result, *s)
	}
	return result, nil
}

// replaceSkill atualiza a row do skill e regrava a junction skill_tools.
// `omit` lista colunas a preservar (não sobrescrever).
func (r *DBRepository) replaceSkill(tx *gorm.DB, existing, model *database.Skill, omit []string) error {
	model.CreatedAt = existing.CreatedAt
	toolRows := model.Tools
	model.Tools = nil

	if err := tx.Where("skill_id = ?", existing.ID).Delete(&database.SkillTool{}).Error; err != nil {
		return err
	}
	if err := tx.Model(existing).Select("*").Omit(omit...).Updates(model).Error; err != nil {
		return err
	}
	for i := range toolRows {
		toolRows[i].SkillID = existing.ID
		if err := tx.Create(&toolRows[i]).Error; err != nil {
			return err
		}
	}
	return nil
}

// builtinVersionNewer indica se candidate é mais novo que current (semver X.Y.Z).
func builtinVersionNewer(candidate, current string) bool {
	if strings.TrimSpace(current) == "" {
		return true
	}
	return compareSemver(candidate, current) > 0
}

func compareSemver(a, b string) int {
	pa, pb := parseSemver(a), parseSemver(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			if pa[i] > pb[i] {
				return 1
			}
			return -1
		}
	}
	return 0
}

func parseSemver(v string) [3]int {
	var out [3]int
	parts := strings.SplitN(strings.TrimSpace(v), ".", 3)
	for i := 0; i < len(parts) && i < 3; i++ {
		n, _ := strconv.Atoi(strings.TrimSpace(parts[i]))
		out[i] = n
	}
	return out
}
