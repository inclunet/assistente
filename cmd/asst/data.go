package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"assistente/internal/app"
	"assistente/internal/portability"

	"github.com/spf13/cobra"
)

type dataBackend interface {
	ExportData(req app.ExportRequest) (string, error)
	ExportDataToFile(req app.ExportRequest, path string) (string, error)
	AnalyzeImportData(jsonData string, credentialExportPassword string) (*app.ImportAnalysis, error)
	ImportDataWithResolutions(req app.ImportRequest) (*app.ImportResult, error)
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
	dataExportTaskListIDs        []string
	dataExportIncludeCredentials bool
	dataExportCredentialPassword string
	dataExportIncludeAudio       bool

	dataAnalyzeCredentialPassword string

	dataImportCredentialPassword string
	dataImportResolutions        []string
)

var dataExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Exporta dados para JSON, HTML ou PDF",
	Long: `Exporta dados do assistente em formato portátil.

Exemplos:
  asst data export --all --out backup.json
  asst data export --conversation-id 12 --format html
  asst data export --conversation-id 12 --format pdf --out conversa.pdf
  asst data export --all --include-credentials --credential-password "senha" --out backup.json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		req := app.ExportRequest{
			OutputFormat:             strings.ToLower(strings.TrimSpace(dataExportFormat)),
			All:                      dataExportAll,
			ConversationIDs:          append([]string(nil), dataExportConversationIDs...),
			ProviderIDs:              append([]string(nil), dataExportProviderIDs...),
			TaskListIDs:              append([]string(nil), dataExportTaskListIDs...),
			IncludeCredentials:       dataExportIncludeCredentials,
			CredentialExportPassword: dataExportCredentialPassword,
			IncludeAudio:             dataExportIncludeAudio,
		}
		return runDataExport(rootApp, os.Stdout, req, dataExportOut)
	},
}

var dataAnalyzeCmd = &cobra.Command{
	Use:   "analyze <arquivo>",
	Short: "Analisa um arquivo de importação",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDataAnalyze(rootApp, os.Stdout, os.ReadFile, args[0], dataAnalyzeCredentialPassword)
	},
}

var dataImportCmd = &cobra.Command{
	Use:   "import <arquivo>",
	Short: "Importa um arquivo portátil",
	Long: `Importa um arquivo portátil do assistente.

Resoluções de conflito podem ser passadas repetindo --resolution:
  --resolution "credential=overwrite=api.openai.com"
  --resolution "provider=rename=openai-custom=>openai-custom-copia"
  --resolution "taskList=skip=roadmap-2026"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		resolutions, err := parseImportResolutionSpecs(dataImportResolutions)
		if err != nil {
			return err
		}
		return runDataImport(rootApp, os.Stdout, os.ReadFile, args[0], dataImportCredentialPassword, resolutions)
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

func runDataImport(svc dataBackend, out io.Writer, readFile func(string) ([]byte, error), path string, credentialPassword string, resolutions []app.ImportResolution) error {
	raw, err := readFile(path)
	if err != nil {
		return fmt.Errorf("erro ao ler arquivo de importação: %w", err)
	}

	result, err := svc.ImportDataWithResolutions(app.ImportRequest{
		JSONData:                 string(raw),
		CredentialExportPassword: credentialPassword,
		Resolutions:              resolutions,
	})
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

func parseImportResolutionSpecs(specs []string) ([]app.ImportResolution, error) {
	if len(specs) == 0 {
		return nil, nil
	}

	resolutions := make([]app.ImportResolution, 0, len(specs))
	for _, spec := range specs {
		resolution, err := parseImportResolutionSpec(spec)
		if err != nil {
			return nil, err
		}
		resolutions = append(resolutions, resolution)
	}
	return resolutions, nil
}

func parseImportResolutionSpec(spec string) (app.ImportResolution, error) {
	parts := strings.SplitN(spec, "=", 3)
	if len(parts) != 3 {
		return app.ImportResolution{}, fmt.Errorf("resolução inválida %q: use tipo=estrategia=identificador ou tipo=rename=identificador=>novo-valor", spec)
	}

	resourceType := strings.TrimSpace(parts[0])
	strategy := portability.ConflictResolutionStrategy(strings.TrimSpace(parts[1]))
	rawValue := strings.TrimSpace(parts[2])
	if resourceType == "" || rawValue == "" {
		return app.ImportResolution{}, fmt.Errorf("resolução inválida %q: tipo e identificador são obrigatórios", spec)
	}

	switch strategy {
	case portability.ConflictResolutionSkip, portability.ConflictResolutionOverwrite:
		return app.ImportResolution{
			ResourceType: resourceType,
			Identifier:   rawValue,
			Strategy:     strategy,
		}, nil
	case portability.ConflictResolutionRename:
		renameParts := strings.SplitN(rawValue, "=>", 2)
		if len(renameParts) != 2 {
			return app.ImportResolution{}, fmt.Errorf("resolução inválida %q: rename exige identificador=>novo-valor", spec)
		}
		identifier := strings.TrimSpace(renameParts[0])
		renameValue := strings.TrimSpace(renameParts[1])
		if identifier == "" || renameValue == "" {
			return app.ImportResolution{}, fmt.Errorf("resolução inválida %q: rename exige identificador e novo valor", spec)
		}
		return app.ImportResolution{
			ResourceType: resourceType,
			Identifier:   identifier,
			Strategy:     strategy,
			RenameValue:  renameValue,
		}, nil
	default:
		return app.ImportResolution{}, fmt.Errorf("estratégia de conflito inválida %q", strategy)
	}
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

func init() {
	dataExportCmd.Flags().StringVar(&dataExportFormat, "format", portability.FormatJSON, "Formato de saída: json, html ou pdf")
	dataExportCmd.Flags().StringVarP(&dataExportOut, "out", "o", "", "Arquivo de saída (obrigatório para PDF)")
	dataExportCmd.Flags().BoolVar(&dataExportAll, "all", false, "Exporta todas as conversas, providers e task lists persistidos")
	dataExportCmd.Flags().StringSliceVar(&dataExportConversationIDs, "conversation-id", nil, "ID de conversa para exportar (repetível)")
	dataExportCmd.Flags().StringSliceVar(&dataExportProviderIDs, "provider-id", nil, "ID de provider para exportar (repetível)")
	dataExportCmd.Flags().StringSliceVar(&dataExportTaskListIDs, "tasklist-id", nil, "ID de task list para exportar (repetível)")
	dataExportCmd.Flags().BoolVar(&dataExportIncludeCredentials, "include-credentials", false, "Inclui credenciais exportáveis")
	dataExportCmd.Flags().StringVar(&dataExportCredentialPassword, "credential-password", "", "Senha para exportar/descriptografar credenciais")
	dataExportCmd.Flags().BoolVar(&dataExportIncludeAudio, "include-audio", false, "Inclui metadados de anexos de áudio")

	dataAnalyzeCmd.Flags().StringVar(&dataAnalyzeCredentialPassword, "credential-password", "", "Senha para analisar credenciais criptografadas")

	dataImportCmd.Flags().StringVar(&dataImportCredentialPassword, "credential-password", "", "Senha para importar credenciais criptografadas")
	dataImportCmd.Flags().StringArrayVar(&dataImportResolutions, "resolution", nil, "Resolução de conflito no formato tipo=estrategia=identificador ou tipo=rename=identificador=>novo-valor")

	dataCmd.AddCommand(dataExportCmd)
	dataCmd.AddCommand(dataAnalyzeCmd)
	dataCmd.AddCommand(dataImportCmd)
}
