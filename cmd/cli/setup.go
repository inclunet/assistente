package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"assistente/controllers"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// providerChoices é a lista ordenada de providers para o wizard interativo.
var providerChoices = []string{
	"OpenAI",
	"Anthropic (Claude)",
	"Google (Gemini)",
	"DeepSeek",
	"xAI (Grok)",
	"OpenRouter",
	"Mistral AI",
	"Groq",
	"Together AI",
	"Fireworks AI",
	"Perplexity",
	"Azure OpenAI",
	"Ollama (Local)",
	"LiteLLM",
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configura o assistente pela primeira vez",
	Long: `Wizard interativo de configuração inicial.
Equivalente ao Welcome Wizard do modo desktop.

Configura:
  - Senha mestre (para proteger credenciais)
  - Provedor LLM (OpenAI, Claude, Ollama, etc.)
  - API key e modelo padrão`,
	RunE: runSetup,
}

func runSetup(cmd *cobra.Command, args []string) error {
	reader := bufio.NewReader(os.Stdin)

	if !rootApp.NeedsWelcomeWizard() {
		fmt.Println("O assistente já está configurado.")
		fmt.Print("Deseja reconfigurar? (s/N): ")
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "s" && answer != "sim" {
			return nil
		}
	}

	// === Passo 1: Senha mestre ===
	if !rootApp.HasMasterKey() {
		fmt.Println()
		fmt.Println("=== Senha Mestre ===")
		fmt.Println("A senha mestre protege suas credenciais (API keys).")
		fmt.Println("Ela é necessária para desbloquear o assistente.")
		fmt.Println()

		password, err := readMasterPassword(reader)
		if err != nil {
			return err
		}

		recoveryKey, err := rootApp.SetupMasterPassword(password)
		if err != nil {
			return fmt.Errorf("erro ao configurar senha mestre: %w", err)
		}

		fmt.Println()
		fmt.Println("=== Chave de Recuperação ===")
		fmt.Println("IMPORTANTE: Guarde esta chave em local seguro!")
		fmt.Println("Ela é a única forma de recuperar suas credenciais se esquecer a senha.")
		fmt.Println()
		fmt.Printf("  %s\n", recoveryKey)
		fmt.Println()
		fmt.Print("Pressione Enter após anotar a chave...")
		_, _ = reader.ReadString('\n')
	} else {
		fmt.Println("Senha mestre já configurada.")
	}

	// === Passo 2: Provedor LLM ===
	fmt.Println()
	fmt.Println("=== Provedor LLM ===")
	fmt.Println("Escolha o provedor de IA que deseja usar:")
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

	// === Passo 3: API Key ===
	apiKey := ""
	needsAPIKey := providerChoice != "Ollama (Local)"

	if needsAPIKey {
		fmt.Println()
		fmt.Printf("Informe a API key para %s.\n", providerChoice)
		key, readErr := readPassword("API Key: ")
		if readErr != nil {
			return fmt.Errorf("erro ao ler API key: %w", readErr)
		}
		apiKey = key
		if apiKey == "" {
			return fmt.Errorf("API key não pode ser vazia")
		}
	}

	// === Passo 4: Testar conexão ===
	providerType := controllers.WizardLabelToProviderType(providerChoice)
	if providerType == "" {
		providerType = string(info.Type)
	}

	fmt.Println()
	fmt.Print("Testando conexão... ")

	ok, testErr := rootApp.TestLLMProvider(controllers.TestLLMProviderRequest{
		Type:   providerType,
		APIKey: apiKey,
	})
	if testErr != nil || !ok {
		fmt.Println("FALHOU")
		if testErr != nil {
			fmt.Printf("Erro: %v\n", testErr)
		}
		fmt.Print("Deseja continuar mesmo assim? (s/N): ")
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "s" && answer != "sim" {
			return fmt.Errorf("configuração cancelada")
		}
	} else {
		fmt.Println("OK")
	}

	// === Passo 5: Escolher modelo ===
	model := info.DefaultModel

	models, modelsErr := rootApp.ListModelsRaw(controllers.TestLLMProviderRequest{
		Type:   providerType,
		APIKey: apiKey,
	})

	if modelsErr == nil && len(models) > 0 {
		fmt.Println()
		fmt.Println("Modelos disponíveis:")
		fmt.Println()
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
		fmt.Printf("Escolha o modelo (Enter para '%s'): ", model)

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
		fmt.Println()
		fmt.Print("Modelo (nome exato): ")
		line, _ := reader.ReadString('\n')
		model = strings.TrimSpace(line)
	}

	// === Passo 6: Criar provedor ===
	fmt.Println()
	fmt.Print("Criando provedor... ")

	err = rootApp.CreateDefaultLLMProvider(providerType, apiKey)
	if err != nil {
		fmt.Println("FALHOU")
		return fmt.Errorf("erro ao criar provedor: %w", err)
	}
	fmt.Println("OK")

	// Definir como padrão
	if info.ID != "" {
		_ = rootApp.SetDefaultProvider(info.ID)
	}

	fmt.Println()
	fmt.Printf("Assistente configurado com sucesso!\n")
	fmt.Printf("  Provedor: %s\n", providerChoice)
	fmt.Printf("  Modelo:   %s\n", model)
	fmt.Println()
	fmt.Println("Use 'assistente chat' para começar a conversar.")

	return nil
}

// readMasterPassword lê e confirma a senha mestre do terminal.
func readMasterPassword(reader *bufio.Reader) (string, error) {
	password, err := readPassword("Senha mestre: ")
	if err != nil {
		return "", fmt.Errorf("erro ao ler senha: %w", err)
	}
	if len(password) < 8 {
		return "", fmt.Errorf("a senha deve ter pelo menos 8 caracteres")
	}

	confirm, err := readPassword("Confirmar senha: ")
	if err != nil {
		return "", fmt.Errorf("erro ao ler confirmação: %w", err)
	}
	if password != confirm {
		return "", fmt.Errorf("as senhas não coincidem")
	}

	return password, nil
}

// readProviderChoice lê a escolha de provider do terminal.
func readProviderChoice(reader *bufio.Reader) (string, error) {
	fmt.Printf("Escolha (1-%d): ", len(providerChoices))
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("erro ao ler escolha: %w", err)
	}

	line = strings.TrimSpace(line)
	num, err := strconv.Atoi(line)
	if err != nil || num < 1 || num > len(providerChoices) {
		return "", fmt.Errorf("escolha inválida: %s (use 1-%d)", line, len(providerChoices))
	}

	return providerChoices[num-1], nil
}

// readPassword lê uma senha do terminal sem eco (oculta caracteres).
func readPassword(prompt string) (string, error) {
	fmt.Print(prompt)
	fd := int(syscall.Stdin)
	if term.IsTerminal(fd) {
		bytes, err := term.ReadPassword(fd)
		fmt.Println() // nova linha após input oculto
		if err != nil {
			return "", err
		}
		return string(bytes), nil
	}
	// Fallback para pipe/redireção: lê como texto normal
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func init() {
	// setup é registrado em main.go
}
