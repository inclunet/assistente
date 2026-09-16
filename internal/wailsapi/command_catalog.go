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
	mu       sync.RWMutex
	session  Session
	registry *commandcatalog.Registry
}

func NewCommandCatalog() *CommandCatalog {
	return &CommandCatalog{}
}

// AttachCommandCatalog associa Session e snapshot canônico ao bind.
// Função de pacote para não entrar no Bind do Wails.
func AttachCommandCatalog(api *CommandCatalog, session Session, registry *commandcatalog.Registry) {
	if api == nil {
		return
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	api.session = session
	api.registry = registry
}

func (api *CommandCatalog) deps() (Session, *commandcatalog.Registry, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()
	if api.session == nil || api.registry == nil {
		return nil, nil, ErrCommandCatalogNotWired
	}
	return api.session, api.registry, nil
}

func (api *CommandCatalog) ListCommands(filter apidto.CommandCatalogFilter) ([]apidto.CommandCatalogItem, error) {
	session, registry, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(context.Context) ([]apidto.CommandCatalogItem, error) {
		locale := normalizeCommandLocale(filter.Locale)
		source := commandCatalogSource(filter.Source)
		definitions := registry.List()
		if strings.TrimSpace(filter.Query) != "" {
			definitions = registry.Search(locale, filter.Query)
		}
		out := make([]apidto.CommandCatalogItem, 0, len(definitions))
		for _, definition := range definitions {
			out = append(out, commandCatalogItemFrom(registry, definition, locale, source))
		}
		return out, nil
	})
}

func (api *CommandCatalog) DescribeCommand(id string, filter apidto.CommandCatalogFilter) (apidto.CommandCatalogDetail, error) {
	id = strings.TrimSpace(id)
	session, registry, err := api.deps()
	if err != nil {
		return apidto.CommandCatalogDetail{}, err
	}
	return WithUser(session, func(context.Context) (apidto.CommandCatalogDetail, error) {
		if id == "" {
			return apidto.CommandCatalogDetail{}, errors.New("comando sem identificador")
		}
		definition, ok := registry.Lookup(id)
		if !ok {
			return apidto.CommandCatalogDetail{}, commandcatalog.ErrNotReady
		}
		locale := normalizeCommandLocale(filter.Locale)
		item := commandCatalogItemFrom(registry, definition, locale, commandCatalogSource(filter.Source))
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

func commandCatalogItemFrom(registry *commandcatalog.Registry, definition commandcatalog.Definition, locale string, source commandcatalog.Source) apidto.CommandCatalogItem {
	metadata := commandcatalog.LocalizedMetadata{}
	presentationVersion := ""
	icon := ""
	if definition.Presentation != nil {
		metadata = definition.Presentation.Locales[locale]
		presentationVersion = definition.Presentation.Version
		icon = definition.Presentation.Icon
	}
	readinessReason := ""
	if _, err := registry.CheckReadiness(definition.ID, source); err != nil {
		readinessReason = err.Error()
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
