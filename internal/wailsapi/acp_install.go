package wailsapi

import (
	"assistente/internal/acp"
	"assistente/internal/acpinstall"
	"assistente/internal/apidto"
	"assistente/internal/database"
	"assistente/internal/llm"
	"context"
	"errors"
	"maps"
	"sync"
	"time"
)

// ACPInstallHooks agrupa side effects do App que o bind não deve conhecer
// diretamente (AEP-0088): repontar provedores, recusar update com turno em voo
// e limpar versões antigas. Handshake, progresso e montagem do instalador
// permanecem no *App (helpers lowercase).
type ACPInstallHooks struct {
	// Installer devolve o instalador compartilhado com o catálogo (lazy no App).
	Installer func() *acpinstall.Installer
	// ProvidersFrom lista provedores LLM que sobem alguma das instalações.
	ProvidersFrom func(installations []acpinstall.Installation) []*llm.ProviderConfig
	// RefuseUpdateDuringTurn recusa atualização enquanto há turno em voo.
	RefuseUpdateDuringTurn func(usando []*llm.ProviderConfig) error
	// RepointProviders reescreve provedores da instalação antiga na nova.
	RepointProviders func(ctx context.Context, usando []*llm.ProviderConfig, installation acpinstall.Installation)
	// RemoveSupersededVersions apaga versões que ninguém mais sobe.
	RemoveSupersededVersions func(ctx context.Context, agentID, keep string, usando []*llm.ProviderConfig)
}

// ACPInstall é o bind Wails do domínio acp_install (AEP-0088).
// Auth só via WithUser — sem chamar o helper de auth do App no call site.
type ACPInstall struct {
	mu      sync.RWMutex
	session Session
	hooks   ACPInstallHooks
}

// NewACPInstall cria o bind vazio; AttachACPInstall preenche deps no startup.
func NewACPInstall() *ACPInstall {
	return &ACPInstall{}
}

// AttachACPInstall associa Session e hooks após o startup.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachACPInstall(api *ACPInstall, session Session, hooks ACPInstallHooks) {
	if api == nil {
		return
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	api.session = session
	api.hooks = hooks
}

func (api *ACPInstall) deps() (Session, ACPInstallHooks, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()
	if api.session == nil || api.hooks.Installer == nil {
		return nil, ACPInstallHooks{}, ErrACPInstallNotWired
	}
	return api.session, api.hooks, nil
}

func (api *ACPInstall) installer() (*acpinstall.Installer, ACPInstallHooks, error) {
	_, hooks, err := api.deps()
	if err != nil {
		return nil, ACPInstallHooks{}, err
	}
	inst := hooks.Installer()
	if inst == nil {
		return nil, ACPInstallHooks{}, ErrACPInstallNotWired
	}
	return inst, hooks, nil
}

// ACPAgentInstallPlan diz o que será instalado do agente pedido e se dá para
// instalar agora.
func (api *ACPInstall) ACPAgentInstallPlan(agentID string) (apidto.ACPInstallPlan, error) {
	session, _, err := api.deps()
	if err != nil {
		return emptyInstallPlan(), err
	}
	return WithUser(session, func(ctx context.Context) (apidto.ACPInstallPlan, error) {
		return api.acpInstallPlan(ctx, agentID)
	})
}

func (api *ACPInstall) acpInstallPlan(ctx context.Context, agentID string) (apidto.ACPInstallPlan, error) {
	installer, _, err := api.installer()
	if err != nil {
		return emptyInstallPlan(), err
	}
	plan, err := installer.Plan(ctx, agentID)
	if err != nil {
		unavailable := emptyInstallPlan()
		unavailable.AgentID = agentID
		unavailable.Runtime = runtimeStatusDTO(acp.FindNodeRuntime())
		unavailable.Reason = acp.SanitizeLabel(err.Error())
		return unavailable, nil
	}
	return installPlanDTO(plan, installer.Installing(agentID)), nil
}

