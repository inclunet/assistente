package main

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"assistente/internal/profiles"

	"github.com/spf13/cobra"
)

// profilesBackend abstracts the app methods used by profile commands (enables testing).
type profilesBackend interface {
	GetProfiles() ([]profiles.ProfileInfo, error)
	GetActiveProfileSlug() string
	GetProfile(slug string) (*profiles.Profile, error)
	SetActiveProfile(slug string) error
	CreateProfile(p profiles.Profile) (string, error)
	UpdateProfile(slug string, p profiles.Profile) error
	DuplicateProfile(slug string) (string, error)
	DeleteProfile(slug string) error
}

var profilesCmd = &cobra.Command{
	Use:   "profiles",
	Short: "Gerencia perfis do assistente",
	Long:  "Lista, cria, edita, duplica e remove perfis do assistente.",
}

var profilesListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lista todos os perfis disponíveis",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProfilesList(rootApp, os.Stdout)
	},
}

func runProfilesList(svc profilesBackend, out io.Writer) error {
	items, err := svc.GetProfiles()
	if err != nil {
		return fmt.Errorf("erro ao listar perfis: %w", err)
	}

	activeSlug := svc.GetActiveProfileSlug()

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SLUG\tNOME\tORIGEM\tATIVO")
	for _, p := range items {
		active := ""
		if p.Slug == activeSlug {
			active = "*"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.Slug, p.Name, p.Source, active)
	}
	return w.Flush()
}

var profilesShowCmd = &cobra.Command{
	Use:   "show [slug]",
	Short: "Exibe detalhes de um perfil",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProfilesShow(rootApp, os.Stdout, args[0])
	},
}

func runProfilesShow(svc profilesBackend, out io.Writer, slug string) error {
	profile, err := svc.GetProfile(slug)
	if err != nil {
		return fmt.Errorf("perfil '%s' não encontrado: %w", slug, err)
	}

	fmt.Fprintf(out, "Nome:        %s\n", profile.Name)
	fmt.Fprintf(out, "Descrição:   %s\n", profile.Description)
	fmt.Fprintf(out, "Ícone:       %s\n", profile.Icon)

	if profile.Chat.LLMProvider != "" {
		fmt.Fprintf(out, "Provider:    %s\n", profile.Chat.LLMProvider)
	}
	if profile.Chat.Model != "" {
		fmt.Fprintf(out, "Modelo:      %s\n", profile.Chat.Model)
	}
	fmt.Fprintf(out, "Temperatura: %.1f\n", profile.Chat.Temperature)
	if profile.Chat.MaxTokens > 0 {
		fmt.Fprintf(out, "Max Tokens:  %d\n", profile.Chat.MaxTokens)
	}
	if len(profile.Chat.EnabledTools) > 0 {
		fmt.Fprintf(out, "Tools:       %v\n", profile.Chat.EnabledTools)
	}

	return nil
}

var profilesActivateCmd = &cobra.Command{
	Use:   "activate <slug>",
	Short: "Ativa um perfil",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProfilesActivate(rootApp, os.Stdout, args[0])
	},
}

func runProfilesActivate(svc profilesBackend, out io.Writer, slug string) error {
	if err := svc.SetActiveProfile(slug); err != nil {
		return fmt.Errorf("erro ao ativar perfil '%s': %w", slug, err)
	}
	fmt.Fprintf(out, "Perfil '%s' ativado.\n", slug)
	return nil
}

// ─── create ─────────────────────────────────────────────────────────────────

var profileCreateName string
var profileCreateModel string
var profileCreateProvider string
var profileCreateTemp float64

var profilesCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Cria um novo perfil",
	Long: `Cria um novo perfil de interação.

Exemplos:
  assistente profiles create --name "Coder" --provider openai-default --model gpt-4o
  assistente profiles create --name "Escritor" --temperature 0.9`,

	RunE: func(cmd *cobra.Command, args []string) error {
		return runProfilesCreate(rootApp, os.Stdout, cmd)
	},
}

func runProfilesCreate(svc profilesBackend, out io.Writer, cmd *cobra.Command) error {
	if profileCreateName == "" {
		return fmt.Errorf("--name é obrigatório")
	}

	p := *profiles.DefaultProfile()
	p.Name = profileCreateName

	if cmd.Flags().Changed("provider") {
		p.Chat.LLMProvider = profileCreateProvider
	}
	if cmd.Flags().Changed("model") {
		p.Chat.Model = profileCreateModel
	}
	if cmd.Flags().Changed("temperature") {
		p.Chat.Temperature = profileCreateTemp
	}

	slug, err := svc.CreateProfile(p)
	if err != nil {
		return fmt.Errorf("erro ao criar perfil: %w", err)
	}

	fmt.Fprintf(out, "Perfil '%s' criado (slug: %s).\n", profileCreateName, slug)
	return nil
}

