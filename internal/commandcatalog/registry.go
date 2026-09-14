// Package commandcatalog inicia o registro canônico da AEP-0103 D2.
// Este incremento valida contratos de efeito, origem e contexto. Não expõe
// execução: autenticação, argumentos, disponibilidade e despacho serão ligados
// ao CommandExecutionService antes de qualquer comando entrar no aplicativo.
package commandcatalog

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

type Effect string

const (
	Read        Effect = "read"
	Write       Effect = "write"
	Destructive Effect = "destructive"
)

type Source string

const (
	KeyboardLocal  Source = "keyboard.local"
	KeyboardGlobal Source = "keyboard.global"
	StreamDeck     Source = "streamdeck.key"
	Palette        Source = "palette"
	UI             Source = "ui.action"
	Chat           Source = "chat"
	CLI            Source = "cli"
	Event          Source = "event"
	System         Source = "system"
)

type Decision string

const (
	NoDecision  Decision = "none"
	Interactive Decision = "interactive"
)

type ContextMode string

const (
	ExactVersion  ContextMode = "exact_version"
	MaxAge        ContextMode = "max_age_ms"
	EventSnapshot ContextMode = "event_snapshot"
)

// ContextFact descreve uma exigência de um provider. MaxAgeMS é obrigatório
// e positivo nos modos baseados em idade; exact_version não usa idade.
type ContextFact struct {
	Provider string
	Fact     string
	Mode     ContextMode
	MaxAgeMS int64
}

// ContextPolicy usa None exclusivamente para leituras independentes do alvo.
// Provider ausente em runtime deverá tornar o comando indisponível.
type ContextPolicy struct {
	None  bool
	Facts []ContextFact
}

// LocalizedMetadata é a apresentação versionada de um comando em um locale.
type LocalizedMetadata struct {
	Name        string
	Description string
	Category    string
	Aliases     []string
}

// Presentation contém somente metadata de apresentação. Ela é opcional para
// preservar contratos de comandos que ainda estão em estágio inicial.
type Presentation struct {
	Version string
	Locales map[string]LocalizedMetadata
}

type Definition struct {
	ID                         string
	Effect                     Effect
	Decision                   Decision
	MutatesEffectiveCapability bool
	HasMutableTarget           bool
	AllowedSources             []Source
	Context                    ContextPolicy
	Presentation               *Presentation
}

// HandlerContract é obtido pelo bootstrap a partir do handler confiável, nunca
// de payload/configuração do usuário. Não contém a rota de execução.
type HandlerContract struct {
	Effect                     Effect
	MutatesEffectiveCapability bool
}

type Registration struct {
	Definition Definition
	Handler    HandlerContract
}

// Registry é um snapshot imutável. Mudanças exigem construir outro snapshot;
// publicação/versionamento sob DispatchGate pertence à integração futura.
type Registry struct{ definitions map[string]Definition }

var commandID = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

func New(registrations []Registration) (*Registry, error) {
	r := &Registry{definitions: make(map[string]Definition, len(registrations))}
	for _, registration := range registrations {
		d := registration.Definition
		if err := validate(d, registration.Handler); err != nil {
			return nil, fmt.Errorf("comando %q: %w", d.ID, err)
		}
		if _, exists := r.definitions[d.ID]; exists {
			return nil, fmt.Errorf("comando duplicado: %s", d.ID)
		}
		r.definitions[d.ID] = clone(d)
	}
	return r, nil
}

func clone(d Definition) Definition {
	d.AllowedSources = slices.Clone(d.AllowedSources)
	d.Context.Facts = slices.Clone(d.Context.Facts)
	if d.Presentation != nil {
		p := &Presentation{Version: d.Presentation.Version, Locales: make(map[string]LocalizedMetadata, len(d.Presentation.Locales))}
		for locale, metadata := range d.Presentation.Locales {
			metadata.Aliases = slices.Clone(metadata.Aliases)
			p.Locales[locale] = metadata
		}
		d.Presentation = p
	}
	return d
}

// Lookup exige ID exato. Aliases e busca futura nunca substituem a identidade
// canônica que o executor recebe.
func (r *Registry) Lookup(id string) (Definition, bool) {
	d, ok := r.definitions[id]
	return clone(d), ok
}

