package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"assistente/internal/apidto"

	"github.com/spf13/cobra"
)

// providersBackend abstracts the app methods used by providers commands.
type providersBackend interface {
	providerSetupBackend
	GetLLMProvidersWithStatus() []map[string]interface{}
	TestLLMProvider(req apidto.TestLLMProviderRequest) (bool, error)
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
		return runProvidersList(asCLI(rootApp), os.Stdout)
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
		return runProviderCommand(asCLI(rootApp), os.Stdout, false)
	},
}

func runProvidersAdd(svc providersBackend, out io.Writer, reader *bufio.Reader, readPwd passwordReader) error {
	return configureProvider(svc, out, reader, readPwd, false)
}

// ─── test ───────────────────────────────────────────────────────────────────

var providersTestCmd = &cobra.Command{
	Use:   "test <id>",
	Short: "Testa conexão com um provedor",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProvidersTest(asCLI(rootApp), os.Stdout, args[0])
	},
}

func runProvidersTest(svc providersBackend, out io.Writer, id string) error {
	_, _ = fmt.Fprintf(out, "Testando provedor '%s'... ", id)

	ok, err := svc.TestLLMProvider(apidto.TestLLMProviderRequest{
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
		return runProvidersModels(asCLI(rootApp), os.Stdout, args[0])
	},
}

func runProvidersModels(svc providersBackend, out io.Writer, id string) error {
	models, err := svc.ListModelsRaw(apidto.TestLLMProviderRequest{
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
		return runProvidersDefault(asCLI(rootApp), os.Stdout, args[0])
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
		return runProvidersRemove(asCLI(rootApp), os.Stdout, args[0])
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
