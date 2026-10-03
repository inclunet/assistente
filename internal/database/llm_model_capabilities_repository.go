package database

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"assistente/internal/llmcapabilities"
	"gorm.io/gorm"
)

var nonPublicSourceReferencePrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"),
	netip.MustParsePrefix("5f00::/16"),
	netip.MustParsePrefix("::/96"),
}

var (
	ErrLLMModelNotFound                     = errors.New("modelo LLM não encontrado")
	ErrStaleCompatibility                   = errors.New("fato de capability pertence a uma revisão antiga do provedor")
	ErrInvalidCatalogBinding                = errors.New("vínculo de catálogo externo inválido")
	ErrSystemProviderWriteRequiresBootstrap = errors.New("escrita em provedor de sistema exige contexto interno de bootstrap")
)

// LLMModelCapabilitiesRepository persiste catálogo por provedor e resolve
// fatos somente com dados locais.
type LLMModelCapabilitiesRepository struct {
	db *gorm.DB
}

func NewLLMModelCapabilitiesRepository(db *gorm.DB) *LLMModelCapabilitiesRepository {
	return &LLMModelCapabilitiesRepository{db: db}
}

// SaveModel cria ou atualiza um modelo do provedor autenticado. Escritas em
// provedores de sistema exigem contexto interno de bootstrap. O ID remoto não
// cruza configurações de provedor.
func (r *LLMModelCapabilitiesRepository) SaveModel(ctx context.Context, providerID, remoteID, displayName string) (*LLMModel, error) {
	if err := RequireUserIDOrBootstrap(ctx); err != nil {
		return nil, err
	}
	if strings.TrimSpace(providerID) == "" || strings.TrimSpace(remoteID) == "" || remoteID != strings.TrimSpace(remoteID) || strings.ContainsRune(remoteID, '\x00') || len(remoteID) > 512 || strings.ContainsRune(displayName, '\x00') || len(displayName) > 512 {
		return nil, errors.New("identidade de modelo inválida")
	}
	var model LLMModel
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := r.providerRevisionForWrite(ctx, tx, providerID); err != nil {
			return err
		}
		lookup := tx.Where("provider_id = ? AND remote_id = ?", providerID, remoteID).First(&model)
		if lookup.Error == nil {
			model.DisplayName = displayName
			return tx.Model(&model).Update("display_name", displayName).Error
		}
		if !errors.Is(lookup.Error, gorm.ErrRecordNotFound) {
			return lookup.Error
		}
		model = LLMModel{ProviderID: providerID, RemoteID: remoteID, DisplayName: displayName}
		return tx.Create(&model).Error
	})
	if err != nil {
		return nil, err
	}
	return &model, nil
}

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

