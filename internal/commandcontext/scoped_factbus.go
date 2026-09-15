package commandcontext

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"assistente/internal/commandcatalog"
	"github.com/google/uuid"
)

// Scope identifica o principal autenticado que pode observar um fato. Um
// workspace nil significa o escopo global daquele usuário; não significa
// "todos os workspaces".
type Scope struct {
	UserID        string
	AuthContextID string
	WorkspaceID   *string
}

var (
	ErrInvalidScope        = errors.New("escopo contextual inválido")
	ErrOwnerMismatch       = errors.New("owner do snapshot contextual divergente")
	ErrProviderUnavailable = errors.New("provider contextual indisponível")
	ErrInvalidProof        = errors.New("prova contextual inválida")
	ErrPolicyMismatch      = errors.New("policy contextual divergente")
)

const maxOpaqueScopeIDBytes = 256

func (s Scope) validate() error {
	if !canonicalUUID7(s.UserID) || !validOpaqueScopeID(s.AuthContextID) {
		return ErrInvalidScope
	}
	if s.WorkspaceID != nil && !validOpaqueScopeID(*s.WorkspaceID) {
		return ErrInvalidScope
	}
	return nil
}

func canonicalUUID7(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id.Version() == 7 && id.Variant() == uuid.RFC4122 && id.String() == value
}

// validOpaqueScopeID valida sem normalizar. Workspace IDs existentes são
// opacos (por exemplo ws-<timestamphex>), enquanto somente UserID exige UUIDv7.
func validOpaqueScopeID(value string) bool {
	return value != "" && len(value) <= maxOpaqueScopeIDBytes && utf8.ValidString(value) &&
		strings.TrimSpace(value) != "" && !strings.ContainsRune(value, '\x00')
}

// Validate verifica se o escopo contém a identidade autenticada completa.
func (s Scope) Validate() error { return s.validate() }

func cloneScopedScope(s Scope) Scope {
	out := s
	if s.WorkspaceID != nil {
		workspace := *s.WorkspaceID
		out.WorkspaceID = &workspace
	}
	return out
}

func sameScope(left, right Scope) bool {
	if left.UserID != right.UserID || left.AuthContextID != right.AuthContextID {
		return false
	}
	if left.WorkspaceID == nil || right.WorkspaceID == nil {
		return left.WorkspaceID == nil && right.WorkspaceID == nil
	}
	return *left.WorkspaceID == *right.WorkspaceID
}

// OwnedSnapshot é o resultado de uma consulta feita por um provider escopado.
// O FactBus nunca aceita um snapshot cujo owner não seja exatamente o escopo
// solicitado.
type OwnedSnapshot struct {
	Owner    Scope
	Snapshot Snapshot
}

// NewOwnedSnapshot é usado por providers confiáveis para devolver ownership
// junto do snapshot. Ele não cria uma prova de admissão: somente o FactBus
// pode produzir a prova opaca consumida na revalidação.
func NewOwnedSnapshot(owner Scope, snapshot Snapshot) (OwnedSnapshot, error) {
	if err := owner.validate(); err != nil {
		return OwnedSnapshot{}, err
	}
	owner = cloneScopedScope(owner)
	return OwnedSnapshot{Owner: owner, Snapshot: snapshot}, nil
}

// ScopedProvider consulta uma fonte autoritativa para um fato dentro de um
// escopo. Implementações devem reconsultar a fonte; cache local e notificações
// não são autoridade para Revalidate.
type ScopedProvider interface {
	Snapshot(context.Context, Scope, string) (OwnedSnapshot, error)
}

// ScopedProviderFunc adapta uma função a ScopedProvider.
type ScopedProviderFunc func(context.Context, Scope, string) (OwnedSnapshot, error)

func (f ScopedProviderFunc) Snapshot(ctx context.Context, scope Scope, fact string) (OwnedSnapshot, error) {
	return f(ctx, scope, fact)
}

// FactProof é deliberadamente opaca: scope, policy e snapshots ficam privados
// e só podem ser produzidos pelo FactBus. O frontend não recebe campos com os
// quais possa construir uma prova válida.
type FactProof struct{ state *factProofState }

type factProofState struct {
	scope     Scope
	policy    commandcatalog.ContextPolicy
	snapshots Snapshots
	version   string
	bus       *FactBus
}

// ContextProof é um nome descritivo alternativo para FactProof.
type ContextProof = FactProof

// ContextVersion expõe apenas o fingerprint necessário para indexar o cache;
// não expõe os snapshots privados da prova.
func (p FactProof) ContextVersion() string {
	if p.state == nil {
		return ""
	}
	return p.state.version
}

func (p FactProof) valid() bool { return p.state != nil }

