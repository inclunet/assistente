package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"assistente/controllers"
	"assistente/internal/profiles"

	"github.com/spf13/cobra"
)

// providersBackend abstracts the app methods used by providers commands.
type providersBackend interface {
	GetLLMProvidersWithStatus() []map[string]interface{}
	TestLLMProvider(req controllers.TestLLMProviderRequest) (bool, error)
	ListModelsRaw(req controllers.TestLLMProviderRequest) ([]string, error)
	CreateDefaultLLMProvider(providerType, apiKey string) error
	CreateLLMProvider(req controllers.CreateLLMProviderRequest) (map[string]interface{}, error)
	SetDefaultProvider(id string) error
	GetActiveProfileSlug() string
	GetProfile(slug string) (*profiles.Profile, error)
	UpdateProfile(slug string, p profiles.Profile) error
	DeleteLLMProvider(ctx context.Context, id string) error
}

var providersCmd = &cobra.Command{
	Use:   "providers",
	Short: "Gerencia provedores LLM",
	Long:  "Lista, adiciona, testa e remove provedores de modelos de linguagem.",
}

// ─── list ───────────────────────────────────────────────────────────────────

var providersListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lista provedores LLM com status de conexão",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProvidersList(rootApp, os.Stdout)
	},
}

func runProvidersList(svc providersBackend, out io.Writer) error {
	items := svc.GetLLMProvidersWithStatus()
	if len(items) == 0 {
		_, err := fmt.Fprintln(out, "Nenhum provedor configurado. Use 'asst providers add' ou 'asst setup'.")
		return err
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tTIPO\tNOME\tMODELO\tSTATUS\tPADRÃO")
	for _, p := range items {
		def := ""
		if isDefault, ok := p["isDefault"].(bool); ok && isDefault {
			def = "*"
		}
		status := str(p["status"])
		if status == "" {
			status = "-"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			str(p["id"]),
			str(p["type"]),
			str(p["name"]),
			str(p["defaultModel"]),
			status,
			def,
		)
	}
	return w.Flush()
}

// ─── add ────────────────────────────────────────────────────────────────────

var providersAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Adiciona um novo provedor LLM (interativo)",
	Long: `Wizard interativo para configurar um novo provedor LLM.
Solicita tipo, API key e modelo, testa a conexão e salva.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProvidersAdd(rootApp, os.Stdout, bufio.NewReader(os.Stdin), readPassword)
	},
}

func runProvidersAdd(svc providersBackend, out io.Writer, reader *bufio.Reader, readPwd passwordReader) error {
	// Passo 1: Escolher tipo
	ew := &errWriter{w: out}
	ew.println("Escolha o tipo de provedor:")
	ew.println()
	for i, choice := range providerChoices {
		ew.printf("  %2d. %s\n", i+1, choice)
	}
	ew.println()
	if ew.err != nil {
		return ew.err
	}

	providerChoice, err := readProviderChoice(reader, out)
	if err != nil {
		return err
	}

	info := controllers.GetWizardProviderInfo(providerChoice)
	providerType := controllers.WizardLabelToProviderType(providerChoice)
	if providerType == "" {
		providerType = string(info.Type)
	}

	// Passo 2: Base URL customizada (se necessário)
	baseURL := ""
	if providerChoice == "Azure OpenAI" || providerChoice == "LiteLLM" || info.Type == "custom" {
		_, _ = fmt.Fprint(out, "Base URL: ")
		line, _ := reader.ReadString('\n')
		baseURL = strings.TrimSpace(line)
		if baseURL == "" {
			return fmt.Errorf("base URL é obrigatória para %s", providerChoice)
		}
	}

	// Passo 3: API Key
	apiKey := ""
	needsAPIKey := providerChoice != "Ollama (Local)"

	if needsAPIKey {
		key, readErr := readPwd("API Key: ")
		if readErr != nil {
			return fmt.Errorf("erro ao ler API key: %w", readErr)
		}
		apiKey = key
		if apiKey == "" {
			return fmt.Errorf("API key não pode ser vazia")
		}
	}

	// Passo 4: Testar conexão
	_, _ = fmt.Fprint(out, "Testando conexão... ")
	testReq := controllers.TestLLMProviderRequest{
		Type:   providerType,
		APIKey: apiKey,
	}
	if baseURL != "" {
		testReq.BaseURL = baseURL
	}

	ok, testErr := svc.TestLLMProvider(testReq)
	if testErr != nil || !ok {
		_, _ = fmt.Fprintln(out, "FALHOU")
		if testErr != nil {
			_, _ = fmt.Fprintf(out, "Erro: %v\n", testErr)
		}
		_, _ = fmt.Fprint(out, "Continuar mesmo assim? (s/N): ")
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "s" && answer != "sim" {
			return fmt.Errorf("cancelado")
		}
	} else {
		_, _ = fmt.Fprintln(out, "OK")
	}

	// Passo 5: Escolher modelo
	model := info.DefaultModel
	modelsReq := controllers.TestLLMProviderRequest{
		Type:   providerType,
		APIKey: apiKey,
	}
	if baseURL != "" {
		modelsReq.BaseURL = baseURL
	}

	models, modelsErr := svc.ListModelsRaw(modelsReq)
	if modelsErr == nil && len(models) > 0 {
		mw := &errWriter{w: out}
		mw.println()
		mw.println("Modelos disponíveis:")
		displayCount := len(models)
		if displayCount > 20 {
			displayCount = 20
		}
		for i := 0; i < displayCount; i++ {
			marker := "  "
			if models[i] == model {
				marker = "* "
			}
			mw.printf("  %s%2d. %s\n", marker, i+1, models[i])
		}
		if len(models) > 20 {
			mw.printf("  ... e mais %d modelos.\n", len(models)-20)
		}
		mw.println()
		if mw.err != nil {
			return mw.err
		}

		defaultHint := ""
		if model != "" {
			defaultHint = fmt.Sprintf(" (Enter para '%s')", model)
		}
		_, _ = fmt.Fprintf(out, "Modelo%s: ", defaultHint)

		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line != "" {
			if num, err := strconv.Atoi(line); err == nil && num >= 1 && num <= len(models) {
				model = models[num-1]
			} else {
				model = line
			}
		}
	} else if model == "" {
		_, _ = fmt.Fprint(out, "Modelo (nome exato): ")
		line, _ := reader.ReadString('\n')
		model = strings.TrimSpace(line)
	}

	// Passo 6: Criar provedor
	if baseURL != "" {
		// Provedor com URL customizada — usar Create completo
		apiFormat := string(info.APIFormat)
		if apiFormat == "" {
			apiFormat = "openai"
		}
		_, err = svc.CreateLLMProvider(controllers.CreateLLMProviderRequest{
			ID:           info.ID,
			Name:         info.Name,
			Type:         providerType,
			APIFormat:    apiFormat,
			BaseURL:      baseURL,
			APIKey:       apiKey,
			DefaultModel: model,
		})
	} else {
		// Provedor padrão — usar template
		err = svc.CreateDefaultLLMProvider(providerType, apiKey)
		if err == nil && model != "" && model != info.DefaultModel {
			// Aplica o modelo selecionado ao perfil ativo
			// (CreateDefaultLLMProvider usa o default do template).
			_ = setActiveProfileChatModel(svc, model)
		}
	}

	if err != nil {
		return fmt.Errorf("erro ao criar provedor: %w", err)
	}

	_, err = fmt.Fprintf(out, "Provedor '%s' criado com sucesso.\n", providerChoice)
	return err
}

// ─── test ───────────────────────────────────────────────────────────────────

var providersTestCmd = &cobra.Command{
	Use:   "test <id>",
	Short: "Testa conexão com um provedor",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProvidersTest(rootApp, os.Stdout, args[0])
	},
}

func runProvidersTest(svc providersBackend, out io.Writer, id string) error {
	_, _ = fmt.Fprintf(out, "Testando provedor '%s'... ", id)

	ok, err := svc.TestLLMProvider(controllers.TestLLMProviderRequest{
		ProviderID: id,
	})
	if err != nil {
		_, _ = fmt.Fprintln(out, "FALHOU")
		return fmt.Errorf("erro: %w", err)
	}
	if !ok {
		_, _ = fmt.Fprintln(out, "FALHOU")
		return fmt.Errorf("conexão falhou")
	}
	_, err = fmt.Fprintln(out, "OK")
	return err
}

// ─── models ─────────────────────────────────────────────────────────────────

var providersModelsCmd = &cobra.Command{
	Use:   "models <id>",
	Short: "Lista modelos disponíveis de um provedor",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProvidersModels(rootApp, os.Stdout, args[0])
	},
}

func runProvidersModels(svc providersBackend, out io.Writer, id string) error {
	models, err := svc.ListModelsRaw(controllers.TestLLMProviderRequest{
		ProviderID: id,
	})
	if err != nil {
		return fmt.Errorf("erro ao listar modelos: %w", err)
	}
	if len(models) == 0 {
		_, err = fmt.Fprintln(out, "Nenhum modelo encontrado.")
		return err
	}
	ew := &errWriter{w: out}
	for _, m := range models {
		ew.println(m)
	}
	return ew.err
}

// ─── default ────────────────────────────────────────────────────────────────

var providersDefaultCmd = &cobra.Command{
	Use:   "default <id>",
	Short: "Define o provedor padrão",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProvidersDefault(rootApp, os.Stdout, args[0])
	},
}

func runProvidersDefault(svc providersBackend, out io.Writer, id string) error {
	if err := svc.SetDefaultProvider(id); err != nil {
		return fmt.Errorf("erro ao definir provedor padrão: %w", err)
	}
	_, err := fmt.Fprintf(out, "Provedor '%s' definido como padrão.\n", id)
	return err
}

// ─── remove ─────────────────────────────────────────────────────────────────

var providersRemoveCmd = &cobra.Command{
	Use:   "remove <id>",
	Short: "Remove um provedor LLM",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProvidersRemove(rootApp, os.Stdout, args[0])
	},
}

func runProvidersRemove(svc providersBackend, out io.Writer, id string) error {
	if err := svc.DeleteLLMProvider(context.Background(), id); err != nil {
		return fmt.Errorf("erro ao remover provedor: %w", err)
	}
	_, err := fmt.Fprintf(out, "Provedor '%s' removido.\n", id)
	return err
}

// ─── helpers ────────────────────────────────────────────────────────────────

// str extrai string de interface{} retornada por map[string]interface{}.
func str(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func init() {
	providersCmd.AddCommand(providersListCmd)
	providersCmd.AddCommand(providersAddCmd)
	providersCmd.AddCommand(providersTestCmd)
	providersCmd.AddCommand(providersModelsCmd)
	providersCmd.AddCommand(providersDefaultCmd)
	providersCmd.AddCommand(providersRemoveCmd)
}