// BindCatalogModel registra identidade externa explicitamente verificada
// contra a revisão atual da conexão local.
func (r *LLMModelCapabilitiesRepository) BindCatalogModel(ctx context.Context, binding *LLMModelCatalogBinding) error {
	if binding == nil {
		return ErrInvalidCatalogBinding
	}
	if err := RequireUserIDOrBootstrap(ctx); err != nil {
		return err
	}
	binding.VerifiedAt = binding.VerifiedAt.UTC()
	if binding.ValidUntil != nil {
		validUntil := binding.ValidUntil.UTC()
		binding.ValidUntil = &validUntil
	}
	if strings.TrimSpace(binding.ExternalProviderID) == "" || strings.TrimSpace(binding.ExternalModelID) == "" || binding.ExternalProviderID != strings.TrimSpace(binding.ExternalProviderID) || binding.ExternalModelID != strings.TrimSpace(binding.ExternalModelID) || strings.ContainsRune(binding.ExternalProviderID, '\x00') || strings.ContainsRune(binding.ExternalModelID, '\x00') || !validSourceReference(binding.SourceReference) || binding.VerifiedAt.IsZero() || binding.VerifiedAt.After(time.Now()) || binding.ProviderCompatibilityRevision < 1 {
		return ErrInvalidCatalogBinding
	}
	if binding.Source != string(llmcapabilities.SourceAppCuration) && binding.Source != string(llmcapabilities.SourceOfficialCatalog) && binding.Source != string(llmcapabilities.SourceThirdPartyCatalog) {
		return ErrInvalidCatalogBinding
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		revision, model, err := r.modelAndProviderRevision(ctx, tx, binding.ModelID)
		if err != nil {
			return err
		}
		if _, err := r.providerRevisionForWrite(ctx, tx, model.ProviderID); err != nil {
			return err
		}
		if binding.ProviderCompatibilityRevision != revision {
			return ErrStaleCompatibility
		}
		if binding.ValidUntil != nil && !binding.ValidUntil.After(binding.VerifiedAt) {
			return ErrInvalidCatalogBinding
		}
		var existing LLMModelCatalogBinding
		find := tx.Where("model_id = ? AND provider_compatibility_revision = ? AND source = ? AND external_provider_id = ? AND external_model_id = ? AND verified_at = ?", model.ID, revision, binding.Source, binding.ExternalProviderID, binding.ExternalModelID, binding.VerifiedAt).First(&existing)
		if find.Error == nil {
			if !sameNullableTime(existing.ValidUntil, binding.ValidUntil) || existing.SourceReference != binding.SourceReference {
				return ErrInvalidCatalogBinding
			}
			*binding = existing
			return nil
		}
		if !errors.Is(find.Error, gorm.ErrRecordNotFound) {
			return find.Error
		}
		return tx.Create(binding).Error
	})
}

func (r *LLMModelCapabilitiesRepository) RecordCapability(ctx context.Context, assertion *LLMModelCapability) error {
	if assertion == nil {
		return llmcapabilities.ErrInvalidAssertion
	}
	if err := RequireUserIDOrBootstrap(ctx); err != nil {
		return err
	}
	normalizeAssertionTimes(&assertion.ObservedAt, &assertion.ValidUntil)
	if !validSourceReference(assertion.SourceReference) || assertion.ObservedAt.After(time.Now()) {
		return llmcapabilities.ErrInvalidAssertion
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		revision, _, err := r.modelAndProviderRevision(ctx, tx, assertion.ModelID)
		if err != nil {
			return err
		}
		var model LLMModel
		if err := tx.First(&model, "id = ?", assertion.ModelID).Error; err != nil {
			return err
		}
		if _, err := r.providerRevisionForWrite(ctx, tx, model.ProviderID); err != nil {
			return err
		}
		if assertion.ProviderCompatibilityRevision < 1 || assertion.ProviderCompatibilityRevision != revision {
			return ErrStaleCompatibility
		}
		verified, bindingRevision, err := r.bindingVerifiedForAssertion(tx, assertion.ModelID, assertion.ProviderCompatibilityRevision, llmcapabilities.Scope(assertion.Scope), llmcapabilities.Source(assertion.Source), assertion.BindingID, assertion.ObservedAt)
		if err != nil {
			return err
		}
		if err := llmcapabilities.ValidateCapabilityAssertion(capabilityAssertionFromModel(assertion, verified, bindingRevision)); err != nil {
			return err
		}
		return tx.Create(assertion).Error
	})
}

