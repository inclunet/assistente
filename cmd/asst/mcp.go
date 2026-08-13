package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	mcpmgr "assistente/internal/mcp"

	"github.com/spf13/cobra"
)

// mcpBackend abstracts the app methods used by mcp commands.
type mcpBackend interface {
	ListMCPServers() []mcpmgr.ServerInfo
	SaveMCPServer(slug string, cfg mcpmgr.ServerConfig) error
	ConnectMCPServer(slug string) error
	DisconnectMCPServer(slug string) error
	GetMCPServerTools(slug string) []mcpmgr.MCPToolInfo
	DeleteMCPServer(slug string) error
}

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
		return runMCPList(asCLI(rootApp), os.Stdout)
	},
}

func runMCPList(svc mcpBackend, out io.Writer) error {
	servers := svc.ListMCPServers()
	if len(servers) == 0 {
		_, err := fmt.Fprintln(out, "Nenhum servidor MCP configurado.")
		return err
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "SLUG\tNOME\tTRANSPORTE\tSTATUS\tTOOLS\tATIVO")
	for _, s := range servers {
		enabled := ""
		if s.Enabled {
			enabled = "sim"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\n",
			s.Slug, s.Name, s.Transport, s.Status, s.ToolCount, enabled)
	}
	return w.Flush()
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
		return runMCPAdd(asCLI(rootApp), os.Stdout, args[0], mcpAddCommand, mcpAddArgs, mcpAddEnv, mcpAddURL, mcpAddTransport)
	},
}

func runMCPAdd(svc mcpBackend, out io.Writer, slug, command, cmdArgs string, envVars []string, url, transport string) error {
	t := mcpmgr.TransportStdio
	if transport != "" {
		t = mcpmgr.TransportType(transport)
	}

	cfg := mcpmgr.ServerConfig{
		Name:      slug,
		Transport: t,
		Command:   command,
		URL:       url,
		Enabled:   true,
	}

	if cmdArgs != "" {
		cfg.Args = strings.Split(cmdArgs, " ")
	}

	if len(envVars) > 0 {
		cfg.Env = make(map[string]string)
		for _, e := range envVars {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				cfg.Env[parts[0]] = parts[1]
			}
		}
	}

	if err := svc.SaveMCPServer(slug, cfg); err != nil {
		return fmt.Errorf("erro ao salvar servidor: %w", err)
	}

	_, err := fmt.Fprintf(out, "Servidor MCP '%s' adicionado.\n", slug)
	return err
}

// ─── connect / disconnect ───────────────────────────────────────────────────

var mcpConnectCmd = &cobra.Command{
	Use:   "connect <slug>",
	Short: "Conecta a um servidor MCP",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMCPConnect(asCLI(rootApp), os.Stdout, args[0])
	},
}

func runMCPConnect(svc mcpBackend, out io.Writer, slug string) error {
	_, _ = fmt.Fprintf(out, "Conectando a '%s'... ", slug)
	if err := svc.ConnectMCPServer(slug); err != nil {
		_, _ = fmt.Fprintln(out, "FALHOU")
		return fmt.Errorf("erro: %w", err)
	}
	_, err := fmt.Fprintln(out, "OK")
	return err
}

var mcpDisconnectCmd = &cobra.Command{
	Use:   "disconnect <slug>",
	Short: "Desconecta de um servidor MCP",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMCPDisconnect(asCLI(rootApp), os.Stdout, args[0])
	},
}

func runMCPDisconnect(svc mcpBackend, out io.Writer, slug string) error {
	if err := svc.DisconnectMCPServer(slug); err != nil {
		return fmt.Errorf("erro ao desconectar: %w", err)
	}
	_, err := fmt.Fprintf(out, "Servidor '%s' desconectado.\n", slug)
	return err
}

// ─── tools ──────────────────────────────────────────────────────────────────

var mcpToolsCmd = &cobra.Command{
	Use:   "tools <slug>",
	Short: "Lista tools de um servidor MCP",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMCPTools(asCLI(rootApp), os.Stdout, args[0])
	},
}

func runMCPTools(svc mcpBackend, out io.Writer, slug string) error {
	tools := svc.GetMCPServerTools(slug)
	if len(tools) == 0 {
		_, err := fmt.Fprintf(out, "Nenhuma tool no servidor '%s'.\n", slug)
		return err
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NOME\tDESCRIÇÃO")
	for _, t := range tools {
		desc := t.Description
		if len(desc) > 60 {
			desc = desc[:57] + "..."
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\n", t.Name, desc)
	}
	return w.Flush()
}

// ─── remove ─────────────────────────────────────────────────────────────────

var mcpRemoveCmd = &cobra.Command{
	Use:   "remove <slug>",
	Short: "Remove um servidor MCP",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMCPRemove(asCLI(rootApp), os.Stdout, args[0])
	},
}

func runMCPRemove(svc mcpBackend, out io.Writer, slug string) error {
	if err := svc.DeleteMCPServer(slug); err != nil {
		return fmt.Errorf("erro ao remover servidor: %w", err)
	}
	_, err := fmt.Fprintf(out, "Servidor MCP '%s' removido.\n", slug)
	return err
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