// InstallACPAgent instala o agente do catálogo e só volta com sucesso depois de
// o comando resolvido responder `initialize` (AEP-0086 D8).
func (api *ACPInstall) InstallACPAgent(agentID string, confirmed apidto.ACPInstallConfirmation) (apidto.ACPInstallation, error) {
	session, _, err := api.deps()
	if err != nil {
		return apidto.ACPInstallation{}, err
	}
	return WithUser(session, func(ctx context.Context) (apidto.ACPInstallation, error) {
		installer, _, err := api.installer()
		if err != nil {
			return apidto.ACPInstallation{}, err
		}
		installation, err := installer.Install(ctx, agentID, confirmedFrom(confirmed))
		if err != nil {
			return apidto.ACPInstallation{}, err
		}
		return installationDTO(installation), nil
	})
}

// UpdateACPAgent troca a instalação deste agente pela versão que o catálogo
// publica (AEP-0086 D10). Side effects de provedores ficam nos hooks do App.
func (api *ACPInstall) UpdateACPAgent(agentID string, confirmed apidto.ACPInstallConfirmation) (apidto.ACPInstallation, error) {
	session, hooks, err := api.deps()
	if err != nil {
		return apidto.ACPInstallation{}, err
	}
	return WithUser(session, func(ctx context.Context) (apidto.ACPInstallation, error) {
		installer, _, err := api.installer()
		if err != nil {
			return apidto.ACPInstallation{}, err
		}
		var pointing []*llm.ProviderConfig
		if hooks.ProvidersFrom != nil {
			pointing = hooks.ProvidersFrom(installer.Installations(agentID))
		}
		if hooks.RefuseUpdateDuringTurn != nil {
			if err := hooks.RefuseUpdateDuringTurn(pointing); err != nil {
				return apidto.ACPInstallation{}, err
			}
		}

		updated, err := installer.Update(ctx, agentID, confirmedFrom(confirmed))
		if err != nil {
			return apidto.ACPInstallation{}, err
		}

		if hooks.RepointProviders != nil {
			hooks.RepointProviders(ctx, pointing, updated.Installed)
		}
		if hooks.RemoveSupersededVersions != nil {
			hooks.RemoveSupersededVersions(ctx, agentID, updated.Installed.Version, pointing)
		}
		return installationDTO(updated.Installed), nil
	})
}

// CancelACPAgentInstall interrompe a instalação em voo (AEP-0086 D13).
func (api *ACPInstall) CancelACPAgentInstall(agentID string) error {
	session, _, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		_ = ctx
		installer, _, err := api.installer()
		if err != nil {
			return struct{}{}, err
		}
		installer.Cancel(agentID)
		return struct{}{}, nil
	})
	return err
}

// CanRemoveACPAgent diz se o agente foi instalado pelo app e ficou sem nenhum
// provedor que o use.
func (api *ACPInstall) CanRemoveACPAgent(agentID string) (bool, error) {
	session, _, err := api.deps()
	if err != nil {
		return false, err
	}
	return WithUser(session, func(ctx context.Context) (bool, error) {
		inUse, err := database.HasAnyLLMProviderForACPAgent(ctx, agentID)
		if err != nil || inUse {
			return false, err
		}
		installer, _, err := api.installer()
		if err != nil {
			return false, err
		}
		for _, installation := range installer.List() {
			if installation.AgentID == agentID {
				return true, nil
			}
		}
		return false, nil
	})
}

// RemoveACPAgent apaga o diretório de um agente sem provedores (AEP-0086 D5).
func (api *ACPInstall) RemoveACPAgent(agentID string) error {
	session, _, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		inUse, err := database.HasAnyLLMProviderForACPAgent(ctx, agentID)
		if err != nil {
			return struct{}{}, err
		}
		if inUse {
			return struct{}{}, errors.New("o agente ainda é usado por outro provedor")
		}
		installer, _, err := api.installer()
		if err != nil {
			return struct{}{}, err
		}
		return struct{}{}, installer.Remove(ctx, agentID)
	})
	return err
}

