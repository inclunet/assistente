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
	GetActiveProfileSlug() string
	GetProfile(slug string) (*profiles.Profile, error)
	GetLLMProviders() []*llm.ProviderConfig
	UpdateProfile(slug string, p profiles.Profile) error
}

// chatModelUpdater agrupa os métodos necessários para fixar o modelo de chat no
// perfil ativo. Substitui o antigo config.SetChatModel legado (#299): o modelo
// passa a viver no perfil (profiles), não mais no config.json.
type chatModelUpdater interface {
	GetActiveProfileSlug() string
	GetProfile(slug string) (*profiles.Profile, error)
	UpdateProfile(slug string, p profiles.Profile) error
}

// setActiveProfileChatModel grava o modelo escolhido no perfil ativo. Resolve o
// slug ativo uma única vez (GetActiveProfileSlug agora compartilha a mesma
// resolução de GetActiveProfile — ver profiles.resolveActive) e usa esse mesmo
// slug para ler e gravar, garantindo que o modelo seja persistido exatamente no
// perfil que o app considera ativo.
func setActiveProfileChatModel(svc chatModelUpdater, model string) error {
	slug := svc.GetActiveProfileSlug()
	if slug == "" {
		return fmt.Errorf("nenhum perfil ativo")
	}
	profile, err := svc.GetProfile(slug)
	if err != nil {
		return err
	}
	if profile == nil {
		return fmt.Errorf("perfil ativo %q não encontrado", slug)
	}
	profile.Chat.Model = model
	return svc.UpdateProfile(slug, *profile)
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
	if err := setActiveProfileChatModel(svc, model); err != nil {
		return fmt.Errorf("erro ao definir modelo: %w", err)
	}
	_, err := fmt.Fprintf(out, "Modelo alterado para '%s' no perfil ativo.\n", model)
	return err
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configProvidersCmd)
	configCmd.AddCommand(configModelCmd)
}
