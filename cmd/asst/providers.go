package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"assistente/controllers"

	"github.com/spf13/cobra"
)

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
		items := rootApp.GetLLMProvidersWithStatus()
		if len(items) == 0 {
			fmt.Println("Nenhum provedor configurado. Use 'assistente providers add' ou 'assistente setup'.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tTIPO\tNOME\tMODELO\tSTATUS\tPADRÃO")
		for _, p := range items {
			def := ""
			if isDefault, ok := p["isDefault"].(bool); ok && isDefault {
				def = "*"
			}
			status := str(p["status"])
			if status == "" {
				status = "-"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				str(p["id"]),
				str(p["type"]),
				str(p["name"]),
				str(p["defaultModel"]),
				status,
				def,
			)
		}
		return w.Flush()
	},
}

// ─── add ────────────────────────────────────────────────────────────────────

var providersAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Adiciona um novo provedor LLM (interativo)",
	Long: `Wizard interativo para configurar um novo provedor LLM.
Solicita tipo, API key e modelo, testa a conexão e salva.`,
	RunE: runProvidersAdd,
}

func runProvidersAdd(cmd *cobra.Command, args []string) error {
	reader := bufio.NewReader(os.Stdin)

	// Passo 1: Escolher tipo
	fmt.Println("Escolha o tipo de provedor:")
	fmt.Println()
	for i, choice := range providerChoices {
		fmt.Printf("  %2d. %s\n", i+1, choice)
	}
	fmt.Println()

	providerChoice, err := readProviderChoice(reader)
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
		fmt.Print("Base URL: ")
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
		key, readErr := readPassword("API Key: ")
		if readErr != nil {
			return fmt.Errorf("erro ao ler API key: %w", readErr)
		}
		apiKey = key
		if apiKey == "" {
			return fmt.Errorf("API key não pode ser vazia")
		}
	}

	// Passo 4: Testar conexão
	fmt.Print("Testando conexão... ")
	testReq := controllers.TestLLMProviderRequest{
		Type:   providerType,
		APIKey: apiKey,
	}
	if baseURL != "" {
		testReq.BaseURL = baseURL
	}

	ok, testErr := rootApp.TestLLMProvider(testReq)
	if testErr != nil || !ok {
		fmt.Println("FALHOU")
		if testErr != nil {
			fmt.Printf("Erro: %v\n", testErr)
		}
		fmt.Print("Continuar mesmo assim? (s/N): ")
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "s" && answer != "sim" {
			return fmt.Errorf("cancelado")
		}
	} else {
		fmt.Println("OK")
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

	models, modelsErr := rootApp.ListModelsRaw(modelsReq)
	if modelsErr == nil && len(models) > 0 {
		fmt.Println()
		fmt.Println("Modelos disponíveis:")
		displayCount := len(models)
		if displayCount > 20 {
			displayCount = 20
		}
		for i := 0; i < displayCount; i++ {
			marker := "  "
			if models[i] == model {
				marker = "* "
			}
			fmt.Printf("  %s%2d. %s\n", marker, i+1, models[i])
		}
		if len(models) > 20 {
			fmt.Printf("  ... e mais %d modelos.\n", len(models)-20)
		}
		fmt.Println()

		defaultHint := ""
		if model != "" {
			defaultHint = fmt.Sprintf(" (Enter para '%s')", model)
		}
		fmt.Printf("Modelo%s: ", defaultHint)

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
		fmt.Print("Modelo (nome exato): ")
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
		_, err = rootApp.CreateLLMProvider(controllers.CreateLLMProviderRequest{
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
		err = rootApp.CreateDefaultLLMProvider(providerType, apiKey)
	}

	if err != nil {
		return fmt.Errorf("erro ao criar provedor: %w", err)
	}

	fmt.Printf("Provedor '%s' criado com sucesso.\n", providerChoice)
	return nil
}

// ─── test ───────────────────────────────────────────────────────────────────

var providersTestCmd = &cobra.Command{
	Use:   "test <id>",
	Short: "Testa conexão com um provedor",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		fmt.Printf("Testando provedor '%s'... ", id)

		ok, err := rootApp.TestLLMProvider(controllers.TestLLMProviderRequest{
			ProviderID: id,
		})
		if err != nil {
			fmt.Println("FALHOU")
			return fmt.Errorf("erro: %w", err)
		}
		if !ok {
			fmt.Println("FALHOU")
			return fmt.Errorf("conexão falhou")
		}
		fmt.Println("OK")
		return nil
	},
}

// ─── models ─────────────────────────────────────────────────────────────────

var providersModelsCmd = &cobra.Command{
	Use:   "models <id>",
	Short: "Lista modelos disponíveis de um provedor",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		models, err := rootApp.ListModelsRaw(controllers.TestLLMProviderRequest{
			ProviderID: id,
		})
		if err != nil {
			return fmt.Errorf("erro ao listar modelos: %w", err)
		}
		if len(models) == 0 {
			fmt.Println("Nenhum modelo encontrado.")
			return nil
		}
		for _, m := range models {
			fmt.Println(m)
		}
		return nil
	},
}

// ─── default ────────────────────────────────────────────────────────────────

var providersDefaultCmd = &cobra.Command{
	Use:   "default <id>",
	Short: "Define o provedor padrão",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		if err := rootApp.SetDefaultProvider(id); err != nil {
			return fmt.Errorf("erro ao definir provedor padrão: %w", err)
		}
		fmt.Printf("Provedor '%s' definido como padrão.\n", id)
		return nil
	},
}

// ─── remove ─────────────────────────────────────────────────────────────────

var providersRemoveCmd = &cobra.Command{
	Use:   "remove <id>",
	Short: "Remove um provedor LLM",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		if err := rootApp.DeleteLLMProvider(context.Background(), id); err != nil {
			return fmt.Errorf("erro ao remover provedor: %w", err)
		}
		fmt.Printf("Provedor '%s' removido.\n", id)
		return nil
	},
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
