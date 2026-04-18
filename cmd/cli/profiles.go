package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var profilesCmd = &cobra.Command{
	Use:   "profiles",
	Short: "Gerencia perfis do assistente",
	Long:  "Lista, exibe detalhes e ativa perfis do assistente.",
}

var profilesListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lista todos os perfis disponíveis",
	RunE: func(cmd *cobra.Command, args []string) error {
		items, err := rootApp.GetProfiles()
		if err != nil {
			return fmt.Errorf("erro ao listar perfis: %w", err)
		}

		activeSlug := rootApp.GetActiveProfileSlug()

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "SLUG\tNOME\tORIGEM\tATIVO")
		for _, p := range items {
			active := ""
			if p.Slug == activeSlug {
				active = "*"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.Slug, p.Name, p.Source, active)
		}
		return w.Flush()
	},
}

var profilesShowCmd = &cobra.Command{
	Use:   "show [slug]",
	Short: "Exibe detalhes de um perfil",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		slug := args[0]
		profile, err := rootApp.GetProfile(slug)
		if err != nil {
			return fmt.Errorf("perfil '%s' não encontrado: %w", slug, err)
		}

		fmt.Printf("Nome:        %s\n", profile.Name)
		fmt.Printf("Descrição:   %s\n", profile.Description)
		fmt.Printf("Ícone:       %s\n", profile.Icon)

		if profile.Chat.LLMProvider != "" {
			fmt.Printf("Provider:    %s\n", profile.Chat.LLMProvider)
		}
		if profile.Chat.Model != "" {
			fmt.Printf("Modelo:      %s\n", profile.Chat.Model)
		}
		if profile.Chat.Temperature > 0 {
			fmt.Printf("Temperatura: %.1f\n", profile.Chat.Temperature)
		}
		if profile.Chat.MaxTokens > 0 {
			fmt.Printf("Max Tokens:  %d\n", profile.Chat.MaxTokens)
		}
		if len(profile.Chat.EnabledTools) > 0 {
			fmt.Printf("Tools:       %v\n", profile.Chat.EnabledTools)
		}

		return nil
	},
}

var profilesActivateCmd = &cobra.Command{
	Use:   "activate <slug>",
	Short: "Ativa um perfil",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		slug := args[0]
		if err := rootApp.SetActiveProfile(slug); err != nil {
			return fmt.Errorf("erro ao ativar perfil '%s': %w", slug, err)
		}
		fmt.Printf("Perfil '%s' ativado.\n", slug)
		return nil
	},
}

func init() {
	profilesCmd.AddCommand(profilesListCmd)
	profilesCmd.AddCommand(profilesShowCmd)
	profilesCmd.AddCommand(profilesActivateCmd)
}
