package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// setupBackend abstracts the app methods used by the setup wizard (enables testing).
type setupBackend interface {
	providerSetupBackend
	NeedsWelcomeWizard() bool
}

// passwordReader abstracts password reading for testing.
type passwordReader func(prompt string) (string, error)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configura o assistente pela primeira vez",
	Long: `Wizard interativo de configuração inicial.
Equivalente ao Welcome Wizard do modo desktop.

Configura:
  - Senha mestre (para proteger credenciais)
  - Conta local e sessão autenticada
  - Provedor LLM pelo catálogo ou configuração manual
  - API key e modelo padrão`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProviderCommand(asCLI(rootApp), os.Stdout, true)
	},
}

func runSetup(svc setupBackend, out io.Writer, reader *bufio.Reader, readPwd passwordReader) error {

	if !svc.NeedsWelcomeWizard() {
		answer, err := (providerPrompt{reader, out}).text("O assistente já está configurado. Criar outra conexão e usá-la no perfil ativo? (s/N)", "n", true)
		if err != nil {
			return err
		}
		answer = strings.ToLower(answer)
		if answer != "s" && answer != "sim" {
			return nil
		}
	}

	return configureProvider(svc, out, reader, readPwd, true)
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