func (r *LLMModelCapabilitiesRepository) RecordField(ctx context.Context, assertion *LLMModelCapabilityField, options []LLMModelCapabilityFieldOption) error {
	if assertion == nil {
		return llmcapabilities.ErrInvalidAssertion
	}
	if err := RequireUserIDOrBootstrap(ctx); err != nil {
		return err
	}
	normalizeAssertionTimes(&assertion.ObservedAt, &assertion.ValidUntil)
	if !validSourceReference(assertion.SourceReference) || assertion.ObservedAt.After(time.Now()) {
		return llmcapabilities.ErrInvalidAssertion
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		revision, _, err := r.modelAndProviderRevision(ctx, tx, assertion.ModelID)
		if err != nil {
			return err
		}
		var model LLMModel
		if err := tx.First(&model, "id = ?", assertion.ModelID).Error; err != nil {
			return err
		}
		if _, err := r.providerRevisionForWrite(ctx, tx, model.ProviderID); err != nil {
			return err
		}
		if assertion.ProviderCompatibilityRevision < 1 || assertion.ProviderCompatibilityRevision != revision {
			return ErrStaleCompatibility
		}
		verified, bindingRevision, err := r.bindingVerifiedForAssertion(tx, assertion.ModelID, assertion.ProviderCompatibilityRevision, llmcapabilities.Scope(assertion.Scope), llmcapabilities.Source(assertion.Source), assertion.BindingID, assertion.ObservedAt)
		if err != nil {
			return err
		}
		domainOptions := make([]llmcapabilities.FieldOption, 0, len(options))
		for _, option := range options {
			domainOptions = append(domainOptions, llmcapabilities.FieldOption{Value: option.Value, Label: option.Label, State: llmcapabilities.SupportState(option.SupportState)})
		}
		domain := fieldAssertionFromModel(assertion, domainOptions, verified, bindingRevision)
		if err := llmcapabilities.ValidateFieldAssertion(domain); err != nil {
			return err
		}
		if err := tx.Create(assertion).Error; err != nil {
			return err
		}
		for i := range options {
			options[i].AssertionID = assertion.ID
		}
		if len(options) > 0 {
			return tx.Create(&options).Error
		}
		return nil
	})
}

func normalizeAssertionTimes(observedAt *time.Time, validUntil **time.Time) {
	*observedAt = observedAt.UTC()
	if *validUntil != nil {
		validUntilUTC := (*validUntil).UTC()
		*validUntil = &validUntilUTC
	}
}

// Resolve carrega apenas facts do modelo solicitado e delega a decisão ao
// resolver puro. Nenhuma chamada externa ocorre nesse caminho.
func (r *LLMModelCapabilitiesRepository) Resolve(ctx context.Context, modelID string, now time.Time) (llmcapabilities.Resolution, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return llmcapabilities.Resolution{}, err
	}
	var resolution llmcapabilities.Resolution
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		revision, model, err := r.modelAndProviderRevision(ctx, tx, modelID)
		if err != nil {
			return err
		}
		var capabilityRows []LLMModelCapability
		if err := tx.Where("model_id = ? AND provider_compatibility_revision = ?", model.ID, revision).Find(&capabilityRows).Error; err != nil {
			return err
		}
		var fieldRows []LLMModelCapabilityField
		if err := tx.Where("model_id = ? AND provider_compatibility_revision = ?", model.ID, revision).Find(&fieldRows).Error; err != nil {
			return err
		}
		bindingIDs := make([]string, 0)
		seenBindings := make(map[string]struct{})
		for _, row := range capabilityRows {
			if row.BindingID != nil {
				if _, exists := seenBindings[*row.BindingID]; !exists {
					seenBindings[*row.BindingID] = struct{}{}
					bindingIDs = append(bindingIDs, *row.BindingID)
				}
			}
		}
		for _, row := range fieldRows {
			if row.BindingID != nil {
				if _, exists := seenBindings[*row.BindingID]; !exists {
					seenBindings[*row.BindingID] = struct{}{}
					bindingIDs = append(bindingIDs, *row.BindingID)
				}
			}
		}
		bindings := make(map[string]LLMModelCatalogBinding, len(bindingIDs))
		if len(bindingIDs) > 0 {
			var bindingRows []LLMModelCatalogBinding
			if err := tx.Where("id IN ?", bindingIDs).Find(&bindingRows).Error; err != nil {
				return err
			}
			for _, binding := range bindingRows {
				bindings[binding.ID] = binding
			}
		}
		optionsByAssertion := make(map[string][]llmcapabilities.FieldOption)
		fieldIDs := make([]string, 0, len(fieldRows))
		for _, row := range fieldRows {
			fieldIDs = append(fieldIDs, row.ID)
		}
		if len(fieldIDs) > 0 {
			var optionRows []LLMModelCapabilityFieldOption
			if err := tx.Where("assertion_id IN ?", fieldIDs).Find(&optionRows).Error; err != nil {
				return err
			}
			for _, option := range optionRows {
				optionsByAssertion[option.AssertionID] = append(optionsByAssertion[option.AssertionID], llmcapabilities.FieldOption{Value: option.Value, Label: option.Label, State: llmcapabilities.SupportState(option.SupportState)})
			}
		}
		domainCapabilities := make([]llmcapabilities.CapabilityAssertion, 0, len(capabilityRows))
		for _, row := range capabilityRows {
			verified, bindingRevision := eligibleBinding(row.BindingID, bindings, model.ID, revision, now, row.ObservedAt)
			domainCapabilities = append(domainCapabilities, capabilityAssertionFromModel(&row, verified, bindingRevision))
		}
		domainFields := make([]llmcapabilities.FieldAssertion, 0, len(fieldRows))
		for _, row := range fieldRows {
			verified, bindingRevision := eligibleBinding(row.BindingID, bindings, model.ID, revision, now, row.ObservedAt)
			domainFields = append(domainFields, fieldAssertionFromModel(&row, optionsByAssertion[row.ID], verified, bindingRevision))
		}
		resolution = llmcapabilities.Resolve(now, revision, domainCapabilities, domainFields)
		return nil
	})
	return resolution, err
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
		return 0, nil, errors.New("revisão de compatibilidade inválida no provedor")
	}
	model := LLMModel{
		UUIDModel:  UUIDModel{ID: row.ModelID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt},
		ProviderID: row.ProviderID, RemoteID: row.RemoteID, DisplayName: row.DisplayName,
	}
	return row.CompatibilityRevision, &model, nil
}

func sameNullableTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func (r *LLMModelCapabilitiesRepository) providerRevisionForUser(ctx context.Context, db *gorm.DB, providerID string) (int, error) {
	userID, err := RequireUserID(ctx)
	if err != nil {
		return 0, err
	}
	var revision int
	err = db.Model(&LLMProvider{}).Select("compatibility_revision").Where("id = ? AND (user_id = ? OR user_id = '')", providerID, userID).Take(&revision).Error
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

// Referências são URLs públicas sem credenciais, query string ou fragmento.
// Dados de proveniência não podem virar um canal acidental para segredos.
func validSourceReference(reference string) bool {
	if reference == "" {
		return true
	}
	if len(reference) > 2048 || strings.TrimSpace(reference) != reference || strings.ContainsAny(reference, "\x00\r\n\t@") {
		return false
	}
	parsed, err := url.Parse(reference)
	if err != nil || !parsed.IsAbs() || parsed.Opaque != "" ||
		(!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https")) ||
		parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return false
	}
	return isPublicReferenceHost(parsed.Hostname())
}

func isPublicReferenceHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "" {
		return false
	}
	if address, err := netip.ParseAddr(host); err == nil {
		address = address.Unmap()
		if !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() ||
			address.IsLinkLocalMulticast() || address.IsUnspecified() || address.IsMulticast() || address.Zone() != "" {
			return false
		}
		for _, prefix := range nonPublicSourceReferencePrefixes {
			if prefix.Contains(address) {
				return false
			}
		}
		return true
	}
	if looksLikeNumericIP(host) {
		// Reject non-canonical numeric IPv4 spellings such as 127.1 and
		// 0x7f.0.0.1. Some URL clients interpret these as IPs when netip does not.
		return false
	}
	if !strings.Contains(host, ".") {
		return false
	}
	for _, suffix := range []string{".localhost", ".localdomain", ".local", ".internal", ".lan", ".home", ".home.arpa", ".test", ".invalid", ".example", ".onion", ".private", ".corp"} {
		if host == strings.TrimPrefix(suffix, ".") || strings.HasSuffix(host, suffix) {
			return false
		}
	}
	return true
}

