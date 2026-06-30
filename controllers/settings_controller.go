package controllers

import (
	"assistente/internal/logging"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"assistente/internal/config"
	"assistente/internal/core/ports"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/profiles"
	"assistente/internal/providers"
	"assistente/internal/skills"
)

// SettingsControllerConfig agrupa as dependências do SettingsController.
type SettingsControllerConfig struct {
	CredMgr     *credentials.Manager
	ProfileMgr  *profiles.Manager
	SkillMgr    *skills.Manager
	Emitter     ports.Emitter
	ProviderSvc *providers.Service
	// Callbacks cross-domain
	RestartChannel func(channelName string) error
	GetModels      func() ([]string, error)
}

// SettingsController é o adapter primário (Inbound) para operações de configurações globais e reset.
type SettingsController struct {
	credMgr        *credentials.Manager
	profileMgr     *profiles.Manager
	skillMgr       *skills.Manager
	emitter        ports.Emitter
	providerSvc    *providers.Service
	restartChannel func(string) error
	getModels      func() ([]string, error)
}

// NewSettingsController cria um SettingsController com suas dependências.
func NewSettingsController(cfg SettingsControllerConfig) *SettingsController {
	return &SettingsController{
		credMgr:        cfg.CredMgr,
		profileMgr:     cfg.ProfileMgr,
		skillMgr:       cfg.SkillMgr,
		emitter:        cfg.Emitter,
		providerSvc:    cfg.ProviderSvc,
		restartChannel: cfg.RestartChannel,
		getModels:      cfg.GetModels,
	}
}

// GetMaintenanceSettings retorna a política de retenção/compactação do banco
// (AEP-0074). Em caso de falha ao ler o config.json, devolve os defaults sem
// erro — coerente com o uso em background (retenção/compactação usam defaults),
// permitindo que a UI edite e salve a política (recriando o arquivo).
func (c *SettingsController) GetMaintenanceSettings() (config.MaintenanceSettings, error) {
	settings, err := config.GetMaintenance()
	if err != nil {
		logging.Errorf(context.Background(), "controllers.settings-controller", "[Settings] falha ao ler manutenção do config.json; usando defaults: %v", err)
		return config.DefaultMaintenanceSettings(), nil
	}
	return settings, nil
}

// SaveMaintenanceSettings persiste a política de manutenção no config.json.
func (c *SettingsController) SaveMaintenanceSettings(settings config.MaintenanceSettings) error {
	return config.SaveMaintenance(settings)
}

// GetDatabaseStats retorna o estado físico atual do banco (tamanho, freelist,
// modo de auto_vacuum) para exibir na tela de configurações.
func (c *SettingsController) GetDatabaseStats(ctx context.Context) (database.DatabaseStats, error) {
	return database.DatabaseStatsSnapshot(ctx)
}

// RunDatabaseMaintenance dispara a compactação física do banco sob demanda
// ("limpar agora"). force=true ignora o limiar de freelist. O limiar padrão vem
// do config.json (AEP-0074).
func (c *SettingsController) RunDatabaseMaintenance(ctx context.Context, force bool) (database.CompactionResult, error) {
	maint, err := config.GetMaintenance()
	if err != nil {
		maint = config.DefaultMaintenanceSettings()
	}
	return database.Compact(ctx, force, maint.VacuumMinFreeBytes)
}

// SendMessageSync envia uma mensagem sem streaming (para acessibilidade e testes).
func (c *SettingsController) SendMessageSync(ctx context.Context, messages []llm.Message, params llm.ChatParams) (string, error) {
	if c.profileMgr == nil {
		return "", fmt.Errorf("nenhum provedor LLM configurado no perfil ativo")
	}
	activeProfile, _ := c.profileMgr.GetActive()
	if c.providerSvc != nil {
		activeProfile = c.providerSvc.ResolveProfileDefaults(ctx, activeProfile)
	}
	if activeProfile == nil || activeProfile.Chat.LLMProvider == "" {
		return "", fmt.Errorf("nenhum provedor LLM configurado no perfil ativo")
	}
	if c.providerSvc == nil {
		return "", fmt.Errorf("provider service not initialized")
	}
	cp, err := c.providerSvc.GetChatProvider(ctx, activeProfile.Chat.LLMProvider)
	if err != nil {
		return "", err
	}
	return cp.SendChat(ctx, messages, params)
}

func (c *SettingsController) TestConnection() (bool, error) {
	if c.getModels == nil {
		return false, fmt.Errorf("getModels não configurado")
	}
	models, err := c.getModels()
	if err != nil {
		return false, err
	}
	if len(models) > 0 {
		return true, nil
	}
	return false, fmt.Errorf("nenhum modelo encontrado")
}

func (c *SettingsController) TestConnectionWithModels() ([]string, error) {
	if c.getModels == nil {
		return nil, fmt.Errorf("getModels não configurado")
	}
	models, err := c.getModels()
	if err != nil {
		return nil, fmt.Errorf("erro ao conectar com a API: %v", err)
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("nenhum modelo encontrado na API")
	}
	return models, nil
}

