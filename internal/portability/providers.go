package portability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"assistente/internal/acpregistry"
	"assistente/internal/database"

	"gorm.io/gorm"
)

// acpAPIFormat repete o valor de `llm.APIFormatACP` porque importar o pacote
// `llm` aqui fecha um ciclo: o teste de `llm` usa `mcp`, e `mcp` usa este
// pacote. A cópia é travada por teste, que compara as duas constantes.
const acpAPIFormat = "acp"

// acpProviderType repete `llm.ProviderACP` pelo mesmo motivo, e é travado pelo
// mesmo teste.
const acpProviderType = "acp"

func exportProvider(provider *database.LLMProvider) (ProviderExport, error) {
	args, err := decodeStringSlice(provider.ACPArgs)
	if err != nil {
		return ProviderExport{}, fmt.Errorf("erro ao decodificar argumentos do agente do provider %s: %w", provider.ID, err)
	}
	return ProviderExport{
		ID:                provider.ID,
		Name:              provider.Name,
		Type:              provider.Type,
		APIFormat:         provider.APIFormat,
		BaseURL:           provider.BaseURL,
		Model:             provider.Model,
		DefaultModel:      provider.DefaultModel,
		IsDefault:         provider.IsDefault,
		Timeout:           provider.Timeout,
		CredentialPattern: provider.CredentialPattern,
		CreatedAt:         provider.CreatedAt,
		ACPCommand:        provider.ACPCommand,
		ACPArgs:           args,
		ACPAgentID:        provider.ACPAgentID,
	}, nil
}

func importProvider(ctx context.Context, provider ProviderExport) (bool, error) {
	normalized, err := validateProviderExport(provider)
	if err != nil {
		return false, err
	}
	if existing, err := findExistingProviderByID(ctx, normalized.ID); err != nil {
		return false, err
	} else if existing != nil {
		return overwriteProvider(ctx, normalized)
	}

	err = database.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return persistProvider(ctx, tx, normalized, nil)
	})
	if err != nil {
		return false, err
	}

	return true, nil
}

func overwriteProvider(ctx context.Context, provider ProviderExport) (bool, error) {
	normalized, err := validateProviderExport(provider)
	if err != nil {
		return false, err
	}

	existing, err := findExistingProviderByID(ctx, normalized.ID)
	if err != nil {
		return false, err
	}
	if existing == nil {
		return importProvider(ctx, normalized)
	}

	err = database.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return persistProvider(ctx, tx, normalized, existing)
	})
	if err != nil {
		return false, err
	}

	return true, nil
}

func persistProvider(ctx context.Context, tx *gorm.DB, provider ProviderExport, existing *database.LLMProvider) error {
	acpArgs, err := encodeACPList(provider.ACPArgs)
	if err != nil {
		return fmt.Errorf("erro ao serializar argumentos do agente do provider %q: %w", provider.ID, err)
	}
	acpEnv, err := encodeACPMap(provider.ACPEnv)
	if err != nil {
		return fmt.Errorf("erro ao serializar ambiente do agente do provider %q: %w", provider.ID, err)
	}
	createdAt := provider.CreatedAt
	if createdAt.IsZero() {
		if existing != nil && !existing.CreatedAt.IsZero() {
			createdAt = existing.CreatedAt
		} else {
			createdAt = time.Now().UTC()
		}
	}
	updatedAt := createdAt
	if existing != nil && provider.CreatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}

	if provider.IsDefault {
		if err := database.ScopeByUser(ctx, tx.Model(&database.LLMProvider{}), "user_id").
			Where("is_default = ? AND id <> ?", true, strings.TrimSpace(provider.ID)).
			Update("is_default", false).Error; err != nil {
			return err
		}
	}

	if existing == nil {
		model := database.LLMProvider{
			ID:                strings.TrimSpace(provider.ID),
			Name:              provider.Name,
			Type:              provider.Type,
			APIFormat:         provider.APIFormat,
			BaseURL:           provider.BaseURL,
			Model:             provider.Model,
			DefaultModel:      provider.DefaultModel,
			IsDefault:         provider.IsDefault,
			Timeout:           provider.Timeout,
			CredentialPattern: provider.CredentialPattern,
			ACPCommand:        provider.ACPCommand,
			ACPArgs:           acpArgs,
			ACPEnv:            acpEnv,
			ACPAgentID:        provider.ACPAgentID,
			CreatedAt:         createdAt,
			UpdatedAt:         updatedAt,
		}
		if userID, ok := database.UserIDFromContext(ctx); ok {
			model.UserID = userID
		}
		return tx.Create(&model).Error
	}

	existing.Name = provider.Name
	existing.Type = provider.Type
	existing.APIFormat = provider.APIFormat
	existing.BaseURL = provider.BaseURL
	existing.Model = provider.Model
	existing.DefaultModel = provider.DefaultModel
	existing.IsDefault = provider.IsDefault
	existing.Timeout = provider.Timeout
	existing.CredentialPattern = provider.CredentialPattern
	existing.ACPCommand = provider.ACPCommand
	existing.ACPArgs = acpArgs
	existing.ACPEnv = acpEnv
	existing.ACPAgentID = provider.ACPAgentID
	existing.CreatedAt = createdAt
	existing.UpdatedAt = updatedAt
	return tx.Save(existing).Error
}