func looksLikeNumericIP(host string) bool {
	for _, component := range strings.Split(host, ".") {
		if component == "" {
			return false
		}
		if strings.Trim(component, "0123456789") == "" {
			continue
		}
		hexadecimal := strings.TrimPrefix(strings.ToLower(component), "0x")
		if hexadecimal != component && hexadecimal != "" && strings.Trim(hexadecimal, "0123456789abcdef") == "" {
			continue
		}
		return false
	}
	return true
}

func (r *LLMModelCapabilitiesRepository) bindingVerifiedForAssertion(tx *gorm.DB, modelID string, revision int, scope llmcapabilities.Scope, source llmcapabilities.Source, bindingID *string, observedAt time.Time) (bool, int, error) {
	if scope != llmcapabilities.ScopeExternalBinding {
		if bindingID != nil {
			return false, 0, ErrInvalidCatalogBinding
		}
		return false, 0, nil
	}
	if bindingID == nil || strings.TrimSpace(*bindingID) == "" {
		return false, 0, ErrInvalidCatalogBinding
	}
	var binding LLMModelCatalogBinding
	if err := tx.Where("id = ? AND model_id = ? AND provider_compatibility_revision = ?", *bindingID, modelID, revision).Take(&binding).Error; err != nil {
		return false, 0, fmt.Errorf("%w: %v", ErrInvalidCatalogBinding, err)
	}
	if binding.Source != string(source) || binding.VerifiedAt.After(observedAt) || binding.ValidUntil != nil && !binding.ValidUntil.After(observedAt) {
		return false, 0, ErrInvalidCatalogBinding
	}
	return true, binding.ProviderCompatibilityRevision, nil
}

func eligibleBinding(bindingID *string, bindings map[string]LLMModelCatalogBinding, modelID string, revision int, now, observedAt time.Time) (bool, int) {
	if bindingID == nil {
		return false, 0
	}
	binding, ok := bindings[*bindingID]
	if !ok || binding.ModelID != modelID || binding.ProviderCompatibilityRevision != revision || binding.VerifiedAt.After(now) || binding.VerifiedAt.After(observedAt) || binding.ValidUntil != nil && (!binding.ValidUntil.After(now) || !binding.ValidUntil.After(observedAt)) {
		return false, 0
	}
	return true, binding.ProviderCompatibilityRevision
}

func capabilityAssertionFromModel(row *LLMModelCapability, verified bool, bindingRevision int) llmcapabilities.CapabilityAssertion {
	return llmcapabilities.CapabilityAssertion{
		ID: row.ID, Capability: llmcapabilities.Capability(row.CapabilityKey), State: llmcapabilities.SupportState(row.SupportState),
		Source: llmcapabilities.Source(row.Source), Scope: llmcapabilities.Scope(row.Scope),
		ProviderRevision: row.ProviderCompatibilityRevision, BindingVerified: verified, BindingRevision: bindingRevision,
		ObservedAt: row.ObservedAt, ValidUntil: row.ValidUntil,
	}
}

func fieldAssertionFromModel(row *LLMModelCapabilityField, options []llmcapabilities.FieldOption, verified bool, bindingRevision int) llmcapabilities.FieldAssertion {
	return llmcapabilities.FieldAssertion{
		ID: row.ID, Capability: llmcapabilities.Capability(row.CapabilityKey), Field: llmcapabilities.FieldKey(row.FieldKey),
		State: llmcapabilities.SupportState(row.SupportState), Source: llmcapabilities.Source(row.Source), Scope: llmcapabilities.Scope(row.Scope),
		ProviderRevision: row.ProviderCompatibilityRevision, BindingVerified: verified, BindingRevision: bindingRevision,
		Minimum: row.Minimum, Maximum: row.Maximum, Step: row.Step, Options: options, ObservedAt: row.ObservedAt, ValidUntil: row.ValidUntil,
	}
}