// ListInstalledACPAgents lista o que o app instalou.
func (api *ACPInstall) ListInstalledACPAgents() ([]apidto.ACPInstallation, error) {
	session, _, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]apidto.ACPInstallation, error) {
		_ = ctx
		installer, _, err := api.installer()
		if err != nil {
			return nil, err
		}
		installations := installer.List()
		out := make([]apidto.ACPInstallation, 0, len(installations))
		for _, installation := range installations {
			out = append(out, installationDTO(installation))
		}
		return out, nil
	})
}

func confirmedFrom(confirmed apidto.ACPInstallConfirmation) acpinstall.Confirmed {
	return acpinstall.Confirmed{
		Distribution:     confirmed.Distribution,
		Origin:           confirmed.Origin,
		SHA256:           confirmed.SHA256,
		AcceptUnverified: confirmed.AcceptUnverified,
	}
}

// emptyInstallPlan é o plano que não oferece nada, com as listas que o DTO
// promete sempre presentes.
func emptyInstallPlan() apidto.ACPInstallPlan {
	return apidto.ACPInstallPlan{RunArgs: []string{}}
}

func installPlanDTO(plan acpinstall.Plan, installing bool) apidto.ACPInstallPlan {
	dto := apidto.ACPInstallPlan{
		AgentID:        plan.AgentID,
		Name:           plan.Name,
		Version:        plan.Version,
		Distribution:   plan.Distribution,
		Origin:         plan.Origin,
		Bytes:          plan.Bytes,
		Target:         plan.Target,
		SHA256:         plan.SHA256,
		Unverified:     plan.Unverified,
		Dir:            plan.Dir,
		InstallCommand: plan.InstallCommand,
		RunArgs:        plan.RunArgs,
		Runtime: apidto.ACPRuntimeStatus{
			Name:     plan.Runtime.Name,
			Required: plan.Runtime.Required,
			Found:    plan.Runtime.Found,
			Path:     plan.Runtime.Path,
			Version:  plan.Runtime.Version,
			Searched: plan.Runtime.Searched,
		},
		CanInstall:   plan.CanInstall,
		Reason:       plan.Reason,
		Update:       plan.Update,
		CanUpdate:    plan.CanUpdate,
		UpdateReason: plan.UpdateReason,
		Installing:   installing,
	}
	if dto.RunArgs == nil {
		dto.RunArgs = []string{}
	}
	if plan.Installed != nil {
		installed := installationDTO(*plan.Installed)
		dto.Installed = &installed
	}
	return dto
}

func installationDTO(installation acpinstall.Installation) apidto.ACPInstallation {
	dto := apidto.ACPInstallation{
		AgentID:      installation.AgentID,
		Name:         installation.Name,
		Version:      installation.Version,
		Distribution: installation.Distribution,
		Target:       installation.Target,
		SHA256:       installation.SHA256,
		SHA256Origin: installation.SHA256Origin,
		Command:      installation.Command,
		Args:         installation.Args,
		Env:          maps.Clone(installation.Env),
		Dir:          installation.Dir,
		DiskBytes:    installation.DiskBytes,
	}
	if dto.Args == nil {
		dto.Args = []string{}
	}
	if !installation.InstalledAt.IsZero() {
		dto.InstalledAt = installation.InstalledAt.UTC().Format(time.RFC3339)
	}
	return dto
}

func runtimeStatusDTO(runtime acp.NodeRuntime) apidto.ACPRuntimeStatus {
	return apidto.ACPRuntimeStatus{
		Name:     acpinstall.RuntimeNode,
		Found:    runtime.Found,
		Path:     runtime.Node,
		Version:  runtime.Version,
		Searched: runtime.Searched,
	}
}