func validateProviderExport(provider ProviderExport) (ProviderExport, error) {
	normalized := provider
	normalized.ID = strings.TrimSpace(provider.ID)
	normalized.Name = strings.TrimSpace(provider.Name)
	normalized.Type = strings.TrimSpace(provider.Type)
	normalized.APIFormat = strings.TrimSpace(provider.APIFormat)
	normalized.BaseURL = strings.TrimSpace(provider.BaseURL)
	normalized.Model = strings.TrimSpace(provider.Model)
	normalized.DefaultModel = strings.TrimSpace(provider.DefaultModel)
	normalized.CredentialPattern = strings.TrimSpace(provider.CredentialPattern)
	normalized.ACPCommand = strings.TrimSpace(provider.ACPCommand)
	normalized.ACPAgentID = strings.TrimSpace(provider.ACPAgentID)

	if normalized.ID == "" {
		return ProviderExport{}, fmt.Errorf("provider sem id não pode ser importado")
	}
	if normalized.Name == "" {
		return ProviderExport{}, fmt.Errorf("provider %q sem name não pode ser importado", normalized.ID)
	}
	if normalized.Type == "" {
		return ProviderExport{}, fmt.Errorf("provider %q sem type não pode ser importado", normalized.ID)
	}
	if isACPExport(normalized) {
		// O agente não tem endereço: o que o encontra é o comando, e é ele que
		// passa a ser obrigatório.
		if normalized.ACPCommand == "" {
			return ProviderExport{}, fmt.Errorf("provider %q em formato acp sem acpCommand não pode ser importado", normalized.ID)
		}
		// Arquivo escrito antes da emenda do D11 nomeia o agente no tipo. Ele
		// entra pelo vocabulário de hoje: tipo único, agente no campo próprio.
		// Importar como estava criaria, do lado de fora da migração, o provedor
		// com tipo que o app não oferece mais.
		if agentID, legado := acpregistry.LegacyProviderTypeAgentID(normalized.Type); legado {
			normalized.Type = acpProviderType
			if normalized.ACPAgentID == "" {
				normalized.ACPAgentID = agentID
			}
		}
		return normalized, nil
	}
	// Configuração de agente fora do formato acp não teria leitor: nenhum
	// caminho HTTP sobe processo. Recusar avisa quem montou o arquivo; guardar
	// em silêncio deixaria a pessoa achando que configurou alguma coisa.
	if normalized.ACPCommand != "" || len(normalized.ACPArgs) > 0 || len(normalized.ACPEnv) > 0 ||
		normalized.ACPAgentID != "" {
		return ProviderExport{}, fmt.Errorf("provider %q traz configuração de agente mas apiFormat é %q; use %q", normalized.ID, normalized.APIFormat, acpAPIFormat)
	}
	if normalized.BaseURL == "" {
		return ProviderExport{}, fmt.Errorf("provider %q sem baseUrl não pode ser importado", normalized.ID)
	}
	return normalized, nil
}

// encodeACPList e encodeACPMap gravam lista e mapa do agente com a mesma
// convenção do store de providers: coleção vazia é coluna vazia, e não o
// literal "[]" ou "{}". Um arquivo de importação escrito à mão pode trazer
// "acpArgs": [], e gravar o literal deixaria a linha de um provider HTTP com
// configuração de agente escrita — exatamente o que a validação recusa quando
// ela vem preenchida.
func encodeACPList(values []string) (string, error) {
	if len(values) == 0 {
		return "", nil
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func encodeACPMap(values map[string]string) (string, error) {
	if len(values) == 0 {
		return "", nil
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func isACPExport(provider ProviderExport) bool {
	return strings.TrimSpace(provider.APIFormat) == acpAPIFormat
}

// acpCommandWarning conta que o agente importado não existe nesta máquina.
// Caminho de binário é a parte do provider que não viaja: quem exportou pode
// ter o Cursor em outro lugar, ou não tê-lo instalado aqui. O provider entra
// assim mesmo — corrigir o comando é editá-lo, como em qualquer outro — mas
// entrar calado faria a primeira conversa falhar sem explicação.
func acpCommandWarning(provider ProviderExport) string {
	if !isACPExport(provider) {
		return ""
	}
	command := strings.TrimSpace(provider.ACPCommand)
	if command == "" {
		return ""
	}
	if _, err := exec.LookPath(command); err == nil {
		return ""
	}
	return fmt.Sprintf("Provider %q usa o agente %q, que não foi encontrado nesta máquina. Instale o agente ou edite o comando do provider antes de usá-lo.", provider.ID, command)
}

func findExistingProviderByID(ctx context.Context, providerID string) (*database.LLMProvider, error) {
	var provider database.LLMProvider
	err := database.ScopeByUser(ctx, database.DB(), "user_id").Where("id = ?", strings.TrimSpace(providerID)).First(&provider).Error
	if err == nil {
		return &provider, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return nil, fmt.Errorf("erro ao localizar provider %q: %w", providerID, err)
}
