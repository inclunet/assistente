package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var toolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "Gerencia ferramentas disponíveis",
}

var toolsListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lista ferramentas disponíveis (built-in + MCP)",
	RunE: func(cmd *cobra.Command, args []string) error {
		tools := rootApp.GetAvailableTools()
		if len(tools) == 0 {
			fmt.Println("Nenhuma ferramenta disponível.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(w, "NOME\tORIGEM\tDESCRIÇÃO")
		for _, t := range tools {
			desc := t.Description
			if len(desc) > 60 {
				desc = desc[:57] + "..."
			}
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", t.Name, t.SourceLabel, desc)
		}
		return w.Flush()
	},
}

func init() {
	toolsCmd.AddCommand(toolsListCmd)
}
