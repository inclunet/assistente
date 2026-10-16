package commandcontext

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"assistente/internal/commandcatalog"
)

// Provider fornece a versão de um fato contextual confiável. A implementação
// é responsável por autenticar o snapshot e deve ser segura para uso
// concorrente; o VersionService não impõe sincronização nem autenticação.
type Provider interface {
	Snapshot(context.Context, string) (Snapshot, error)
}

// VersionService mantém um registro imutável de providers confiáveis.
type VersionService struct {
	providers map[string]Provider
}

// NewVersionService valida e copia o registro fornecido pelo bootstrap.
func NewVersionService(providers map[string]Provider) (*VersionService, error) {
	registry := make(map[string]Provider, len(providers))
	for id, provider := range providers {
		if strings.TrimSpace(id) == "" {
			return nil, errors.New("provider contextual com ID vazio")
		}
		if isNilProvider(provider) {
			return nil, fmt.Errorf("provider contextual %q inválido", id)
		}
		registry[id] = provider
	}
	return &VersionService{providers: registry}, nil
}

// Capture coleta somente os fatos exigidos pela policy e calcula o fingerprint
// determinístico de seus pares provider/fato/versão. Nenhum timestamp é criado
// aqui: ele deve vir do provider confiável.
func (s *VersionService) Capture(ctx context.Context, policy commandcatalog.ContextPolicy) (Snapshots, string, error) {
	if ctx == nil {
		return nil, "", errors.New("contexto nil")
	}
	if s == nil {
		return nil, "", errors.New("serviço de versões nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if err := validatePolicy(policy); err != nil {
		return nil, "", err
	}
	if policy.None {
		return Snapshots{}, "", nil
	}

	captured := make(Snapshots, len(policy.Facts))
	for _, fact := range policy.Facts {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		provider, ok := s.providers[fact.Provider]
		if !ok {
			return nil, "", fmt.Errorf("provider %q ausente: %w", fact.Provider, ErrMissingSnapshot)
		}
		snapshot, err := provider.Snapshot(ctx, fact.Fact)
		if err != nil {
			return nil, "", fmt.Errorf("provider %q fato %q: %w", fact.Provider, fact.Fact, err)
		}
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		if strings.TrimSpace(snapshot.Version) == "" {
			return nil, "", fmt.Errorf("provider %q fato %q: %w", fact.Provider, fact.Fact, ErrVersionMismatch)
		}
		captured[FactKey{Provider: fact.Provider, Fact: fact.Fact}] = snapshot
	}

	return captured, fingerprint(captured), nil
}

// Revalidate coleta novamente os mesmos fatos e valida a policy sem alterar os
// timestamps dos snapshots capturados.
func (s *VersionService) Revalidate(ctx context.Context, policy commandcatalog.ContextPolicy, captured Snapshots, now time.Time) error {
	if ctx == nil {
		return errors.New("contexto nil")
	}
	if s == nil {
		return errors.New("serviço de versões nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validatePolicy(policy); err != nil {
		return err
	}
	if policy.None {
		return ValidateFreshness(policy, captured, nil, now)
	}
	for _, fact := range policy.Facts {
		key := FactKey{Provider: fact.Provider, Fact: fact.Fact}
		snapshot, ok := captured[key]
		if ok && strings.TrimSpace(snapshot.Version) == "" {
			return fmt.Errorf("snapshot capturado para provider %q fato %q: %w", fact.Provider, fact.Fact, ErrVersionMismatch)
		}
	}

	current, _, err := s.Capture(ctx, policy)
	if err != nil {
		return err
	}
	return ValidateFreshness(policy, captured, current, now)
}

type fingerprintEntry struct {
	provider string
	fact     string
	version  string
}

// fingerprint calcula o context_version dos pares capturados. Este fingerprint
// é específico do contexto e não equivale ao fingerprint de request RFC 8785.
func fingerprint(snapshots Snapshots) string {
	entries := make([]fingerprintEntry, 0, len(snapshots))
	for key, snapshot := range snapshots {
		entries = append(entries, fingerprintEntry{provider: key.Provider, fact: key.Fact, version: snapshot.Version})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].provider != entries[j].provider {
			return entries[i].provider < entries[j].provider
		}
		if entries[i].fact != entries[j].fact {
			return entries[i].fact < entries[j].fact
		}
		return entries[i].version < entries[j].version
	})

	h := sha256.New()
	writeLengthPrefixed(h, "assistente.commandcontext.context_version.v1")
	for _, entry := range entries {
		writeLengthPrefixed(h, entry.provider)
		writeLengthPrefixed(h, entry.fact)
		writeLengthPrefixed(h, entry.version)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func writeLengthPrefixed(h interface{ Write([]byte) (int, error) }, value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = h.Write(length[:])
	_, _ = h.Write([]byte(value))
}

func isNilProvider(provider Provider) bool {
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