// FactBus mantém um registro imutável de providers confiáveis. Não há método
// de registro posterior: trocar providers exige construir e publicar outro
// FactBus sob o gate do chamador.
type FactBus struct {
	mu        sync.RWMutex
	providers map[string]ScopedProvider
	subs      map[*factSubscription]struct{}
}

func NewFactBus(providers map[string]ScopedProvider) (*FactBus, error) {
	registry := make(map[string]ScopedProvider, len(providers))
	for id, provider := range providers {
		if strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("provider contextual com ID vazio")
		}
		if isNilScopedProvider(provider) {
			return nil, fmt.Errorf("provider contextual %q inválido: %w", id, ErrProviderUnavailable)
		}
		registry[id] = provider
	}
	return &FactBus{providers: registry, subs: make(map[*factSubscription]struct{})}, nil
}

// ContextFactBus é o nome usado pelo item I03.2; ambos denotam o mesmo bus.
type ContextFactBus = FactBus

func NewContextFactBus(providers map[string]ScopedProvider) (*FactBus, error) {
	return NewFactBus(providers)
}

func isNilScopedProvider(provider ScopedProvider) bool {
	if provider == nil {
		return true
	}
	v := reflect.ValueOf(provider)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

// Capture consulta cada provider exigido pela policy. Nenhum timestamp é
// preenchido pelo bus: em policies temporais ele exige CapturedAt vindo da
// fonte confiável, inclusive para event_snapshot.
func (b *FactBus) Capture(ctx context.Context, scope Scope, policy commandcatalog.ContextPolicy) (FactProof, error) {
	if ctx == nil {
		return FactProof{}, errors.New("contexto nil")
	}
	if b == nil {
		return FactProof{}, ErrProviderUnavailable
	}
	if err := scope.validate(); err != nil {
		return FactProof{}, err
	}
	if err := ctx.Err(); err != nil {
		return FactProof{}, err
	}
	if err := validatePolicy(policy); err != nil {
		return FactProof{}, err
	}
	snapshots, version, err := b.capture(ctx, scope, policy)
	if err != nil {
		return FactProof{}, err
	}
	return FactProof{state: &factProofState{
		scope: cloneScopedScope(scope), policy: cloneContextPolicy(policy),
		snapshots: cloneSnapshots(snapshots), version: version, bus: b,
	}}, nil
}

func (b *FactBus) capture(ctx context.Context, scope Scope, policy commandcatalog.ContextPolicy) (Snapshots, string, error) {
	if policy.None {
		return Snapshots{}, "", nil
	}
	captured := make(Snapshots, len(policy.Facts))
	for _, fact := range policy.Facts {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		b.mu.RLock()
		provider, ok := b.providers[fact.Provider]
		b.mu.RUnlock()
		if !ok {
			return nil, "", fmt.Errorf("provider %q ausente: %w", fact.Provider, ErrMissingSnapshot)
		}
		owned, err := readScopedProvider(ctx, provider, cloneScopedScope(scope), fact.Fact)
		if err != nil {
			return nil, "", fmt.Errorf("provider %q fato %q: %w", fact.Provider, fact.Fact, ErrProviderUnavailable)
		}
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		if err := validateOwnedSnapshot(owned, scope); err != nil {
			return nil, "", fmt.Errorf("provider %q fato %q: %w", fact.Provider, fact.Fact, err)
		}
		if strings.TrimSpace(owned.Snapshot.Version) == "" {
			return nil, "", fmt.Errorf("provider %q fato %q: %w", fact.Provider, fact.Fact, ErrVersionMismatch)
		}
		if fact.Mode == commandcatalog.MaxAge || fact.Mode == commandcatalog.EventSnapshot {
			if owned.Snapshot.CapturedAt.IsZero() {
				return nil, "", fmt.Errorf("provider %q fato %q: %w", fact.Provider, fact.Fact, ErrInvalidTimestamp)
			}
		}
		captured[FactKey{Provider: fact.Provider, Fact: fact.Fact}] = owned.Snapshot
	}
	return captured, fingerprint(captured), nil
}

func readScopedProvider(ctx context.Context, provider ScopedProvider, scope Scope, fact string) (snapshot OwnedSnapshot, err error) {
	defer func() {
		if recover() != nil {
			snapshot = OwnedSnapshot{}
			err = ErrProviderUnavailable
		}
	}()
	return provider.Snapshot(ctx, scope, fact)
}

func validateOwnedSnapshot(owned OwnedSnapshot, requested Scope) error {
	if err := requested.validate(); err != nil {
		return err
	}
	if err := owned.Owner.validate(); err != nil {
		return ErrOwnerMismatch
	}
	if !sameScope(owned.Owner, requested) {
		return ErrOwnerMismatch
	}
	return nil
}

// Revalidate sempre chama os providers registrados em sequência. Não usa
// snapshots notificados, cache ou a versão presente na prova como autoridade.
// A prova inicial permanece intacta, especialmente seu CapturedAt.
func (b *FactBus) Revalidate(ctx context.Context, scope Scope, policy commandcatalog.ContextPolicy, proof FactProof, now time.Time) error {
	if ctx == nil {
		return errors.New("contexto nil")
	}
	if b == nil {
		return ErrProviderUnavailable
	}
	if err := scope.validate(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validatePolicy(policy); err != nil {
		return err
	}
	if !proof.valid() || proof.state.bus != b || !sameScope(proof.state.scope, scope) {
		return ErrInvalidProof
	}
	if !sameContextPolicy(proof.state.policy, policy) {
		return ErrPolicyMismatch
	}
	current, _, err := b.capture(ctx, scope, policy)
	if err != nil {
		return err
	}
	return ValidateFreshness(policy, proof.state.snapshots, current, now)
}

// Change é um hint de dirty/resync. Não contém snapshot nem prova.
type Change struct {
	ProviderID string
	Fact       string
}

type factSubscription struct {
	scope Scope
	ch    chan Change
}

// Subscribe registra um canal bufferizado de capacidade 1. O cancel remove a
// assinatura, mas não fecha o canal; assim Notify nunca concorre com close.
func (b *FactBus) Subscribe(scope Scope) (<-chan Change, func(), error) {
	if b == nil {
		return nil, nil, ErrProviderUnavailable
	}
	if err := scope.validate(); err != nil {
		return nil, nil, err
	}
	subscription := &factSubscription{scope: cloneScopedScope(scope), ch: make(chan Change, 1)}
	b.mu.Lock()
	if b.subs == nil {
		b.subs = make(map[*factSubscription]struct{})
	}
	b.subs[subscription] = struct{}{}
	b.mu.Unlock()
	cancel := func() {
		b.mu.Lock()
		delete(b.subs, subscription)
		b.mu.Unlock()
	}
	return subscription.ch, cancel, nil
}

// Notify valida a origem registrada e o owner, publica apenas um hint
// não-bloqueante e nunca atualiza snapshot/prova. Hints drop/coalesce são
// esperados: Revalidate continua sendo síncrono contra a fonte.
func (b *FactBus) Notify(ctx context.Context, scope Scope, providerID, fact string, owned OwnedSnapshot) error {
	if b == nil {
		return ErrProviderUnavailable
	}
	if ctx == nil {
		return errors.New("contexto nil")
	}
	if err := scope.validate(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	b.mu.RLock()
	_, registered := b.providers[providerID]
	b.mu.RUnlock()
	if !registered || strings.TrimSpace(fact) == "" {
		return ErrProviderUnavailable
	}
	if err := validateOwnedSnapshot(owned, scope); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for subscription := range b.subs {
		if !sameScope(subscription.scope, scope) {
			continue
		}
		select {
		case subscription.ch <- Change{ProviderID: providerID, Fact: fact}:
		default:
		}
	}
	return nil
}

// ProviderCapturedAt devolve uma cópia dos timestamps confiáveis exigidos por
// max_age_ms/event_snapshot. Se um provider forneceu vários fatos, conserva o
// instante mais antigo. exact_version não aparece no mapa.
func (p FactProof) ProviderCapturedAt() map[string]time.Time {
	if !p.valid() {
		return nil
	}
	result := make(map[string]time.Time)
	for _, fact := range p.state.policy.Facts {
		if fact.Mode != commandcatalog.MaxAge && fact.Mode != commandcatalog.EventSnapshot {
			continue
		}
		snapshot, ok := p.state.snapshots[FactKey{Provider: fact.Provider, Fact: fact.Fact}]
		if !ok || snapshot.CapturedAt.IsZero() {
			continue
		}
		if current, exists := result[fact.Provider]; !exists || snapshot.CapturedAt.Before(current) {
			result[fact.Provider] = snapshot.CapturedAt
		}
	}
	return result
}

func cloneContextPolicy(policy commandcatalog.ContextPolicy) commandcatalog.ContextPolicy {
	policy.Facts = append([]commandcatalog.ContextFact(nil), policy.Facts...)
	return policy
}

func sameContextPolicy(left, right commandcatalog.ContextPolicy) bool {
	if left.None != right.None || len(left.Facts) != len(right.Facts) {
		return false
	}
	for i := range left.Facts {
		if left.Facts[i] != right.Facts[i] {
			return false
		}
	}
	return true
}

func cloneSnapshots(in Snapshots) Snapshots {
	if in == nil {
		return nil
	}
	out := make(Snapshots, len(in))
	for key, snapshot := range in {
		out[key] = snapshot
	}
	return out
}
