package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	mcpmgr "assistente/internal/mcp"

	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Gerencia servidores MCP",
	Long:  "Lista, adiciona, conecta/desconecta e remove servidores MCP (Model Context Protocol).",
}

// ─── list ───────────────────────────────────────────────────────────────────

var mcpListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lista servidores MCP e status",
	RunE: func(cmd *cobra.Command, args []string) error {
		servers := rootApp.ListMCPServers()
		if len(servers) == 0 {
			fmt.Println("Nenhum servidor MCP configurado.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "SLUG\tNOME\tTRANSPORTE\tSTATUS\tTOOLS\tATIVO")
		for _, s := range servers {
			enabled := ""
			if s.Enabled {
				enabled = "sim"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\n",
				s.Slug, s.Name, s.Transport, s.Status, s.ToolCount, enabled)
		}
		return w.Flush()
	},
}

// ─── add ────────────────────────────────────────────────────────────────────

var mcpAddCommand string
var mcpAddArgs string
var mcpAddEnv []string
var mcpAddURL string
var mcpAddTransport string

var mcpAddCmd = &cobra.Command{
	Use:   "add <slug>",
	Short: "Adiciona um servidor MCP",
	Long: `Adiciona um novo servidor MCP.

Exemplos:
  assistente mcp add filesystem --command npx --args "@modelcontextprotocol/server-filesystem /tmp"
  assistente mcp add remote-api --url https://mcp.example.com/sse --transport sse`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		slug := args[0]

		transport := mcpmgr.TransportStdio
		if mcpAddTransport != "" {
			transport = mcpmgr.TransportType(mcpAddTransport)
		}

		cfg := mcpmgr.ServerConfig{
			Name:      slug,
			Transport: transport,
			Command:   mcpAddCommand,
			URL:       mcpAddURL,
			Enabled:   true,
		}

		if mcpAddArgs != "" {
			cfg.Args = strings.Split(mcpAddArgs, " ")
		}

		if len(mcpAddEnv) > 0 {
			cfg.Env = make(map[string]string)
			for _, e := range mcpAddEnv {
				parts := strings.SplitN(e, "=", 2)
				if len(parts) == 2 {
					cfg.Env[parts[0]] = parts[1]
				}
			}
		}

		if err := rootApp.SaveMCPServer(slug, cfg); err != nil {
			return fmt.Errorf("erro ao salvar servidor: %w", err)
		}

		fmt.Printf("Servidor MCP '%s' adicionado.\n", slug)
		return nil
	},
}

// ─── connect / disconnect ───────────────────────────────────────────────────

var mcpConnectCmd = &cobra.Command{
	Use:   "connect <slug>",
	Short: "Conecta a um servidor MCP",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		slug := args[0]
		fmt.Printf("Conectando a '%s'... ", slug)
		if err := rootApp.ConnectMCPServer(slug); err != nil {
			fmt.Println("FALHOU")
			return fmt.Errorf("erro: %w", err)
		}
		fmt.Println("OK")
		return nil
	},
}

var mcpDisconnectCmd = &cobra.Command{
	Use:   "disconnect <slug>",
	Short: "Desconecta de um servidor MCP",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		slug := args[0]
		if err := rootApp.DisconnectMCPServer(slug); err != nil {
			return fmt.Errorf("erro ao desconectar: %w", err)
		}
		fmt.Printf("Servidor '%s' desconectado.\n", slug)
		return nil
	},
}

// ─── tools ──────────────────────────────────────────────────────────────────

var mcpToolsCmd = &cobra.Command{
	Use:   "tools <slug>",
	Short: "Lista tools de um servidor MCP",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		slug := args[0]
		tools := rootApp.GetMCPServerTools(slug)
		if len(tools) == 0 {
			fmt.Printf("Nenhuma tool no servidor '%s'.\n", slug)
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NOME\tDESCRIÇÃO")
		for _, t := range tools {
			desc := t.Description
			if len(desc) > 60 {
				desc = desc[:57] + "..."
			}
			fmt.Fprintf(w, "%s\t%s\n", t.Name, desc)
		}
		return w.Flush()
	},
}

// ─── remove ─────────────────────────────────────────────────────────────────

var mcpRemoveCmd = &cobra.Command{
	Use:   "remove <slug>",
	Short: "Remove um servidor MCP",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		slug := args[0]
		if err := rootApp.DeleteMCPServer(slug); err != nil {
			return fmt.Errorf("erro ao remover servidor: %w", err)
		}
		fmt.Printf("Servidor MCP '%s' removido.\n", slug)
		return nil
	},
}

func init() {
	mcpAddCmd.Flags().StringVar(&mcpAddCommand, "command", "", "Comando para iniciar o servidor (modo stdio)")
	mcpAddCmd.Flags().StringVar(&mcpAddArgs, "args", "", "Argumentos do comando (separados por espaço)")
	mcpAddCmd.Flags().StringSliceVar(&mcpAddEnv, "env", nil, "Variáveis de ambiente (KEY=VALUE, pode repetir)")
	mcpAddCmd.Flags().StringVar(&mcpAddURL, "url", "", "URL do servidor (modo SSE/Streamable)")
	mcpAddCmd.Flags().StringVar(&mcpAddTransport, "transport", "stdio", "Tipo de transporte (stdio, sse, streamable)")

	mcpCmd.AddCommand(mcpListCmd)
	mcpCmd.AddCommand(mcpAddCmd)
	mcpCmd.AddCommand(mcpConnectCmd)
	mcpCmd.AddCommand(mcpDisconnectCmd)
	mcpCmd.AddCommand(mcpToolsCmd)
	mcpCmd.AddCommand(mcpRemoveCmd)
}
