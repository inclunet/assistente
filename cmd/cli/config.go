package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Exibe configurações do assistente",
	Long:  "Mostra perfil ativo, modelo, providers e configurações do assistente.",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Exibe visão geral da configuração",
	RunE: func(cmd *cobra.Command, args []string) error {
		profile, err := rootApp.GetActiveProfile()
		if err != nil {
			return fmt.Errorf("erro ao obter perfil ativo: %w", err)
		}

		fmt.Printf("Perfil ativo:  %s\n", profile.Name)
		fmt.Printf("Provider:      %s\n", profile.Chat.LLMProvider)
		fmt.Printf("Modelo:        %s\n", profile.Chat.Model)
		fmt.Printf("Temperatura:   %.1f\n", profile.Chat.Temperature)
		fmt.Printf("Max Tokens:    %d\n", profile.Chat.MaxTokens)
		fmt.Printf("Timeout (s):   %d\n", profile.Chat.ResponseTimeout)

		return nil
	},
}

var configProvidersCmd = &cobra.Command{
	Use:   "providers",
	Short: "Lista provedores LLM configurados",
	RunE: func(cmd *cobra.Command, args []string) error {
		providers := rootApp.GetLLMProviders()
		if len(providers) == 0 {
			fmt.Println("Nenhum provedor LLM configurado.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tTIPO\tNOME\tBASE URL\tPADRÃO")
		for _, p := range providers {
			def := ""
			if p.IsDefault {
				def = "*"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", p.ID, p.Type, p.Name, p.BaseURL, def)
		}
		return w.Flush()
	},
}

var configModelCmd = &cobra.Command{
	Use:   "model <nome>",
	Short: "Altera o modelo do perfil ativo",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		model := args[0]
		if err := rootApp.SetChatModel(model); err != nil {
			return fmt.Errorf("erro ao definir modelo: %w", err)
		}
		fmt.Printf("Modelo alterado para '%s'.\n", model)
		return nil
	},
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configProvidersCmd)
	configCmd.AddCommand(configModelCmd)
}
