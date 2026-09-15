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
	"unicode/utf8"
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

// Risk é a classificação estática de risco do comando. A autorização efetiva
// não é derivada deste valor; ele é parte do contrato que o executor revalida.
type Risk string

const (
	RiskLow      Risk = "low"
	RiskMedium   Risk = "medium"
	RiskHigh     Risk = "high"
	RiskCritical Risk = "critical"
)

type Scope string

const (
	ScopeSession      Scope = "session"
	ScopeConversation Scope = "conversation"
	ScopeWorkspace    Scope = "workspace"
	ScopeProfile      Scope = "profile"
	ScopeDialog       Scope = "dialog"
	ScopeSurface      Scope = "surface"
	ScopeApplication  Scope = "application"
	ScopeGlobal       Scope = "global"
)

type AvailabilityStatus string

const (
	Available   AvailabilityStatus = "available"
	Unavailable AvailabilityStatus = "unavailable"
)

type Availability struct {
	Status AvailabilityStatus
	// Reason é um código estável (não texto livre), adequado para logs e
	// tradução posterior pela superfície que o apresenta.
	Reason string
}

type PersistenceMode string

const (
	PersistenceNever     PersistenceMode = "never"
	PersistencePlaintext PersistenceMode = "plaintext"
	PersistenceRedacted  PersistenceMode = "redacted"
	PersistenceSummary   PersistenceMode = "summary"
)

// PersistencePolicy declara separadamente como argumentos, resultado e
// auditoria podem ser persistidos. O catálogo não persiste dados por si só.
type PersistencePolicy struct {
	Arguments PersistenceMode
	Result    PersistenceMode
	Audit     PersistenceMode
}

type SensitivePaths struct {
	Input  []string
	Output []string
}

type PresentationState string

const (
	PresentationStateDefault     PresentationState = "default"
	PresentationStateDisabled    PresentationState = "disabled"
	PresentationStateUnavailable PresentationState = "unavailable"
	PresentationStateActive      PresentationState = "active"
)

type HandlerClassification string

const (
	HandlerInternal HandlerClassification = "internal"
	HandlerBackend  HandlerClassification = "backend"
	HandlerUI       HandlerClassification = "ui"
	HandlerTool     HandlerClassification = "tool"
	HandlerJob      HandlerClassification = "job"
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
	Icon    string
	States  map[string]string
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
	ArgumentsSchema            *Schema
	ResultSchema               *Schema
	Risk                       Risk
	SensitivePaths             SensitivePaths
	Persistence                PersistencePolicy
	Scopes                     []Scope
	Availability               Availability
	HandlerRoute               string
	HandlerClassification      HandlerClassification
}

// HandlerContract é obtido pelo bootstrap a partir do handler confiável, nunca
// de payload/configuração do usuário. Rota e classificação são parte dessa
// prova confiável; o catálogo não os usa para executar nada.
type HandlerContract struct {
	Effect                     Effect
	HasMutableTarget           bool
	MutatesEffectiveCapability bool
	Route                      string
	Classification             HandlerClassification
}

type Registration struct {
	Definition Definition
	Handler    HandlerContract
}

// Registry é um snapshot imutável. Mudanças exigem construir outro snapshot;
// publicação/versionamento sob DispatchGate pertence à integração futura.
type Registry struct {
	definitions map[string]Definition
	complete    bool
}

var commandID = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

func New(registrations []Registration) (*Registry, error) {
	return newRegistry(registrations, false)
}

// NewComplete exige o contrato integral da AEP-0103. New permanece como
// construtor compatível do protótipo e nunca produz um registry completo.
func NewComplete(registrations []Registration) (*Registry, error) {
	return newRegistry(registrations, true)
}

func newRegistry(registrations []Registration, complete bool) (*Registry, error) {
	r := &Registry{definitions: make(map[string]Definition, len(registrations)), complete: complete}
	for _, registration := range registrations {
		d := registration.Definition
		var err error
		if complete {
			err = ValidateDefinitionComplete(d, registration.Handler)
		} else {
			err = validate(d, registration.Handler)
		}
		if err != nil {
			return nil, fmt.Errorf("comando %q: %w", d.ID, err)
		}
		if _, exists := r.definitions[d.ID]; exists {
			return nil, fmt.Errorf("comando duplicado: %s", d.ID)
		}
		r.definitions[d.ID] = clone(d)
	}
	return r, nil
}

// Complete informa se o snapshot foi criado pelo construtor rigoroso.
func (r *Registry) Complete() bool {
	return r != nil && r.complete
}

