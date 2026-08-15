package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"assistente/internal/app"
	"assistente/internal/database"
	"assistente/internal/llm"
	mcpmgr "assistente/internal/mcp"
	"assistente/internal/portability"

	"github.com/spf13/cobra"
)

type dataBackend interface {
	ExportData(req app.ExportRequest) (string, error)
	ExportDataToFile(req app.ExportRequest, path string) (string, error)
	AnalyzeImportData(jsonData string, credentialExportPassword string) (*app.ImportAnalysis, error)
	ImportData(jsonData string, credentialExportPassword string) (*app.ImportResult, error)
	GetConversations() ([]app.Conversation, error)
	GetLLMProviders() []*llm.ProviderConfig
	GetAllTaskLists() ([]database.TaskList, error)
	ListMCPServers() ([]mcpmgr.ServerInfo, error)
}

var dataCmd = &cobra.Command{
	Use:   "data",
	Short: "Importa e exporta dados portáveis",
	Long:  "Exporta, analisa e importa dados no formato portátil do assistente.",
}

var (
	dataExportFormat             string
	dataExportOut                string
	dataExportAll                bool
	dataExportConversationIDs    []string
	dataExportProviderIDs        []string
	dataExportMCPServerSlugs     []string
	dataExportTaskListIDs        []string
	dataExportIncludeCredentials bool
	dataExportCredentialPassword string
	dataExportIncludeAudio       bool
	dataExportConversations      bool
	dataExportProviders          bool
	dataExportMCPServers         bool
	dataExportTaskLists          bool
	dataExportCredentialsOnly    bool

	dataAnalyzeCredentialPassword string

	dataImportCredentialPassword string
)

var dataExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Exporta dados para JSON, HTML ou PDF",
	Long: `Exporta dados do assistente em formato portátil.

Exemplos:
  asst data export --all --out backup.json
  asst data export --providers --out providers.json
  asst data export --tasklists --out tasklists.json
  asst data export --only-credentials --credential-password "senha" --out credenciais.json
  asst data export --conversation-id 12 --format html
  asst data export --conversation-id 12 --format pdf --out conversa.pdf
  asst data export --all --include-credentials --credential-password "senha" --out backup.json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		req := app.ExportRequest{
			OutputFormat:             strings.ToLower(strings.TrimSpace(dataExportFormat)),
			All:                      dataExportAll,
			ConversationIDs:          append([]string(nil), dataExportConversationIDs...),
			ProviderIDs:              append([]string(nil), dataExportProviderIDs...),
			MCPServerSlugs:           append([]string(nil), dataExportMCPServerSlugs...),
			TaskListIDs:              append([]string(nil), dataExportTaskListIDs...),
			IncludeCredentials:       dataExportIncludeCredentials,
			CredentialExportPassword: dataExportCredentialPassword,
			IncludeAudio:             dataExportIncludeAudio,
		}
		req, err := prepareDataExportRequest(asCLI(rootApp), req, dataExportSelection{
			Conversations:   dataExportConversations,
			Providers:       dataExportProviders,
			MCPServers:      dataExportMCPServers,
			TaskLists:       dataExportTaskLists,
			CredentialsOnly: dataExportCredentialsOnly,
		})
		if err != nil {
			return err
		}
		return runDataExport(asCLI(rootApp), os.Stdout, req, dataExportOut)
	},
}

type dataExportSelection struct {
	Conversations   bool
	Providers       bool
	MCPServers      bool
	TaskLists       bool
	CredentialsOnly bool
}

var dataAnalyzeCmd = &cobra.Command{
	Use:   "analyze <arquivo>",
	Short: "Analisa um arquivo de importação",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDataAnalyze(asCLI(rootApp), os.Stdout, os.ReadFile, args[0], dataAnalyzeCredentialPassword)
	},
}

var dataImportCmd = &cobra.Command{
	Use:   "import <arquivo>",
	Short: "Importa um arquivo portátil",
	Long:  "Importa um arquivo portátil do assistente no modelo UUID versionado.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDataImport(asCLI(rootApp), os.Stdout, os.ReadFile, args[0], dataImportCredentialPassword)
	},
}

func runDataExport(svc dataBackend, out io.Writer, req app.ExportRequest, outPath string) error {
	if strings.TrimSpace(req.OutputFormat) == "" {
		req.OutputFormat = portability.FormatJSON
	}
	req.OutputFormat = strings.ToLower(strings.TrimSpace(req.OutputFormat))

	if req.OutputFormat == portability.FormatPDF && strings.TrimSpace(outPath) == "" {
		return fmt.Errorf("arquivo de saída é obrigatório para exportação PDF")
	}

	if strings.TrimSpace(outPath) != "" {
		path, err := svc.ExportDataToFile(req, outPath)
		if err != nil {
			return fmt.Errorf("erro ao exportar dados: %w", err)
		}
		_, err = fmt.Fprintf(out, "Dados exportados para %s\n", path)
		return err
	}

	rendered, err := svc.ExportData(req)
	if err != nil {
		return fmt.Errorf("erro ao exportar dados: %w", err)
	}
	_, err = io.WriteString(out, rendered)
	return err
}

func prepareDataExportRequest(svc dataBackend, req app.ExportRequest, selection dataExportSelection) (app.ExportRequest, error) {
	hasSpecificIDs := len(req.ConversationIDs) > 0 || len(req.ProviderIDs) > 0 || len(req.MCPServerSlugs) > 0 || len(req.TaskListIDs) > 0
	hasTypeSelection := selection.Conversations || selection.Providers || selection.MCPServers || selection.TaskLists

	if req.All && (hasSpecificIDs || hasTypeSelection || selection.CredentialsOnly) {
		return req, fmt.Errorf("--all não pode ser combinado com seleções específicas")
	}
	if selection.CredentialsOnly && (hasSpecificIDs || hasTypeSelection || req.All) {
		return req, fmt.Errorf("--only-credentials não pode ser combinado com outros recursos")
	}

	if selection.CredentialsOnly {
		req.IncludeCredentials = true
		req.ExplicitSelection = true
		return req, nil
	}

	if !hasSpecificIDs && !hasTypeSelection {
		return req, nil
	}

	req.ExplicitSelection = true

	if selection.Conversations {
		conversations, err := svc.GetConversations()
		if err != nil {
			return req, fmt.Errorf("erro ao listar conversas para exportação: %w", err)
		}
		ids := make([]string, 0, len(conversations))
		for _, conversation := range conversations {
			ids = append(ids, strings.TrimSpace(conversation.ID))
		}
		req.ConversationIDs = mergeUniqueStrings(req.ConversationIDs, ids)
	}

	if selection.Providers {
		providers := svc.GetLLMProviders()
		ids := make([]string, 0, len(providers))
		for _, provider := range providers {
			if provider == nil {
				continue
			}
			id := strings.TrimSpace(provider.ID)
			if id == "" {
				continue
			}
			ids = append(ids, id)
		}
		req.ProviderIDs = mergeUniqueStrings(req.ProviderIDs, ids)
	}

	if selection.MCPServers {
		servers, err := svc.ListMCPServers()
		if err != nil {
			return req, fmt.Errorf("erro ao listar servidores MCP para exportação: %w", err)
		}
		slugs := make([]string, 0, len(servers))
		for _, server := range servers {
			slugs = append(slugs, strings.TrimSpace(server.Slug))
		}
		req.MCPServerSlugs = mergeUniqueStrings(req.MCPServerSlugs, slugs)
	}

	if selection.TaskLists {
		taskLists, err := svc.GetAllTaskLists()
		if err != nil {
			return req, fmt.Errorf("erro ao listar task lists para exportação: %w", err)
		}
		ids := make([]string, 0, len(taskLists))
		for _, taskList := range taskLists {
			ids = append(ids, strings.TrimSpace(taskList.ID))
		}
		req.TaskListIDs = mergeUniqueStrings(req.TaskListIDs, ids)
	}

	return req, nil
}

func runDataAnalyze(svc dataBackend, out io.Writer, readFile func(string) ([]byte, error), path string, credentialPassword string) error {
	raw, err := readFile(path)
	if err != nil {
		return fmt.Errorf("erro ao ler arquivo de importação: %w", err)
	}

	analysis, err := svc.AnalyzeImportData(string(raw), credentialPassword)
	if err != nil {
		return fmt.Errorf("erro ao analisar importação: %w", err)
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "CAMPO\tVALOR")
	_, _ = fmt.Fprintf(w, "Versão\t%d\n", analysis.Version)
	_, _ = fmt.Fprintf(w, "Versão do app\t%s\n", fallbackDash(analysis.AppVersion))
	_, _ = fmt.Fprintf(w, "Conversas\t%d\n", analysis.ConversationCount)
	_, _ = fmt.Fprintf(w, "Mensagens\t%d\n", analysis.MessageCount)
	_, _ = fmt.Fprintf(w, "Providers\t%d\n", analysis.ProviderCount)
	_, _ = fmt.Fprintf(w, "Task lists\t%d\n", analysis.TaskListCount)
	_, _ = fmt.Fprintf(w, "Tarefas\t%d\n", analysis.TaskCount)
	_, _ = fmt.Fprintf(w, "Notas\t%d\n", analysis.TaskNoteCount)
	_, _ = fmt.Fprintf(w, "Credenciais\t%d\n", analysis.CredentialCount)
	_, _ = fmt.Fprintf(w, "Inclui credenciais\t%s\n", yesNo(analysis.IncludesCredentials))
	_, _ = fmt.Fprintf(w, "Requer senha de credenciais\t%s\n", yesNo(analysis.RequiresCredentialPassword))
	_, _ = fmt.Fprintf(w, "Conflitos\t%d\n", analysis.ConflictCount)
	_ = w.Flush()

	printConflictGroup(out, "Conflitos de conversas", analysis.ConversationConflicts)
	printConflictGroup(out, "Conflitos de providers", analysis.ProviderConflicts)
	printConflictGroup(out, "Conflitos de task lists", analysis.TaskListConflicts)
	printConflictGroup(out, "Conflitos de credenciais", analysis.CredentialConflicts)

	if len(analysis.UnsupportedResourceTypes) > 0 {
		_, _ = fmt.Fprintf(out, "\nRecursos fora do escopo atual: %s\n", strings.Join(analysis.UnsupportedResourceTypes, ", "))
	}
	if analysis.CredentialAnalysisError != "" {
		_, _ = fmt.Fprintf(out, "\nAviso de credenciais: %s\n", analysis.CredentialAnalysisError)
	}
	for _, warning := range analysis.Warnings {
		_, _ = fmt.Fprintf(out, "\nAviso: %s\n", warning)
	}

	return nil
}

func runDataImport(svc dataBackend, out io.Writer, readFile func(string) ([]byte, error), path string, credentialPassword string) error {
	raw, err := readFile(path)
	if err != nil {
		return fmt.Errorf("erro ao ler arquivo de importação: %w", err)
	}

	result, err := svc.ImportData(string(raw), credentialPassword)
	if err != nil {
		return fmt.Errorf("erro ao importar dados: %w", err)
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "CAMPO\tVALOR")
	_, _ = fmt.Fprintf(w, "Sucesso\t%s\n", yesNo(result.Success))
	_, _ = fmt.Fprintf(w, "Importados\t%d\n", result.Imported)
	_, _ = fmt.Fprintf(w, "Ignorados\t%d\n", result.Skipped)
	_, _ = fmt.Fprintf(w, "Falhas\t%d\n", result.Failed)
	_, _ = fmt.Fprintf(w, "Ignorados por conversa vazia\t%d\n", result.SkippedEmptyConversations)
	_, _ = fmt.Fprintf(w, "Ignorados por conflito de conversa\t%d\n", result.SkippedConversationConflict)
	_, _ = fmt.Fprintf(w, "Ignorados por conflito de provider\t%d\n", result.SkippedProviderConflict)
	_, _ = fmt.Fprintf(w, "Ignorados por conflito de task list\t%d\n", result.SkippedTaskListConflict)
	_, _ = fmt.Fprintf(w, "Ignorados por conflito de credencial\t%d\n", result.SkippedCredentialConflict)
	_, _ = fmt.Fprintf(w, "Ignorados por outros motivos\t%d\n", result.SkippedOther)
	_ = w.Flush()

	if strings.TrimSpace(result.Message) != "" {
		_, _ = fmt.Fprintf(out, "\n%s\n", result.Message)
	}
	if len(result.UnsupportedResourceTypes) > 0 {
		_, _ = fmt.Fprintf(out, "\nRecursos fora do escopo atual: %s\n", strings.Join(result.UnsupportedResourceTypes, ", "))
	}
	for _, warning := range result.Warnings {
		_, _ = fmt.Fprintf(out, "\nAviso: %s\n", warning)
	}
	for _, importErr := range result.Errors {
		_, _ = fmt.Fprintf(out, "\nErro: %s\n", importErr)
	}

	if !result.Success {
		return fmt.Errorf("importação concluída com falhas")
	}
	return nil
}

func printConflictGroup(out io.Writer, title string, conflicts []portability.ImportConflict) {
	if len(conflicts) == 0 {
		return
	}

	_, _ = fmt.Fprintf(out, "\n%s:\n", title)
	for _, conflict := range conflicts {
		_, _ = fmt.Fprintf(
			out,
			"- %s | %s | estratégias: %s\n",
			conflict.Identifier,
			conflict.Reason,
			strings.Join(stringifyStrategies(conflict.SupportedStrategies), ", "),
		)
	}
}

func stringifyStrategies(strategies []portability.ConflictResolutionStrategy) []string {
	items := make([]string, 0, len(strategies))
	for _, strategy := range strategies {
		items = append(items, string(strategy))
	}
	return items
}

func yesNo(value bool) string {
	if value {
		return "sim"
	}
	return "não"
}

func fallbackDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func mergeUniqueStrings(existing []string, incoming []string) []string {
	merged := make([]string, 0, len(existing)+len(incoming))
	seen := make(map[string]struct{}, len(existing)+len(incoming))

	for _, item := range existing {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		merged = append(merged, trimmed)
	}
	for _, item := range incoming {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		merged = append(merged, trimmed)
	}

	return merged
}

func init() {
	dataExportCmd.Flags().StringVar(&dataExportFormat, "format", portability.FormatJSON, "Formato de saída: json, html, pdf ou mcp-json")
	dataExportCmd.Flags().StringVarP(&dataExportOut, "out", "o", "", "Arquivo de saída (obrigatório para PDF)")
	dataExportCmd.Flags().BoolVar(&dataExportAll, "all", false, "Exporta todas as conversas, providers, servidores MCP e task lists persistidos")
	dataExportCmd.Flags().BoolVar(&dataExportConversations, "conversations", false, "Exporta todas as conversas")
	dataExportCmd.Flags().BoolVar(&dataExportProviders, "providers", false, "Exporta todos os providers persistidos")
	dataExportCmd.Flags().BoolVar(&dataExportMCPServers, "mcp-servers", false, "Exporta todos os servidores MCP")
	dataExportCmd.Flags().BoolVar(&dataExportTaskLists, "tasklists", false, "Exporta todas as task lists persistidas")
	dataExportCmd.Flags().BoolVar(&dataExportCredentialsOnly, "only-credentials", false, "Exporta apenas o bloco portátil de credenciais")
	dataExportCmd.Flags().StringSliceVar(&dataExportConversationIDs, "conversation-id", nil, "ID de conversa para exportar (repetível)")
	dataExportCmd.Flags().StringSliceVar(&dataExportProviderIDs, "provider-id", nil, "ID de provider para exportar (repetível)")
	dataExportCmd.Flags().StringSliceVar(&dataExportMCPServerSlugs, "mcp-server", nil, "Slug de servidor MCP para exportar (repetível)")
	dataExportCmd.Flags().StringSliceVar(&dataExportTaskListIDs, "tasklist-id", nil, "ID de task list para exportar (repetível)")
	dataExportCmd.Flags().BoolVar(&dataExportIncludeCredentials, "include-credentials", false, "Inclui credenciais exportáveis")
	dataExportCmd.Flags().StringVar(&dataExportCredentialPassword, "credential-password", "", "Senha para exportar/descriptografar credenciais")
	dataExportCmd.Flags().BoolVar(&dataExportIncludeAudio, "include-audio", false, "Inclui o conteúdo de áudio das mensagens, além de preservar audioMimeType")

	dataAnalyzeCmd.Flags().StringVar(&dataAnalyzeCredentialPassword, "credential-password", "", "Senha para analisar credenciais criptografadas")

	dataImportCmd.Flags().StringVar(&dataImportCredentialPassword, "credential-password", "", "Senha para importar credenciais criptografadas")

	dataCmd.AddCommand(dataExportCmd)
	dataCmd.AddCommand(dataAnalyzeCmd)
	dataCmd.AddCommand(dataImportCmd)
}
