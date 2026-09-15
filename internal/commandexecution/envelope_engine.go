package commandexecution

import (
	"fmt"

	"assistente/internal/commandcatalog"
)

// NewComplete publica o Service para o pipeline de envelopes I04.
//
// A publicação é deliberadamente separada de New: o construtor legado aceita
// somente o recorte read/local_session, enquanto este construtor exige um
// snapshot completo do catálogo e valida os contratos reais dos handlers
// instalados. Assim, um descriptor canônico não pode ser usado para publicar
// uma rota, classificação ou efeito diferente do handler que será executado.
//
// O método retorna o mesmo tipo Service para preservar a API do pacote. Os
// métodos legados Execute/GetInvocation ficam desabilitados no serviço
// completo; o pipeline de envelopes deve ser a única entrada de execução.
func NewComplete(config Config) (*Service, error) {
	if err := validateCompleteBootstrap(config); err != nil {
		return nil, err
	}
	handlers := make(map[string]Handler, len(config.Handlers))
	for id, handler := range config.Handlers {
		handlers[id] = handler
	}
	config.Handlers = handlers
	ports := *config.Envelope
	if config.Envelope.Identity != nil {
		identityPorts := *config.Envelope.Identity
		ports.Identity = &identityPorts
	}
	config.Envelope = &ports
	return &Service{config: config, complete: true}, nil
}

func validateCompleteBootstrap(config Config) error {
	if config.Envelope == nil || config.Envelope.Context == nil {
		return ErrInvalidConfiguration
	}
	if config.Envelope.Identity != nil {
		if !config.Envelope.Identity.complete() {
			return ErrInvalidConfiguration
		}
	} else if config.Sessions == nil || config.Envelope.Snapshot == nil || config.Envelope.Resolve == nil || config.Envelope.Authorize == nil || config.Envelope.AuthorizeLookup == nil || config.Envelope.Actor == nil {
		return ErrInvalidConfiguration
	}
	switch config.Source {
	case commandcatalog.Palette, commandcatalog.UI, commandcatalog.CLI, commandcatalog.Chat, commandcatalog.KeyboardLocal, commandcatalog.KeyboardGlobal, commandcatalog.StreamDeck:
	case commandcatalog.Event, commandcatalog.System:
		if config.Envelope.Identity == nil {
			return ErrInvalidConfiguration
		}
	default:
		return ErrInvalidConfiguration
	}
	if config.Envelope.Decisions != nil && config.Envelope.DecisionTTL <= 0 {
		return ErrInvalidConfiguration
	}
	if config.Epochs == nil || config.Store == nil || config.Registry == nil ||
		config.Keys == nil || config.Now == nil || config.Retention <= 0 ||
		config.ExecutionTimeout <= 0 || config.FinalizationTimeout <= 0 ||
		!nonblank(config.RegistryVersion) || !keyVersionPattern.MatchString(config.KeyVersion) {
		return ErrInvalidConfiguration
	}
	if !config.Registry.Complete() || len(config.Handlers) == 0 {
		return ErrInvalidConfiguration
	}
	if err := validateCompleteHandlerContracts(config.Registry, config.Handlers); err != nil {
		return err
	}
	return nil
}

// validateCompleteHandlerContracts exige cobertura bijetiva entre o snapshot
// publicado e os handlers executáveis. O contrato do handler é sempre a prova
// obtida no bootstrap; nunca é lido do envelope ou de argumentos.
func validateCompleteHandlerContracts(registry *commandcatalog.Registry, handlers map[string]Handler) error {
	if registry == nil || !registry.Complete() || len(handlers) != len(registry.List()) {
		return ErrInvalidConfiguration
	}
	for id, handler := range handlers {
		if handler.Start == nil {
			return fmt.Errorf("%w: handler %q sem Start", ErrInvalidConfiguration, id)
		}
		definition, ok := registry.Lookup(id)
		if !ok {
			return fmt.Errorf("%w: handler %q ausente no catálogo", ErrInvalidConfiguration, id)
		}
		if err := commandcatalog.ValidateDefinitionComplete(definition, handler.Contract); err != nil {
			return fmt.Errorf("%w: contrato do handler %q: %v", ErrInvalidConfiguration, id, err)
		}
	}
	for _, definition := range registry.List() {
		if _, ok := handlers[definition.ID]; !ok {
			return fmt.Errorf("%w: catálogo %q sem handler", ErrInvalidConfiguration, definition.ID)
		}
	}
	return nil
}
