package wailsapi

import (
	"context"
	"errors"
	"strings"
	"sync"

	"assistente/internal/apidto"
	"assistente/internal/commandcatalog"
)

// CommandCatalog é o bind Wails do catálogo canônico de comandos (AEP-0103).
// Ele é somente leitura/execução via serviço confiável: não mantém listas
// paralelas no frontend e exige sessão autenticada para qualquer consulta.
type CommandCatalog struct {
	mu        sync.RWMutex
	session   Session
	registry  *commandcatalog.Registry
	readiness CommandCatalogReadiness
}

// CommandCatalogReadiness é fornecido pelo bootstrap confiável. O catálogo
// continua declarando AllowedSources, mas só esta porta conhece quais delas
// têm adapter operacional publicado no runtime atual. Ela também deve validar
// a política de preparação: o preflight read-only legado não descreve handlers
// de escrita montados pelo produto. Esta porta não é controlada pelo frontend.
type CommandCatalogReadiness func(context.Context, commandcatalog.Definition, commandcatalog.Source) error

var errCommandCatalogBackendUnavailable = errors.New("catálogo de comandos indisponível no runtime")

func NewCommandCatalog() *CommandCatalog {
	return &CommandCatalog{}
}

// AttachCommandCatalog associa Session e snapshot canônico ao bind.
// Função de pacote para não entrar no Bind do Wails.
func AttachCommandCatalog(api *CommandCatalog, session Session, registry *commandcatalog.Registry, readiness CommandCatalogReadiness) {
	if api == nil {
		return
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	api.session = session
	api.registry = registry
	api.readiness = readiness
}

func (api *CommandCatalog) deps() (Session, *commandcatalog.Registry, CommandCatalogReadiness, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()
	if api.session == nil || api.registry == nil {
		return nil, nil, nil, ErrCommandCatalogNotWired
	}
	return api.session, api.registry, api.readiness, nil
}

func (api *CommandCatalog) ListCommands(filter apidto.CommandCatalogFilter) ([]apidto.CommandCatalogItem, error) {
	session, registry, readiness, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]apidto.CommandCatalogItem, error) {
		locale := normalizeCommandLocale(filter.Locale)
		source := commandCatalogSource(filter.Source)
		definitions := registry.List()
		if strings.TrimSpace(filter.Query) != "" {
			definitions = registry.Search(locale, filter.Query)
		}
		out := make([]apidto.CommandCatalogItem, 0, len(definitions))
		for _, definition := range definitions {
			out = append(out, commandCatalogItemFrom(ctx, definition, locale, source, readiness))
		}
		return out, nil
	})
}

func (api *CommandCatalog) DescribeCommand(id string, filter apidto.CommandCatalogFilter) (apidto.CommandCatalogDetail, error) {
	id = strings.TrimSpace(id)
	session, registry, readiness, err := api.deps()
	if err != nil {
		return apidto.CommandCatalogDetail{}, err
	}
	return WithUser(session, func(ctx context.Context) (apidto.CommandCatalogDetail, error) {
		if id == "" {
			return apidto.CommandCatalogDetail{}, errors.New("comando sem identificador")
		}
		definition, ok := registry.Lookup(id)
		if !ok {
			return apidto.CommandCatalogDetail{}, commandcatalog.ErrNotReady
		}
		locale := normalizeCommandLocale(filter.Locale)
		item := commandCatalogItemFrom(ctx, definition, locale, commandCatalogSource(filter.Source), readiness)
		return apidto.CommandCatalogDetail{
			CommandCatalogItem:         item,
			ArgumentsSchema:            definition.ArgumentsSchema,
			ResultSchema:               definition.ResultSchema,
			HasMutableTarget:           definition.HasMutableTarget,
			MutatesEffectiveCapability: definition.MutatesEffectiveCapability,
			ContextNone:                definition.Context.None,
		}, nil
	})
}

func normalizeCommandLocale(locale string) string {
	switch strings.TrimSpace(locale) {
	case "en", "es", "pt-BR":
		return strings.TrimSpace(locale)
	default:
		return "pt-BR"
	}
}

func commandCatalogSource(source string) commandcatalog.Source {
	switch commandcatalog.Source(strings.TrimSpace(source)) {
	case commandcatalog.KeyboardLocal, commandcatalog.KeyboardGlobal, commandcatalog.StreamDeck, commandcatalog.Palette, commandcatalog.UI, commandcatalog.Chat, commandcatalog.CLI, commandcatalog.Event, commandcatalog.System:
		return commandcatalog.Source(strings.TrimSpace(source))
	default:
		return commandcatalog.Palette
	}
}

func commandCatalogItemFrom(ctx context.Context, definition commandcatalog.Definition, locale string, source commandcatalog.Source, readiness CommandCatalogReadiness) apidto.CommandCatalogItem {
	metadata := commandcatalog.LocalizedMetadata{}
	presentationVersion := ""
	icon := ""
	if definition.Presentation != nil {
		metadata = definition.Presentation.Locales[locale]
		presentationVersion = definition.Presentation.Version
		icon = definition.Presentation.Icon
	}
	readinessReason := ""
	// Invariantes de descoberta são comuns; efeitos e alvos suportados são
	// responsabilidade da política confiável publicada junto do runtime.
	if !definition.AllowsSource(source) {
		readinessReason = "origem não permitida para o comando"
	} else if definition.Presentation == nil {
		readinessReason = "apresentação do comando ausente"
	}
	if readiness == nil {
		readinessReason = errCommandCatalogBackendUnavailable.Error()
	} else if readinessReason == "" {
		if err := readiness(ctx, definition, source); err != nil {
			readinessReason = err.Error()
		}
	}
	return apidto.CommandCatalogItem{
		ID:                  definition.ID,
		Name:                metadata.Name,
		Description:         metadata.Description,
		Category:            metadata.Category,
		Aliases:             append([]string(nil), metadata.Aliases...),
		Icon:                icon,
		Effect:              string(definition.Effect),
		Risk:                string(definition.Risk),
		Decision:            string(definition.Decision),
		Available:           definition.Availability.Status == commandcatalog.Available && readinessReason == "",
		AvailabilityStatus:  string(definition.Availability.Status),
		AvailabilityReason:  definition.Availability.Reason,
		ReadinessReason:     readinessReason,
		AllowedSources:      commandCatalogSources(definition.AllowedSources),
		Scopes:              commandCatalogScopes(definition.Scopes),
		PresentationVersion: presentationVersion,
	}
}

func commandCatalogSources(values []commandcatalog.Source) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

func commandCatalogScopes(values []commandcatalog.Scope) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}
