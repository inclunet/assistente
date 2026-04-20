package main

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"assistente/controllers"

	"github.com/spf13/cobra"
)

// toolsBackend abstracts the app methods used by tools commands.
type toolsBackend interface {
	GetAvailableTools() []controllers.ToolInfo
}

var toolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "Gerencia ferramentas disponíveis",
}

var toolsListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lista ferramentas disponíveis (built-in + MCP)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runToolsList(rootApp, os.Stdout)
	},
}

func runToolsList(svc toolsBackend, out io.Writer) error {
	tools := svc.GetAvailableTools()
	if len(tools) == 0 {
		fmt.Fprintln(out, "Nenhuma ferramenta disponível.")
		return nil
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NOME\tORIGEM\tDESCRIÇÃO")
	for _, t := range tools {
		desc := t.Description
		if len(desc) > 60 {
			desc = desc[:57] + "..."
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", t.Name, t.SourceLabel, desc)
	}
	return w.Flush()
}

func init() {
	toolsCmd.AddCommand(toolsListCmd)
}
