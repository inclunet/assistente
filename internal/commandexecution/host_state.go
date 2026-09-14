package commandexecution

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
	"assistente/internal/commandsecurity"
	"github.com/google/uuid"
)

var (
	ErrInvalidHostState       = errors.New("estado do host inválido")
	ErrHostStateDisabled      = errors.New("estado do host desabilitado")
	ErrInvalidHostUser        = errors.New("usuário do host inválido")
	ErrInvalidHostPrincipal   = errors.New("principal local inválido")
	ErrHostUserNotPublished   = errors.New("configuração do usuário não publicada")
	ErrInvalidHostLayers      = errors.New("camadas ativas inválidas")
	ErrHostGenerationOverflow = errors.New("contador de geração do host esgotado")
)

// HostState é a projeção autoritativa, em memória, do estado que o executor
// precisa revalidar. Ele não autentica, autoriza nem consulta cofre, banco ou
// sistema operacional. Essas responsabilidades pertencem ao bootstrap e aos
// adapters confiáveis; os setters de lock apenas recebem o fato já observado.
//
// O mutex é deliberadamente independente do DispatchGate. Snapshot é chamado
// dentro de Capture/Admit, cujos callbacks já detêm o gate, e portanto nunca
// pode tentar readquiri-lo.
type HostState struct {
	mu sync.RWMutex

	epochs          *commandsecurity.EpochService
	registryVersion string
	startup         string
	counter         uint64
	disabled        bool

	vaultUnlocked bool
	osKnown       bool
	osLocked      bool
	users         map[string]hostUserState
}

type hostUserState struct {
	readySession        string
	configuration       *commandbindings.Configuration
	activeLayers        []string
	globalConfig        string
	activeLayersVersion string
}

// NewHostState cria um estado seguro: o cofre começa fechado e o estado da
// sessão do sistema operacional começa desconhecido. registryVersion é fixada
// nesta instância; mudança de catálogo exige uma nova instância e a invalidação
// coordenada correspondente no bootstrap.
func NewHostState(epochs *commandsecurity.EpochService, registryVersion string) (*HostState, error) {
	if epochs == nil || !validHostVersion(registryVersion) {
		return nil, ErrInvalidHostState
	}
	startup, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("gerar identidade de startup do host: %w", err)
	}
	return &HostState{
		epochs:          epochs,
		registryVersion: registryVersion,
		startup:         startup.String(),
		osLocked:        true,
		users:           make(map[string]hostUserState),
	}, nil
}

// Epochs devolve exatamente a dependência recebida no bootstrap. A identidade
// compartilhada coordena este estado com o executor; não cria nem substitui um
// EpochService.
func (s *HostState) Epochs() *commandsecurity.EpochService {
	if s == nil {
		return nil
	}
	return s.epochs
}

// Snapshot implementa Config.Snapshot. Unlocked também exige mapa reconstruído
// para a sessão exata; publicar apenas configuração não ativa bindings.
// É uma leitura curta, sem autenticação
// ou autorização, e espera um principal já derivado pela borda confiável.
// Nenhuma chamada ao EpochService ocorre aqui: este método é seguro dentro de
// callbacks de Capture/Admit.
func (s *HostState) Snapshot(ctx context.Context, principal auth.LocalSessionPrincipal) (Versions, error) {
	if s == nil || ctx == nil {
		return Versions{}, ErrInvalidHostState
	}
	if err := ctx.Err(); err != nil {
		return Versions{}, err
	}
	if !validHostID(principal.UserID) || !validHostID(principal.SessionID) {
		return Versions{}, ErrInvalidHostPrincipal
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return Versions{}, err
	}
	if s.disabled {
		return Versions{}, ErrHostStateDisabled
	}
	user, ok := s.users[principal.UserID]
	if !ok || user.configuration == nil {
		return Versions{}, ErrHostUserNotPublished
	}
	return Versions{
		Registry:     s.registryVersion,
		GlobalConfig: user.globalConfig,
		ActiveLayers: user.activeLayersVersion,
		Unlocked:     s.vaultUnlocked && s.osKnown && !s.osLocked && user.readySession == principal.SessionID,
	}, nil
}

// PublishUserConfiguration publica um ponteiro Configuration já construído.
// Configuration é um snapshot imutável: seus índices são privados e suas
// operações retornam projeções detached. A publicação e a geração global do
// usuário são coordenadas pelo EpochService; somente as execuções desse
// usuário são canceladas antes do callback.
// Esta publicação é inerte para despacho: a sessão utilizável só é associada
// por RebuildUserConfiguration após autenticação e reconstrução completas.
func (s *HostState) PublishUserConfiguration(ctx context.Context, userID string, configuration *commandbindings.Configuration) error {
	if s == nil {
		return ErrInvalidHostState
	}
	if !validHostID(userID) {
		return ErrInvalidHostUser
	}
	if configuration == nil {
		return ErrInvalidHostState
	}
	return s.epochs.MutateUserConfiguration(ctx, userID, func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.disabled {
			return ErrHostStateDisabled
		}
		user := s.users[userID]
		needActiveGeneration := user.configuration == nil
		generations, err := s.reserveGenerationsLocked(boolToCount(needActiveGeneration))
		if err != nil {
			return err
		}
		user.configuration = configuration
		user.readySession = ""
		user.globalConfig = generations[0]
		if needActiveGeneration {
			user.activeLayersVersion = generations[1]
		}
		s.users[userID] = user
		return nil
	})
}

