package main

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"assistente/internal/llm"
	"assistente/internal/profiles"

	"github.com/spf13/cobra"
)

// configBackend abstracts the app methods used by config commands.
type configBackend interface {
	GetActiveProfile() (*profiles.Profile, error)
	GetLLMProviders() []*llm.ProviderConfig
	SetChatModel(model string) error
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Exibe configurações do assistente",
	Long:  "Mostra perfil ativo, modelo, providers e configurações do assistente.",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Exibe visão geral da configuração",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runConfigShow(rootApp, os.Stdout)
	},
}

func runConfigShow(svc configBackend, out io.Writer) error {
	profile, err := svc.GetActiveProfile()
	if err != nil {
		return fmt.Errorf("erro ao obter perfil ativo: %w", err)
	}

	ew := &errWriter{w: out}
	ew.printf("Perfil ativo:  %s\n", profile.Name)
	ew.printf("Provider:      %s\n", profile.Chat.LLMProvider)
	ew.printf("Modelo:        %s\n", profile.Chat.Model)
	ew.printf("Temperatura:   %g\n", profile.Chat.Temperature)
	ew.printf("Max Tokens:    %d\n", profile.Chat.MaxTokens)
	ew.printf("Timeout (s):   %d\n", profile.Chat.ResponseTimeout)

	return ew.err
}

var configProvidersCmd = &cobra.Command{
	Use:   "providers",
	Short: "Lista provedores LLM configurados",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runConfigProviders(rootApp, os.Stdout)
	},
}

func runConfigProviders(svc configBackend, out io.Writer) error {
	providers := svc.GetLLMProviders()
	if len(providers) == 0 {
		_, err := fmt.Fprintln(out, "Nenhum provedor LLM configurado.")
		return err
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tTIPO\tNOME\tBASE URL\tPADRÃO")
	for _, p := range providers {
		def := ""
		if p.IsDefault {
			def = "*"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", p.ID, p.Type, p.Name, p.BaseURL, def)
	}
	return w.Flush()
}

var configModelCmd = &cobra.Command{
	Use:   "model <nome>",
	Short: "Altera o modelo do perfil ativo",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runConfigModel(rootApp, os.Stdout, args[0])
	},
}

func runConfigModel(svc configBackend, out io.Writer, model string) error {
	if err := svc.SetChatModel(model); err != nil {
		return fmt.Errorf("erro ao definir modelo: %w", err)
	}
	_, err := fmt.Fprintf(out, "Modelo alterado para '%s'.\n", model)
	return err
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configProvidersCmd)
	configCmd.AddCommand(configModelCmd)
}