func (r *Registry) List() []Definition {
	ids := make([]string, 0, len(r.definitions))
	for id := range r.definitions {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	result := make([]Definition, 0, len(ids))
	for _, id := range ids {
		result = append(result, clone(r.definitions[id]))
	}
	return result
}

// AllowsSource consulta somente a declaração estática. true não é autorização:
// o executor ainda deve autenticar a origem e aplicar todos os gates.
func (d Definition) AllowsSource(source Source) bool {
	return slices.Contains(d.AllowedSources, source)
}

func validate(d Definition, h HandlerContract) error {
	if !commandID.MatchString(d.ID) {
		return fmt.Errorf("ID deve ser namespaced e canônico")
	}
	if d.Effect != Read && d.Effect != Write && d.Effect != Destructive {
		return fmt.Errorf("efeito inválido")
	}
	if d.Decision != NoDecision && d.Decision != Interactive {
		return fmt.Errorf("requisito de decisão inválido")
	}
	if d.Effect != h.Effect || d.MutatesEffectiveCapability != h.MutatesEffectiveCapability {
		return fmt.Errorf("metadata diverge do contrato do handler")
	}
	if err := validatePresentation(d.Presentation); err != nil {
		return err
	}
	if len(d.AllowedSources) == 0 {
		return fmt.Errorf("origens permitidas são obrigatórias")
	}
	seen := make(map[Source]bool)
	for _, source := range d.AllowedSources {
		switch source {
		case KeyboardLocal, KeyboardGlobal, StreamDeck, Palette, UI, Chat, CLI, Event, System:
		default:
			return fmt.Errorf("origem desconhecida: %s", source)
		}
		if seen[source] {
			return fmt.Errorf("origem repetida: %s", source)
		}
		seen[source] = true
		if d.Effect == Destructive && (source == CLI || source == Event || source == System) {
			return fmt.Errorf("origem headless proibida para efeito destrutivo")
		}
	}
	if d.Effect == Destructive && d.Decision != Interactive {
		return fmt.Errorf("efeito destrutivo exige decisão interativa")
	}
	if d.Context.None {
		if len(d.Context.Facts) != 0 {
			return fmt.Errorf("contexto none não declara providers")
		}
		if d.Effect != Read || d.HasMutableTarget || d.MutatesEffectiveCapability {
			return fmt.Errorf("contexto none exige leitura sem alvo mutável nem alteração de capacidade")
		}
		return nil
	}
	if len(d.Context.Facts) == 0 {
		return fmt.Errorf("contexto exige ao menos um fato")
	}
	seenFacts := make(map[[2]string]bool)
	for _, fact := range d.Context.Facts {
		if strings.TrimSpace(fact.Provider) == "" || strings.TrimSpace(fact.Fact) == "" {
			return fmt.Errorf("provider e fato são obrigatórios")
		}
		key := [2]string{fact.Provider, fact.Fact}
		if seenFacts[key] {
			return fmt.Errorf("fato repetido no provider")
		}
		seenFacts[key] = true
		switch fact.Mode {
		case ExactVersion:
			if fact.MaxAgeMS != 0 {
				return fmt.Errorf("exact_version não admite idade")
			}
		case MaxAge, EventSnapshot:
			if fact.MaxAgeMS <= 0 {
				return fmt.Errorf("política temporal exige idade positiva")
			}
		default:
			return fmt.Errorf("modo de contexto inválido")
		}
	}
	return nil
}

func validatePresentation(p *Presentation) error {
	if p == nil {
		return nil
	}
	if strings.TrimSpace(p.Version) == "" {
		return fmt.Errorf("versão da apresentação é obrigatória")
	}
	if len(p.Locales) != len(supportedLocales) {
		return fmt.Errorf("apresentação deve conter exatamente pt-BR, en e es")
	}
	for locale := range p.Locales {
		if _, ok := supportedLocales[locale]; !ok {
			return fmt.Errorf("locale de apresentação desconhecido: %s", locale)
		}
	}
	for locale, metadata := range p.Locales {
		if strings.TrimSpace(metadata.Name) == "" || strings.TrimSpace(metadata.Description) == "" || strings.TrimSpace(metadata.Category) == "" {
			return fmt.Errorf("metadata incompleta no locale %s", locale)
		}
		seen := make(map[string]struct{}, len(metadata.Aliases))
		for _, alias := range metadata.Aliases {
			normalized := normalize(alias)
			if normalized == "" {
				return fmt.Errorf("alias vazio no locale %s", locale)
			}
			if _, ok := seen[normalized]; ok {
				return fmt.Errorf("alias repetido no locale %s", locale)
			}
			seen[normalized] = struct{}{}
		}
	}
	return nil
}