// ─── edit ───────────────────────────────────────────────────────────────────

var profileEditName string
var profileEditModel string
var profileEditProvider string
var profileEditTemp float64

var profilesEditCmd = &cobra.Command{
	Use:   "edit <slug>",
	Short: "Edita campos de um perfil existente",
	Long: `Edita campos de um perfil existente via flags.
Apenas os campos fornecidos são alterados.

Exemplos:
  assistente profiles edit coder --model gpt-4o-mini
  assistente profiles edit tradutor --name "Tradutor PRO" --temperature 0.3`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProfilesEdit(rootApp, os.Stdout, cmd, args[0])
	},
}

func runProfilesEdit(svc profilesBackend, out io.Writer, cmd *cobra.Command, slug string) error {
	profile, err := svc.GetProfile(slug)
	if err != nil {
		return fmt.Errorf("perfil '%s' não encontrado: %w", slug, err)
	}

	if profileEditName != "" {
		profile.Name = profileEditName
	}
	if profileEditModel != "" {
		profile.Chat.Model = profileEditModel
	}
	if profileEditProvider != "" {
		profile.Chat.LLMProvider = profileEditProvider
	}
	if cmd.Flags().Changed("temperature") {
		profile.Chat.Temperature = profileEditTemp
	}

	if err := svc.UpdateProfile(slug, *profile); err != nil {
		return fmt.Errorf("erro ao atualizar perfil: %w", err)
	}

	fmt.Fprintf(out, "Perfil '%s' atualizado.\n", slug)
	return nil
}

// ─── duplicate ──────────────────────────────────────────────────────────────

var profilesDuplicateCmd = &cobra.Command{
	Use:   "duplicate <slug>",
	Short: "Duplica um perfil existente",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProfilesDuplicate(rootApp, os.Stdout, args[0])
	},
}

func runProfilesDuplicate(svc profilesBackend, out io.Writer, slug string) error {
	newSlug, err := svc.DuplicateProfile(slug)
	if err != nil {
		return fmt.Errorf("erro ao duplicar perfil '%s': %w", slug, err)
	}
	fmt.Fprintf(out, "Perfil duplicado: %s\n", newSlug)
	return nil
}

// ─── delete ─────────────────────────────────────────────────────────────────

var profilesDeleteCmd = &cobra.Command{
	Use:   "delete <slug>",
	Short: "Remove um perfil",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProfilesDelete(rootApp, os.Stdout, args[0])
	},
}

func runProfilesDelete(svc profilesBackend, out io.Writer, slug string) error {
	if err := svc.DeleteProfile(slug); err != nil {
		return fmt.Errorf("erro ao remover perfil '%s': %w", slug, err)
	}
	fmt.Fprintf(out, "Perfil '%s' removido.\n", slug)
	return nil
}

func init() {
	profilesCreateCmd.Flags().StringVar(&profileCreateName, "name", "", "Nome do perfil (obrigatório)")
	profilesCreateCmd.Flags().StringVar(&profileCreateModel, "model", "", "Modelo LLM")
	profilesCreateCmd.Flags().StringVar(&profileCreateProvider, "provider", "", "ID do provider LLM")
	profilesCreateCmd.Flags().Float64Var(&profileCreateTemp, "temperature", 0.7, "Temperatura (0.0-2.0)")

	profilesEditCmd.Flags().StringVar(&profileEditName, "name", "", "Novo nome")
	profilesEditCmd.Flags().StringVar(&profileEditModel, "model", "", "Novo modelo")
	profilesEditCmd.Flags().StringVar(&profileEditProvider, "provider", "", "Novo provider")
	profilesEditCmd.Flags().Float64Var(&profileEditTemp, "temperature", 0, "Nova temperatura")

	profilesCmd.AddCommand(profilesListCmd)
	profilesCmd.AddCommand(profilesShowCmd)
	profilesCmd.AddCommand(profilesActivateCmd)
	profilesCmd.AddCommand(profilesCreateCmd)
	profilesCmd.AddCommand(profilesEditCmd)
	profilesCmd.AddCommand(profilesDuplicateCmd)
	profilesCmd.AddCommand(profilesDeleteCmd)
}