func (c *SettingsController) ResetConfig() error {
	configPath, err := config.GetConfigPath()
	if err != nil {
		return fmt.Errorf("erro ao obter caminho da configuração: %v", err)
	}
	if _, err := os.Stat(configPath); err == nil {
		if err := os.Remove(configPath); err != nil {
			return fmt.Errorf("erro ao remover arquivo de configuração: %v", err)
		}
	}
	return nil
}

// ClearAllCredentials apaga todas as credenciais visíveis ao usuário
// do contexto, iterando pattern por pattern (ListVisible já filtra
// instance secrets e cross-user). Exige `userID` no `ctx`.
func (c *SettingsController) ClearAllCredentials(ctx context.Context) error {
	if c.credMgr == nil {
		return fmt.Errorf("gerenciador de credenciais não disponível")
	}
	if _, err := database.RequireUserID(ctx); err != nil {
		return fmt.Errorf("ClearAllCredentials requer usuário autenticado: %w", err)
	}

	creds, err := c.credMgr.ListVisibleCredentialsWithContext(ctx)
	if err != nil {
		return fmt.Errorf("erro ao listar credenciais: %w", err)
	}
	deleted := 0
	for _, cred := range creds {
		if cred.Pattern == "" {
			continue
		}
		if err := c.credMgr.DeletePattern(ctx, cred.Pattern); err != nil {
			return fmt.Errorf("erro ao apagar credencial %q: %w", cred.Pattern, err)
		}
		deleted++
	}
	logging.Infof(ctx, "controllers.settings-controller", "[ClearAllCredentials] %d credenciais apagadas (escopo do usuário autenticado)", deleted)
	c.emitter.Emit("credentials:cleared", nil)
	return nil
}

func (c *SettingsController) ClearAllProfiles() error {
	if c.profileMgr == nil {
		return fmt.Errorf("gerenciador de perfis não disponível")
	}
	list, err := c.profileMgr.List()
	if err != nil {
		return fmt.Errorf("erro ao listar perfis: %v", err)
	}
	for _, p := range list {
		if err := c.profileMgr.Delete(p.Slug); err != nil {
			logging.Errorf(context.Background(), "controllers.settings-controller", "[ClearAllProfiles] Erro ao deletar perfil %s: %v", p.Slug, err)
		}
	}
	logging.Println(context.Background(), "controllers.settings-controller", "[ClearAllProfiles] Perfis apagados")
	c.emitter.Emit("profiles:cleared", nil)
	return nil
}

func (c *SettingsController) ClearAllSkills() error {
	if c.skillMgr == nil {
		return fmt.Errorf("gerenciador de skills não disponível")
	}
	list, err := c.skillMgr.List()
	if err != nil {
		return fmt.Errorf("erro ao listar skills: %v", err)
	}
	for _, s := range list {
		if err := c.skillMgr.Delete(s.Slug); err != nil {
			logging.Errorf(context.Background(), "controllers.settings-controller", "[ClearAllSkills] Erro ao deletar skill %s: %v", s.Slug, err)
		}
	}
	logging.Println(context.Background(), "controllers.settings-controller", "[ClearAllSkills] Skills apagados")
	c.emitter.Emit("skills:cleared", nil)
	return nil
}

func (c *SettingsController) ClearAllChannels() error {
	if c.restartChannel == nil {
		return fmt.Errorf("restartChannel não configurado")
	}
	// sem acesso direto ao gateway — usa callback injetado
	c.emitter.Emit("channels:cleared", nil)
	logging.Println(context.Background(), "controllers.settings-controller", "[ClearAllChannels] Evento channels:cleared emitido")
	return nil
}

// ResetDatabase apaga o banco de dados, resetando ao estado inicial.
func (c *SettingsController) ResetDatabase() error {
	configPath, err := config.GetConfigPath()
	if err != nil {
		return fmt.Errorf("erro ao obter caminho do banco de dados: %v", err)
	}
	dbPath := filepath.Join(filepath.Dir(configPath), "conversations.db")

	if err := database.Close(); err != nil {
		return fmt.Errorf("erro ao fechar banco de dados: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	if _, err := os.Stat(dbPath); err == nil {
		if err := os.Remove(dbPath); err != nil {
			return fmt.Errorf("erro ao remover banco de dados: %v", err)
		}
		_ = os.Remove(dbPath + "-wal")
		_ = os.Remove(dbPath + "-shm")
	}

	if err := database.Init(); err != nil {
		return fmt.Errorf("erro ao reinicializar banco: %v", err)
	}
	logging.Println(context.Background(), "controllers.settings-controller", "[ResetDatabase] Banco resetado com sucesso")
	c.emitter.Emit("database:reset", nil)
	return nil
}

// ClearMessages apaga as mensagens e conversas pertencentes ao usuário do
// contexto. Usa ClearAllConversationsWithContext, que respeita o escopo do
// usuário; o caller (Wails binding) é responsável por validar autenticação
// antes de chamar.
func (c *SettingsController) ClearMessages(ctx context.Context) error {
	if err := database.ClearAllConversationsWithContext(ctx); err != nil {
		return fmt.Errorf("erro ao limpar mensagens e conversas: %v", err)
	}
	logging.Println(ctx, "controllers.settings-controller", "[ClearMessages] Mensagens e conversas apagadas")
	c.emitter.Emit("messages:cleared", nil)
	return nil
}