// IsComplete informa se a definição contém todos os campos necessários para
// o construtor rigoroso. A consistência com o descriptor do handler é
// verificada por ValidateDefinitionComplete.
func (d Definition) IsComplete() bool {
	return validateCompleteDefinition(d) == nil
}

func clone(d Definition) Definition {
	d.AllowedSources = slices.Clone(d.AllowedSources)
	d.Context.Facts = slices.Clone(d.Context.Facts)
	d.Scopes = slices.Clone(d.Scopes)
	d.SensitivePaths.Input = slices.Clone(d.SensitivePaths.Input)
	d.SensitivePaths.Output = slices.Clone(d.SensitivePaths.Output)
	d.ArgumentsSchema = cloneSchema(d.ArgumentsSchema)
	d.ResultSchema = cloneSchema(d.ResultSchema)
	if d.Presentation != nil {
		p := &Presentation{Version: d.Presentation.Version, Icon: d.Presentation.Icon, States: make(map[string]string, len(d.Presentation.States)), Locales: make(map[string]LocalizedMetadata, len(d.Presentation.Locales))}
		for state, icon := range d.Presentation.States {
			p.States[state] = icon
		}
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
	if strings.TrimSpace(p.Icon) != "" && !validPresentationToken(p.Icon) {
		return fmt.Errorf("ícone da apresentação inválido")
	}
	for state, icon := range p.States {
		if !validPresentationToken(state) || !validPresentationToken(icon) {
			return fmt.Errorf("estado/ícone da apresentação inválido")
		}
	}
	return nil
}

// ValidateDefinitionComplete valida uma definição contra o descriptor
// confiável do handler. O descriptor não é aceito de argumentos do usuário.
func ValidateDefinitionComplete(d Definition, h HandlerContract) error {
	if err := validate(d, h); err != nil {
		return err
	}
	if err := validateCompleteDefinition(d); err != nil {
		return err
	}
	if d.HandlerRoute != h.Route || d.HandlerClassification != h.Classification {
		return fmt.Errorf("rota/classificação divergem do contrato do handler")
	}
	if d.HasMutableTarget != h.HasMutableTarget {
		return fmt.Errorf("mutabilidade do alvo diverge do contrato do handler")
	}
	return nil
}

func validateCompleteDefinition(d Definition) error {
	if d.ArgumentsSchema == nil || d.ResultSchema == nil {
		return fmt.Errorf("schemas de argumentos e resultado são obrigatórios")
	}
	if err := validateSchemaDefinition(d.ArgumentsSchema, "arguments", true); err != nil {
		return err
	}
	if err := validateSchemaDefinition(d.ResultSchema, "result", true); err != nil {
		return err
	}
	if !validRisk(d.Risk) {
		return fmt.Errorf("risco inválido")
	}
	if err := validatePaths(d.SensitivePaths); err != nil {
		return err
	}
	if err := validatePersistence(d.Persistence); err != nil {
		return err
	}
	if len(d.Scopes) == 0 {
		return fmt.Errorf("escopos são obrigatórios")
	}
	seenScopes := make(map[Scope]struct{}, len(d.Scopes))
	for _, scope := range d.Scopes {
		if !validScope(scope) {
			return fmt.Errorf("escopo desconhecido: %s", scope)
		}
		if _, duplicate := seenScopes[scope]; duplicate {
			return fmt.Errorf("escopo repetido: %s", scope)
		}
		seenScopes[scope] = struct{}{}
	}
	if err := validateAvailability(d.Availability); err != nil {
		return err
	}
	if !validExactRoute(d.HandlerRoute) {
		return fmt.Errorf("rota do handler é obrigatória")
	}
	if !validHandlerClassification(d.HandlerClassification) {
		return fmt.Errorf("classificação do handler inválida")
	}
	if err := validatePresentationComplete(d.Presentation); err != nil {
		return err
	}
	if err := validateSensitivePathsAgainstSchemas(d); err != nil {
		return err
	}
	return nil
}

func validRisk(value Risk) bool {
	switch value {
	case RiskLow, RiskMedium, RiskHigh, RiskCritical:
		return true
	default:
		return false
	}
}

func validScope(value Scope) bool {
	switch value {
	case ScopeSession, ScopeConversation, ScopeWorkspace, ScopeProfile, ScopeDialog, ScopeSurface, ScopeApplication, ScopeGlobal:
		return true
	default:
		return false
	}
}

func validatePaths(paths SensitivePaths) error {
	seen := map[string]struct{}{}
	for kind, values := range map[string][]string{"input": paths.Input, "output": paths.Output} {
		for _, value := range values {
			if !validJSONPointer(value) {
				return fmt.Errorf("path sensível %s inválido", kind)
			}
			if _, duplicate := seen[kind+"\x00"+value]; duplicate {
				return fmt.Errorf("path sensível %s repetido", kind)
			}
			seen[kind+"\x00"+value] = struct{}{}
		}
	}
	return nil
}

func validatePersistence(policy PersistencePolicy) error {
	for kind, mode := range map[string]PersistenceMode{
		"arguments": policy.Arguments,
		"result":    policy.Result,
		"audit":     policy.Audit,
	} {
		switch mode {
		case PersistenceNever, PersistencePlaintext, PersistenceRedacted, PersistenceSummary:
		default:
			return fmt.Errorf("política de persistência %s inválida", kind)
		}
	}
	return nil
}

func validateAvailability(availability Availability) error {
	switch availability.Status {
	case Available:
		if strings.TrimSpace(availability.Reason) != "" {
			return fmt.Errorf("comando disponível não deve ter motivo")
		}
	case Unavailable:
		if !validReasonCode(availability.Reason) {
			return fmt.Errorf("comando indisponível exige motivo")
		}
	default:
		return fmt.Errorf("estado de disponibilidade inválido")
	}
	return nil
}

func validatePresentationComplete(p *Presentation) error {
	if err := validatePresentation(p); err != nil {
		return err
	}
	if p == nil {
		return fmt.Errorf("apresentação é obrigatória")
	}
	return nil
}

func validExactRoute(route string) bool {
	return route != "" && route == strings.TrimSpace(route) && utf8.ValidString(route) && !strings.ContainsRune(route, '\x00')
}

func validHandlerClassification(value HandlerClassification) bool {
	switch value {
	case HandlerInternal, HandlerBackend, HandlerUI, HandlerTool, HandlerJob:
		return true
	default:
		return false
	}
}

var reasonCode = regexp.MustCompile(`^[a-z][a-z0-9_.-]*$`)
var presentationToken = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.:/-]*$`)

func validReasonCode(value string) bool {
	return reasonCode.MatchString(value)
}

func validPresentationToken(value string) bool {
	return presentationToken.MatchString(value)
}

func validJSONPointer(value string) bool {
	if value == "" {
		return true
	}
	if !utf8.ValidString(value) || value[0] != '/' || strings.ContainsRune(value, '\x00') {
		return false
	}
	for i := 1; i < len(value); i++ {
		if value[i] != '~' {
			continue
		}
		if i+1 >= len(value) || (value[i+1] != '0' && value[i+1] != '1') {
			return false
		}
		i++
	}
	return true
}

func validateSensitivePathsAgainstSchemas(d Definition) error {
	if len(d.SensitivePaths.Input) != 0 {
		if d.Persistence.Arguments == PersistencePlaintext {
			return fmt.Errorf("persistência plaintext proibida para paths sensíveis de argumentos")
		}
		for _, path := range d.SensitivePaths.Input {
			if _, ok := schemaAtJSONPointer(d.ArgumentsSchema, path); !ok {
				return fmt.Errorf("path sensível de argumentos não existe no schema")
			}
		}
	}
	if len(d.SensitivePaths.Output) != 0 {
		if d.Persistence.Result == PersistencePlaintext {
			return fmt.Errorf("persistência plaintext proibida para paths sensíveis de resultado")
		}
		for _, path := range d.SensitivePaths.Output {
			if _, ok := schemaAtJSONPointer(d.ResultSchema, path); !ok {
				return fmt.Errorf("path sensível de resultado não existe no schema")
			}
		}
	}
	return nil
}

func schemaAtJSONPointer(root *Schema, pointer string) (*Schema, bool) {
	if root == nil {
		return nil, false
	}
	if pointer == "" {
		return root, true
	}
	current := root
	for _, encoded := range strings.Split(pointer[1:], "/") {
		segment, ok := decodeJSONPointerSegment(encoded)
		if !ok {
			return nil, false
		}
		switch current.Type {
		case SchemaObject:
			property, exists := current.Properties[segment]
			if !exists {
				return nil, false
			}
			current = &property
		case SchemaArray:
			if segment == "" || (len(segment) > 1 && segment[0] == '0') {
				return nil, false
			}
			for _, char := range segment {
				if char < '0' || char > '9' {
					return nil, false
				}
			}
			if current.Items == nil {
				return nil, false
			}
			current = current.Items
		default:
			return nil, false
		}
	}
	return current, true
}

func decodeJSONPointerSegment(value string) (string, bool) {
	if !validJSONPointer("/" + value) {
		return "", false
	}
	var out strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] != '~' {
			out.WriteByte(value[i])
			continue
		}
		switch value[i+1] {
		case '0':
			out.WriteByte('~')
		case '1':
			out.WriteByte('/')
		}
		i++
	}
	return out.String(), true
}