// SetActiveLayers troca somente a lista de camadas efetivamente ativas do
// usuário. Os IDs são opacos: o host não inventa nomes nem resolve aliases.
// A lista é copiada na entrada e na saída para impedir aliasing com o chamador.
func (s *HostState) SetActiveLayers(ctx context.Context, userID string, layers []string) error {
	if s == nil {
		return ErrInvalidHostState
	}
	if !validHostID(userID) {
		return ErrInvalidHostUser
	}
	if err := validateHostLayers(layers); err != nil {
		return err
	}
	return s.epochs.MutateUserConfiguration(ctx, userID, func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.disabled {
			return ErrHostStateDisabled
		}
		user, ok := s.users[userID]
		if !ok || user.configuration == nil {
			return ErrHostUserNotPublished
		}
		if slicesEqual(user.activeLayers, layers) {
			return nil
		}
		generation, err := s.reserveGenerationsLocked(1)
		if err != nil {
			return err
		}
		user.activeLayers = cloneStrings(layers)
		user.activeLayersVersion = generation[0]
		s.users[userID] = user
		return nil
	})
}

// ForgetUserConfiguration remove o snapshot completo do usuário sob a mesma
// coordenação de configuração. É idempotente para uma conta não publicada;
// republicação posterior recebe novas gerações do contador desta instância.
func (s *HostState) ForgetUserConfiguration(ctx context.Context, userID string) error {
	if s == nil {
		return ErrInvalidHostState
	}
	if !validHostID(userID) {
		return ErrInvalidHostUser
	}
	return s.epochs.MutateUserConfiguration(ctx, userID, func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.disabled {
			return ErrHostStateDisabled
		}
		delete(s.users, userID)
		_, err := s.reserveGenerationsLocked(1)
		return err
	})
}

// UserConfiguration devolve o snapshot publicado e uma cópia detached das
// camadas para um host confiável. Não é uma operação de autenticação ou
// autorização e não faz consultas externas.
func (s *HostState) UserConfiguration(ctx context.Context, userID string) (*commandbindings.Configuration, []string, error) {
	if s == nil || ctx == nil {
		return nil, nil, ErrInvalidHostState
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if !validHostID(userID) {
		return nil, nil, ErrInvalidHostUser
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if s.disabled {
		return nil, nil, ErrHostStateDisabled
	}
	user, ok := s.users[userID]
	if !ok || user.configuration == nil {
		return nil, nil, ErrHostUserNotPublished
	}
	return user.configuration, cloneStrings(user.activeLayers), nil
}

// SetVaultUnlocked atualiza somente a observação do cofre sob a invalidação
// global de segurança. O host não abre, fecha ou consulta o cofre.
func (s *HostState) SetVaultUnlocked(ctx context.Context, unlocked bool) error {
	if s == nil {
		return ErrInvalidHostState
	}
	return s.epochs.MutateSecurity(ctx, func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.disabled {
			return ErrHostStateDisabled
		}
		s.vaultUnlocked = unlocked
		return nil
	})
}

// SetOSSessionState registra uma observação já produzida pelo adapter do SO.
// Estado desconhecido permanece fechado independentemente do valor de locked.
// Toda observação descarta mapas: até o primeiro unlock exige reconstrução.
func (s *HostState) SetOSSessionState(ctx context.Context, known bool, locked bool) error {
	if s == nil {
		return ErrInvalidHostState
	}
	return s.epochs.MutateSecurity(ctx, func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.disabled {
			return ErrHostStateDisabled
		}
		s.osKnown = known
		s.osLocked = locked
		// Nem unlock nem uma observação inicial podem reutilizar mapas que
		// antecedem o fato do SO. Reconstrução autenticada é obrigatória.
		clear(s.users)
		_, err := s.reserveGenerationsLocked(1)
		return err
	})
}

func (s *HostState) reserveGenerationsLocked(count uint64) ([]string, error) {
	if s.disabled {
		return nil, ErrHostStateDisabled
	}
	if count == 0 {
		return nil, ErrInvalidHostState
	}
	if s.counter > math.MaxUint64-count {
		s.disabled = true
		return nil, ErrHostGenerationOverflow
	}
	result := make([]string, count)
	for i := range result {
		s.counter++
		result[i] = fmt.Sprintf("%s:%d", s.startup, s.counter)
	}
	return result, nil
}

func validHostVersion(value string) bool {
	return value != "" && strings.TrimSpace(value) == value
}

func validHostID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id.Version() == 7 && id.Variant() == uuid.RFC4122 && id.String() == value
}

func validateHostLayers(layers []string) error {
	seen := make(map[string]struct{}, len(layers))
	for _, layer := range layers {
		if strings.TrimSpace(layer) == "" {
			return ErrInvalidHostLayers
		}
		if _, exists := seen[layer]; exists {
			return ErrInvalidHostLayers
		}
		seen[layer] = struct{}{}
	}
	return nil
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}

func slicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func boolToCount(two bool) uint64 {
	if two {
		return 2
	}
	return 1
}
