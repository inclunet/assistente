package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"assistente/controllers"

	"github.com/spf13/cobra"
)

// credentialsBackend abstracts the app methods used by credentials commands.
type credentialsBackend interface {
	ListCredentials() ([]controllers.CredentialSummary, error)
	UpsertCredential(input controllers.CredentialInput) error
	DeleteCredential(pattern string) error
}

var credentialsCmd = &cobra.Command{
	Use:   "credentials",
	Short: "Gerencia credenciais (API keys, tokens)",
	Long:  "Lista, cria/atualiza e remove credenciais armazenadas de forma segura.",
}

// ─── list ───────────────────────────────────────────────────────────────────

var credentialsListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lista credenciais registradas (sem exibir secrets)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCredentialsList(rootApp, os.Stdout)
	},
}

func runCredentialsList(svc credentialsBackend, out io.Writer) error {
	items, err := svc.ListCredentials()
	if err != nil {
		return fmt.Errorf("erro ao listar credenciais: %w", err)
	}
	if len(items) == 0 {
		_, _ = fmt.Fprintln(out, "Nenhuma credencial registrada.")
		return nil
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "PADRÃO\tTIPO\tVALOR (MASCARADO)\tGERENCIADA")
	for _, c := range items {
		managed := ""
		if c.Managed {
			managed = "sim"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", c.Pattern, c.Type, c.Masked, managed)
	}
	return w.Flush()
}

// ─── set ────────────────────────────────────────────────────────────────────

var credentialsSetCmd = &cobra.Command{
	Use:   "set <pattern>",
	Short: "Cria ou atualiza uma credencial",
	Long: `Cria ou atualiza credencial para o padrão especificado.

O valor pode ser passado via:
  --value "sk-..."           (flag)
  echo "sk-..." | asst credentials set api.openai.com  (stdin/pipe)

Exemplos:
  asst credentials set api.openai.com --value "sk-abc123"
  asst credentials set api.anthropic.com --type bearer --value "sk-ant-..."`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCredentialsSet(rootApp, os.Stdout, args[0], credSetValue, credSetType, readPassword)
	},
}

var credSetValue string
var credSetType string

func runCredentialsSet(svc credentialsBackend, out io.Writer, pattern, value, credType string, readPwd passwordReader) error {
	if value == "" {
		// Tentar ler do stdin (pipe)
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			v, err := readStdinLine()
			if err != nil {
				return fmt.Errorf("erro ao ler valor do stdin: %w", err)
			}
			value = v
		}
	}
	if value == "" {
		// Modo interativo: ler senha oculta
		v, err := readPwd("Valor (secret): ")
		if err != nil {
			return fmt.Errorf("erro ao ler valor: %w", err)
		}
		value = v
	}
	if value == "" {
		return fmt.Errorf("valor não pode ser vazio — use --value ou pipe")
	}

	if credType == "" {
		credType = "bearer"
	}

	input := controllers.CredentialInput{
		Pattern: pattern,
		Type:    credType,
		Token:   value,
	}

	if err := svc.UpsertCredential(input); err != nil {
		return fmt.Errorf("erro ao salvar credencial: %w", err)
	}

	_, _ = fmt.Fprintf(out, "Credencial '%s' salva.\n", pattern)
	return nil
}

// ─── remove ─────────────────────────────────────────────────────────────────

var credentialsRemoveCmd = &cobra.Command{
	Use:   "remove <pattern>",
	Short: "Remove uma credencial",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCredentialsRemove(rootApp, os.Stdout, args[0])
	},
}

func runCredentialsRemove(svc credentialsBackend, out io.Writer, pattern string) error {
	if err := svc.DeleteCredential(pattern); err != nil {
		return fmt.Errorf("erro ao remover credencial: %w", err)
	}
	_, _ = fmt.Fprintf(out, "Credencial '%s' removida.\n", pattern)
	return nil
}

// ─── helpers ────────────────────────────────────────────────────────────────

func readStdinLine() (string, error) {
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 256)
	for {
		n, err := os.Stdin.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	return strings.TrimSpace(string(buf)), nil
}

func init() {
	credentialsSetCmd.Flags().StringVar(&credSetValue, "value", "", "Valor da credencial (API key, token)")
	credentialsSetCmd.Flags().StringVar(&credSetType, "type", "bearer", "Tipo da credencial (bearer, basic, custom)")

	credentialsCmd.AddCommand(credentialsListCmd)
	credentialsCmd.AddCommand(credentialsSetCmd)
	credentialsCmd.AddCommand(credentialsRemoveCmd)
}
