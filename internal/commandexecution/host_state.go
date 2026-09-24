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
	ErrInvalidHostState         = errors.New("estado do host inválido")
	ErrHostStateDisabled        = errors.New("estado do host desabilitado")
	ErrInvalidHostUser          = errors.New("usuário do host inválido")
	ErrInvalidHostPrincipal     = errors.New("principal local inválido")
	ErrHostUserNotPublished     = errors.New("configuração do usuário não publicada")
	ErrInvalidHostLayers        = errors.New("camadas ativas inválidas")
	ErrHostGenerationOverflow   = errors.New("contador de geração do host esgotado")
	ErrJobProjectionBaseChanged = errors.New("base persistida mudou durante projeção de claims de job")
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
	projectionGuard     func(context.Context) error
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
	if err := ctx.Err(); err != nil {
		s.mu.RUnlock()
		return Versions{}, err
	}
	if s.disabled {
		s.mu.RUnlock()
		return Versions{}, ErrHostStateDisabled
	}
	user, ok := s.users[principal.UserID]
	if !ok || user.configuration == nil {
		s.mu.RUnlock()
		return Versions{}, ErrHostUserNotPublished
	}
	if user.projectionGuard == nil {
		result := Versions{
			Registry:     s.registryVersion,
			GlobalConfig: user.globalConfig,
			ActiveLayers: user.activeLayersVersion,
			Unlocked:     s.vaultUnlocked && s.osKnown && !s.osLocked && user.readySession == principal.SessionID,
		}
		s.mu.RUnlock()
		return result, nil
	}
	counter, configuration, session, guard := s.counter, user.configuration, user.readySession, user.projectionGuard
	s.mu.RUnlock()
	if err := guard(ctx); err != nil {
		return Versions{}, err
	}
	if err := ctx.Err(); err != nil {
		return Versions{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.disabled {
		return Versions{}, ErrHostStateDisabled
	}
	current, ok := s.users[principal.UserID]
	if !ok || current.configuration == nil {
		return Versions{}, ErrHostUserNotPublished
	}
	if s.counter != counter || current.configuration != configuration || current.readySession != session {
		return Versions{}, ErrStale
	}
	return Versions{
		Registry:     s.registryVersion,
		GlobalConfig: current.globalConfig,
		ActiveLayers: current.activeLayersVersion,
		Unlocked:     s.vaultUnlocked && s.osKnown && !s.osLocked && current.readySession == principal.SessionID,
	}, nil
}

// WithPublishedVersions cerca um handoff local com a versão publicada exata.
// Não consulta projectionGuard: o caller deve fazer Snapshot completo antes
// de entrar no gate e revalidar seus epochs sob esse gate. fn não pode fazer
// I/O nem reentrar no HostState; somente o claim efêmero pertence a este lock.
func (s *HostState) WithPublishedVersions(ctx context.Context, principal auth.LocalSessionPrincipal, expected Versions, fn func() error) error {
	if s == nil || ctx == nil || fn == nil {
		return ErrInvalidHostState
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	user, ok := s.users[principal.UserID]
	if s.disabled || !ok || user.configuration == nil {
		return ErrHostUserNotPublished
	}
	actual := Versions{Registry: s.registryVersion, GlobalConfig: user.globalConfig, ActiveLayers: user.activeLayersVersion,
		Unlocked: s.vaultUnlocked && s.osKnown && !s.osLocked && user.readySession == principal.SessionID}
	if !actual.Unlocked || actual != expected {
		return ErrStale
	}
	return fn()
}

// InteractiveSessionReady permite apresentar novamente uma decisão já visível,
// inclusive sobre desbloqueio do cofre. Não autoriza comandos: esses continuam
// exigindo SourceSecurityReady, principal e todas as demais revalidações.
func (s *HostState) InteractiveSessionReady(ctx context.Context) (bool, error) {
	if s == nil || ctx == nil {
		return false, ErrInvalidHostState
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if s.disabled {
		return false, ErrHostStateDisabled
	}
	return s.osKnown && !s.osLocked, nil
}

// SourceSecurityReady verifica somente o estado de segurança local necessário
// para uma fonte autenticada: host ativo, cofre desbloqueado e sessão do SO
// conhecida/desbloqueada. Não exige configuração, sessão publicada, versões
// ou projectionGuard; uma recomposição/restore pode apagar users sem tornar a
// fonte de segurança indisponível.
func (s *HostState) SourceSecurityReady(ctx context.Context) (bool, error) {
	if s == nil || ctx == nil {
		return false, ErrInvalidHostState
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if s.disabled {
		return false, ErrHostStateDisabled
	}
	return s.vaultUnlocked && s.osKnown && !s.osLocked, nil
}

// ResolutionSnapshot captura a configuração, as camadas ativas e as versões
// que pertencem à mesma projeção pronta do usuário. A leitura é inteiramente
// local; o guard roda fora do mutex e o counter é conferido novamente antes
// do retorno. Não adquire o DispatchGate nem chama o EpochService. Uma
// projeção só é pronta para resolução quando a sessão
// publicada é exatamente a sessão do principal e cofre/SO estão desbloqueados.
func (s *HostState) ResolutionSnapshot(ctx context.Context, principal auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, Versions, error) {
	if s == nil || ctx == nil {
		return nil, nil, Versions{}, ErrInvalidHostState
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, Versions{}, err
	}
	if !validHostID(principal.UserID) || !validHostID(principal.SessionID) {
		return nil, nil, Versions{}, ErrInvalidHostPrincipal
	}

	s.mu.RLock()
	if err := ctx.Err(); err != nil {
		s.mu.RUnlock()
		return nil, nil, Versions{}, err
	}
	if s.disabled {
		s.mu.RUnlock()
		return nil, nil, Versions{}, ErrHostStateDisabled
	}
	user, ok := s.users[principal.UserID]
	if !ok || user.configuration == nil || user.readySession != principal.SessionID || !s.vaultUnlocked || !s.osKnown || s.osLocked {
		s.mu.RUnlock()
		return nil, nil, Versions{}, ErrHostUserNotPublished
	}
	if user.projectionGuard == nil {
		configuration, layers, versions := user.configuration, cloneStrings(user.activeLayers), Versions{
			Registry:     s.registryVersion,
			GlobalConfig: user.globalConfig,
			ActiveLayers: user.activeLayersVersion,
			Unlocked:     true,
		}
		s.mu.RUnlock()
		return configuration, layers, versions, nil
	}
	counter, configuration, session, guard := s.counter, user.configuration, user.readySession, user.projectionGuard
	s.mu.RUnlock()
	if err := guard(ctx); err != nil {
		return nil, nil, Versions{}, err
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, Versions{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.disabled {
		return nil, nil, Versions{}, ErrHostStateDisabled
	}
	current, ok := s.users[principal.UserID]
	if !ok || current.configuration == nil || current.readySession != principal.SessionID || !s.vaultUnlocked || !s.osKnown || s.osLocked {
		return nil, nil, Versions{}, ErrHostUserNotPublished
	}
	if s.counter != counter || current.configuration != configuration || current.readySession != session {
		return nil, nil, Versions{}, ErrStale
	}
	return current.configuration, cloneStrings(current.activeLayers), Versions{
		Registry:     s.registryVersion,
		GlobalConfig: current.globalConfig,
		ActiveLayers: current.activeLayersVersion,
		Unlocked:     true,
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
		user.projectionGuard = nil
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

// SuspendUserConfiguration invalida a projeção volátil de um usuário sem
// adquirir o DispatchGate. É uma porta interna para callbacks que já estão
// dentro do gate exclusivo (por exemplo, BeforeCommit de uma mutação SQL).
// Não habilita nada, não consulta fontes externas e não tenta reentrar no
// EpochService; a reconstrução autenticada precisa publicar um novo snapshot
// depois que a operação terminar.
func (s *HostState) SuspendUserConfiguration(ctx context.Context, userID string) error {
	if s == nil || ctx == nil {
		return ErrInvalidHostState
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validHostID(userID) {
		return ErrInvalidHostUser
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.disabled {
		return ErrHostStateDisabled
	}
	if _, ok := s.users[userID]; !ok {
		return ErrHostUserNotPublished
	}
	delete(s.users, userID)
	return ctx.Err()
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
