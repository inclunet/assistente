package commandcontext

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sync"

	"assistente/internal/commandsecurity"
	"github.com/google/uuid"
)

var (
	ErrGenerationOverflow = errors.New("geração contextual excedeu o limite")
	ErrInvalidGeneration  = errors.New("geração contextual inválida")
)

// GenerationSnapshot é a projeção exata para D2.1/D12. A configuração global
// é por usuário; a configuração de workspace é nil no escopo global e aponta
// para uma geração por usuário+workspace. ActiveLayers combina os dois
// contadores e não é um contador independente.
type GenerationSnapshot struct {
	GlobalConfigGeneration    string
	WorkspaceConfigGeneration *string
	ActiveLayersGeneration    string
}

type generationKey struct {
	user      string
	workspace string
	hasWS     bool
}

func makeGenerationKey(scope Scope) (generationKey, error) {
	if err := scope.validate(); err != nil {
		return generationKey{}, err
	}
	key := generationKey{user: scope.UserID}
	if scope.WorkspaceID != nil {
		key.workspace, key.hasWS = *scope.WorkspaceID, true
	}
	return key, nil
}

// GenerationStore é somente memória. O prefixo UUIDv7 impede que uma versão
// de uma execução anterior seja ressuscitada após restart. AuthContextID é
// validado no Scope, mas não é dimensão de configuração.
type GenerationStore struct {
	mu        sync.RWMutex
	gate      *commandsecurity.DispatchGate
	prefix    string
	global    map[string]uint64
	workspace map[generationKey]uint64
	layers    map[generationKey]uint64
}

func NewGenerationStore(gate *commandsecurity.DispatchGate) (*GenerationStore, error) {
	if gate == nil {
		return nil, ErrInvalidGeneration
	}
	prefix, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	return &GenerationStore{
		gate: gate, prefix: prefix.String(), global: make(map[string]uint64),
		workspace: make(map[generationKey]uint64), layers: make(map[generationKey]uint64),
	}, nil
}

func (s *GenerationStore) valid() bool {
	return s != nil && s.gate != nil && canonicalUUID7(s.prefix)
}

func (s *GenerationStore) currentGlobal(user string) uint64 {
	value := s.global[user]
	if value == 0 {
		value = 1
		s.global[user] = value
	}
	return value
}

func currentScopedCounter(values map[generationKey]uint64, key generationKey) uint64 {
	value := values[key]
	if value == 0 {
		value = 1
		values[key] = value
	}
	return value
}

func generationID(prefix string, value uint64) string { return fmt.Sprintf("%s:%d", prefix, value) }

func generationHash(globalID, localID string) string {
	h := sha256.New()
	write := func(value string) {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = h.Write(length[:])
		_, _ = h.Write([]byte(value))
	}
	write("assistente.commandcontext.active_layers.v1")
	write(globalID)
	write(localID)
	return hex.EncodeToString(h.Sum(nil))
}

func cloneGenerationPointer(value string) *string {
	copy := value
	return &copy
}

// Snapshot devolve uma projeção consistente. Duas sessões do mesmo usuário
// observam as mesmas gerações, e uma mutação global aparece em todo workspace
// daquele usuário na próxima leitura.
func (s *GenerationStore) Snapshot(scope Scope) (GenerationSnapshot, error) {
	if !s.valid() {
		return GenerationSnapshot{}, ErrInvalidGeneration
	}
	key, err := makeGenerationKey(scope)
	if err != nil {
		return GenerationSnapshot{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	global := generationID(s.prefix, s.currentGlobal(key.user))
	globalLayers := generationID(s.prefix, currentScopedCounter(s.layers, generationKey{user: key.user}))
	activeLocal := generationID(s.prefix, currentScopedCounter(s.layers, key))
	snapshot := GenerationSnapshot{GlobalConfigGeneration: global, ActiveLayersGeneration: generationHash(generationHash(global, globalLayers), activeLocal)}
	if key.hasWS {
		workspace := generationID(s.prefix, currentScopedCounter(s.workspace, key))
		snapshot.WorkspaceConfigGeneration = cloneGenerationPointer(workspace)
	}
	return snapshot, nil
}

type generationTarget uint8

const (
	targetGlobal generationTarget = iota
	targetWorkspace
	targetLayers
)

func (s *GenerationStore) bump(ctx context.Context, scope Scope, target generationTarget) error {
	if !s.valid() {
		return ErrInvalidGeneration
	}
	key, err := makeGenerationKey(scope)
	if err != nil {
		return err
	}
	return s.gate.WithMutation(ctx, func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if target != targetGlobal {
			values := s.workspace
			if target == targetLayers {
				values = s.layers
			}
			value := currentScopedCounter(values, key)
			if value == math.MaxUint64 {
				return ErrGenerationOverflow
			}
			values[key] = value + 1
			return nil
		}
		value := s.currentGlobal(key.user)
		if value == math.MaxUint64 {
			return ErrGenerationOverflow
		}
		s.global[key.user] = value + 1
		return nil
	})
}

func (s *GenerationStore) BumpGlobalConfig(ctx context.Context, scope Scope) error {
	return s.bump(ctx, scope, targetGlobal)
}

func (s *GenerationStore) BumpWorkspaceConfig(ctx context.Context, scope Scope) error {
	if scope.WorkspaceID == nil {
		return ErrInvalidScope
	}
	return s.bump(ctx, scope, targetWorkspace)
}

// BumpActiveLayers altera somente o contador local do escopo indicado. A
// geração publicada continua sendo o hash composto global+local.
func (s *GenerationStore) BumpActiveLayers(ctx context.Context, scope Scope) error {
	return s.bump(ctx, scope, targetLayers)
}
