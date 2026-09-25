// Package commandcontext valida a atualidade dos fatos contextuais da D2.
// A validação é deliberadamente pura: autenticação e autorização pertencem ao
// serviço de execução, não a este helper ou ao catálogo estático.
package commandcontext

import (
	"errors"
	"math"
	"strings"
	"time"

	"assistente/internal/commandcatalog"
)

// FactKey identifica um fato de forma independente do formato de transporte.
type FactKey struct {
	Provider string
	Fact     string
}

// Snapshot é o estado confiável de um fato no instante em que foi observado.
// CapturedAt é usado somente pelas políticas temporais.
type Snapshot struct {
	Version    string
	CapturedAt time.Time
}

// Snapshots contém snapshots indexados por provider e fato.
type Snapshots map[FactKey]Snapshot

var (
	ErrInvalidPolicy      = errors.New("política de contexto inválida")
	ErrMissingSnapshot    = errors.New("snapshot contextual ausente")
	ErrVersionMismatch    = errors.New("versão contextual divergente")
	ErrInvalidTimestamp   = errors.New("timestamp contextual inválido")
	ErrSnapshotExpired    = errors.New("snapshot contextual expirado")
	ErrInvalidNow         = errors.New("instante de validação inválido")
	ErrTTLUnrepresentable = errors.New("TTL contextual não representável")
)

// ValidateFreshness verifica a policy contra snapshots capturados por
// providers/fatos confiáveis e contra o estado atual desses providers. A
// autenticidade é pré-condição externa: este helper não autentica snapshots.
func ValidateFreshness(policy commandcatalog.ContextPolicy, captured, current Snapshots, now time.Time) error {
	if now.IsZero() {
		return ErrInvalidNow
	}
	if err := validatePolicy(policy); err != nil {
		return err
	}
	if policy.None {
		return nil
	}

	for _, fact := range policy.Facts {
		key := FactKey{Provider: fact.Provider, Fact: fact.Fact}
		capturedSnapshot, ok := captured[key]
		if !ok {
			return ErrMissingSnapshot
		}
		currentSnapshot, ok := current[key]
		if !ok {
			return ErrMissingSnapshot
		}

		switch fact.Mode {
		case commandcatalog.ExactVersion:
			if strings.TrimSpace(capturedSnapshot.Version) == "" || strings.TrimSpace(currentSnapshot.Version) == "" {
				return ErrVersionMismatch
			}
			if capturedSnapshot.Version != currentSnapshot.Version {
				return ErrVersionMismatch
			}
		case commandcatalog.MaxAge, commandcatalog.EventSnapshot:
			if capturedSnapshot.CapturedAt.IsZero() || capturedSnapshot.CapturedAt.After(now) {
				return ErrInvalidTimestamp
			}
			// time.Duration is nanoseconds. Reject a TTL that cannot be
			// represented instead of overflowing its conversion. Add/Before
			// compares the exact nanosecond boundary, including a +1ns age.
			const maxMilliseconds = int64(math.MaxInt64) / int64(time.Millisecond)
			if fact.MaxAgeMS > maxMilliseconds {
				return ErrTTLUnrepresentable
			}
			ttl := time.Duration(fact.MaxAgeMS) * time.Millisecond
			if capturedSnapshot.CapturedAt.Add(ttl).Before(now) {
				return ErrSnapshotExpired
			}
		default:
			// validatePolicy makes this unreachable, but retaining a closed
			// default keeps the validator safe if the policy type evolves.
			return ErrInvalidPolicy
		}
	}
	return nil
}

func validatePolicy(policy commandcatalog.ContextPolicy) error {
	if policy.None {
		if len(policy.Facts) != 0 {
			return ErrInvalidPolicy
		}
		return nil
	}
	if len(policy.Facts) == 0 {
		return ErrInvalidPolicy
	}

	seen := make(map[FactKey]struct{}, len(policy.Facts))
	for _, fact := range policy.Facts {
		if strings.TrimSpace(fact.Provider) == "" || strings.TrimSpace(fact.Fact) == "" {
			return ErrInvalidPolicy
		}
		key := FactKey{Provider: fact.Provider, Fact: fact.Fact}
		if _, ok := seen[key]; ok {
			return ErrInvalidPolicy
		}
		seen[key] = struct{}{}
		switch fact.Mode {
		case commandcatalog.ExactVersion:
			if fact.MaxAgeMS != 0 {
				return ErrInvalidPolicy
			}
		case commandcatalog.MaxAge, commandcatalog.EventSnapshot:
			if fact.MaxAgeMS <= 0 {
				return ErrInvalidPolicy
			}
		default:
			return ErrInvalidPolicy
		}
	}
	return nil
}
