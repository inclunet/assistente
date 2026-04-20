package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"syscall"

	"assistente/controllers"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// setupBackend abstracts the app methods used by the setup wizard (enables testing).
type setupBackend interface {
	NeedsWelcomeWizard() bool
	HasMasterKey() bool
	SetupMasterPassword(password string) (string, error)
	TestLLMProvider(req controllers.TestLLMProviderRequest) (bool, error)
	ListModelsRaw(req controllers.TestLLMProviderRequest) ([]string, error)
	CreateDefaultLLMProvider(providerType, apiKey string) error
	SetDefaultProvider(id string) error
	SetChatModel(model string) error
}

// passwordReader abstracts password reading for testing.
type passwordReader func(prompt string) (string, error)

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
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSetup(rootApp, readPassword, os.Stdout)
	},
}

func runSetup(svc setupBackend, readPwd passwordReader, out io.Writer) error {
	reader := bufio.NewReader(os.Stdin)

	if !svc.NeedsWelcomeWizard() {
		_, _ = fmt.Fprintln(out, "O assistente já está configurado.")
		_, _ = fmt.Fprint(out, "Deseja reconfigurar? (s/N): ")
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "s" && answer != "sim" {
			return nil
		}
	}

	// === Passo 1: Senha mestre ===
	if !svc.HasMasterKey() {
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintln(out, "=== Senha Mestre ===")
		_, _ = fmt.Fprintln(out, "A senha mestre protege suas credenciais (API keys).")
		_, _ = fmt.Fprintln(out, "Ela é necessária para desbloquear o assistente.")
		_, _ = fmt.Fprintln(out)

		password, err := readMasterPassword(readPwd)
		if err != nil {
			return err
		}

		recoveryKey, err := svc.SetupMasterPassword(password)
		if err != nil {
			return fmt.Errorf("erro ao configurar senha mestre: %w", err)
		}

		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintln(out, "=== Chave de Recuperação ===")
		_, _ = fmt.Fprintln(out, "IMPORTANTE: Guarde esta chave em local seguro!")
		_, _ = fmt.Fprintln(out, "Ela é a única forma de recuperar suas credenciais se esquecer a senha.")
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintf(out, "  %s\n", recoveryKey)
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprint(out, "Pressione Enter após anotar a chave...")
		_, _ = reader.ReadString('\n')
	} else {
		_, _ = fmt.Fprintln(out, "Senha mestre já configurada.")
	}

	// === Passo 2: Provedor LLM ===
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "=== Provedor LLM ===")
	_, _ = fmt.Fprintln(out, "Escolha o provedor de IA que deseja usar:")
	_, _ = fmt.Fprintln(out)

	for i, choice := range providerChoices {
		_, _ = fmt.Fprintf(out, "  %2d. %s\n", i+1, choice)
	}
	_, _ = fmt.Fprintln(out)

	providerChoice, err := readProviderChoice(reader, out)
	if err != nil {
		return err
	}

	info := controllers.GetWizardProviderInfo(providerChoice)

	// === Passo 3: API Key ===
	apiKey := ""
	needsAPIKey := providerChoice != "Ollama (Local)"

	if needsAPIKey {
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintf(out, "Informe a API key para %s.\n", providerChoice)
		key, readErr := readPwd("API Key: ")
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

	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprint(out, "Testando conexão... ")

	ok, testErr := svc.TestLLMProvider(controllers.TestLLMProviderRequest{
		Type:   providerType,
		APIKey: apiKey,
	})
	if testErr != nil || !ok {
		_, _ = fmt.Fprintln(out, "FALHOU")
		if testErr != nil {
			_, _ = fmt.Fprintf(out, "Erro: %v\n", testErr)
		}
		_, _ = fmt.Fprint(out, "Deseja continuar mesmo assim? (s/N): ")
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "s" && answer != "sim" {
			return fmt.Errorf("configuração cancelada")
		}
	} else {
		_, _ = fmt.Fprintln(out, "OK")
	}

	// === Passo 5: Escolher modelo ===
	model := info.DefaultModel

	models, modelsErr := svc.ListModelsRaw(controllers.TestLLMProviderRequest{
		Type:   providerType,
		APIKey: apiKey,
	})

	if modelsErr == nil && len(models) > 0 {
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintln(out, "Modelos disponíveis:")
		_, _ = fmt.Fprintln(out)
		displayCount := len(models)
		if displayCount > 20 {
			displayCount = 20
		}
		for i := 0; i < displayCount; i++ {
			marker := "  "
			if models[i] == model {
				marker = "* "
			}
			_, _ = fmt.Fprintf(out, "  %s%2d. %s\n", marker, i+1, models[i])
		}
		if len(models) > 20 {
			_, _ = fmt.Fprintf(out, "  ... e mais %d modelos.\n", len(models)-20)
		}
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintf(out, "Escolha o modelo (Enter para '%s'): ", model)

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
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprint(out, "Modelo (nome exato): ")
		line, _ := reader.ReadString('\n')
		model = strings.TrimSpace(line)
	}

	// === Passo 6: Criar provedor ===
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprint(out, "Criando provedor... ")

	err = svc.CreateDefaultLLMProvider(providerType, apiKey)
	if err != nil {
		_, _ = fmt.Fprintln(out, "FALHOU")
		return fmt.Errorf("erro ao criar provedor: %w", err)
	}
	_, _ = fmt.Fprintln(out, "OK")

	// Definir como padrão
	if info.ID != "" {
		_ = svc.SetDefaultProvider(info.ID)
	}

	// Aplicar modelo selecionado
	if model != "" {
		_ = svc.SetChatModel(model)
	}

	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintf(out, "Assistente configurado com sucesso!\n")
	_, _ = fmt.Fprintf(out, "  Provedor: %s\n", providerChoice)
	_, _ = fmt.Fprintf(out, "  Modelo:   %s\n", model)
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Use 'assistente chat' para começar a conversar.")

	return nil
}

// readMasterPassword lê e confirma a senha mestre do terminal.
func readMasterPassword(readPwd passwordReader) (string, error) {
	password, err := readPwd("Senha mestre: ")
	if err != nil {
		return "", fmt.Errorf("erro ao ler senha: %w", err)
	}
	if len(password) < 8 {
		return "", fmt.Errorf("a senha deve ter pelo menos 8 caracteres")
	}

	confirm, err := readPwd("Confirmar senha: ")
	if err != nil {
		return "", fmt.Errorf("erro ao ler confirmação: %w", err)
	}
	if password != confirm {
		return "", fmt.Errorf("as senhas não coincidem")
	}

	return password, nil
}

// readProviderChoice lê a escolha de provider do terminal.
func readProviderChoice(reader *bufio.Reader, out io.Writer) (string, error) {
	_, _ = fmt.Fprintf(out, "Escolha (1-%d): ", len(providerChoices))
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
