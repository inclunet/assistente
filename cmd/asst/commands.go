package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"assistente/internal/app"
	"assistente/internal/commandcli"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

const commandCLILocale = "pt-BR"

// commandCLIBackend é a fronteira mínima da CLI. Mantê-la local permite testar
// a forma do protocolo sem iniciar o App, abrir banco ou executar comandos.
type commandCLIBackend interface {
	List(context.Context, string) ([]commandcli.Description, error)
	Describe(context.Context, string, string) (commandcli.Description, error)
	Execute(context.Context, commandcli.Request) (commandcli.Result, error)
	Retry(context.Context, commandcli.Request) (commandcli.Result, error)
	Status(context.Context, string) (commandcli.Result, error)
}

var commandsCmd = &cobra.Command{
	Use:   "commands",
	Short: "Lista e executa comandos do Assistente",
}

var (
	commandsListJSON     bool
	commandsDescribeJSON bool
)

var commandsListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lista os comandos disponíveis em JSON",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !commandsListJSON {
			return errors.New("commands list suporta somente saída JSON")
		}
		backend, err := resolveCommandCLI()
		if err != nil {
			return err
		}
		return runCommandsList(cmd.Context(), backend, cmd.OutOrStdout())
	},
}

var commandsDescribeCmd = &cobra.Command{
	Use:   "describe <ID>",
	Short: "Descreve um comando em JSON",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !commandsDescribeJSON {
			return errors.New("commands describe suporta somente saída JSON")
		}
		backend, err := resolveCommandCLI()
		if err != nil {
			return err
		}
		return runCommandsDescribe(cmd.Context(), backend, cmd.OutOrStdout(), args[0])
	},
}

var (
	commandsExecuteArguments string
	commandsRetryArguments   string
	commandsRetryRequestID   string
	commandsStatusRequestID  string
)

var commandsExecuteCmd = &cobra.Command{
	Use:   "execute <ID> --arguments JSON",
	Short: "Executa um comando",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		arguments, err := parseCommandArguments(commandsExecuteArguments)
		if err != nil {
			return err
		}
		backend, err := resolveCommandCLI()
		if err != nil {
			return err
		}
		return runCommandsExecute(cmd.Context(), backend, cmd.OutOrStdout(), args[0], arguments)
	},
}

var commandsRetryCmd = &cobra.Command{
	Use:   "retry <ID> --request-id UUID --arguments JSON",
	Short: "Repete uma solicitação de comando",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateCommandRequestID(commandsRetryRequestID); err != nil {
			return err
		}
		arguments, err := parseCommandArguments(commandsRetryArguments)
		if err != nil {
			return err
		}
		backend, err := resolveCommandCLI()
		if err != nil {
			return err
		}
		return runCommandsRetry(cmd.Context(), backend, cmd.OutOrStdout(), args[0], commandsRetryRequestID, arguments)
	},
}

var commandsStatusCmd = &cobra.Command{
	Use:   "status --request-id UUID",
	Short: "Consulta o estado de uma solicitação",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateCommandRequestID(commandsStatusRequestID); err != nil {
			return err
		}
		backend, err := resolveCommandCLI()
		if err != nil {
			return err
		}
		return runCommandsStatus(cmd.Context(), backend, cmd.OutOrStdout(), commandsStatusRequestID)
	},
}

func init() {
	commandsListCmd.Flags().BoolVar(&commandsListJSON, "json", true, "Usa saída JSON")
	commandsDescribeCmd.Flags().BoolVar(&commandsDescribeJSON, "json", true, "Usa saída JSON")
	commandsExecuteCmd.Flags().StringVar(&commandsExecuteArguments, "arguments", "", "Argumentos JSON do comando")
	commandsExecuteCmd.Flags().StringVar(&commandsExecuteArguments, "args", "", "Alias de --arguments")
	commandsRetryCmd.Flags().StringVar(&commandsRetryArguments, "arguments", "", "Argumentos JSON do comando")
	commandsRetryCmd.Flags().StringVar(&commandsRetryArguments, "args", "", "Alias de --arguments")
	commandsRetryCmd.Flags().StringVar(&commandsRetryRequestID, "request-id", "", "ID UUIDv7 da solicitação")
	commandsStatusCmd.Flags().StringVar(&commandsStatusRequestID, "request-id", "", "ID UUIDv7 da solicitação")

	commandsCmd.AddCommand(commandsListCmd)
	commandsCmd.AddCommand(commandsDescribeCmd)
	commandsCmd.AddCommand(commandsExecuteCmd)
	commandsCmd.AddCommand(commandsRetryCmd)
	commandsCmd.AddCommand(commandsStatusCmd)
}

func resolveCommandCLI() (commandCLIBackend, error) {
	if rootApp == nil {
		return nil, errors.New("aplicação não inicializada")
	}
	backend, err := app.NewCommandCLI(rootApp)
	if err != nil {
		return nil, fmt.Errorf("falha ao preparar CLI de comandos: %w", err)
	}
	return backend, nil
}

func runCommandsList(ctx context.Context, backend commandCLIBackend, out io.Writer) error {
	descriptions, err := backend.List(ctx, commandCLILocale)
	if err != nil {
		return err
	}
	if descriptions == nil {
		descriptions = []commandcli.Description{}
	}
	return encodeCommandsJSON(out, descriptions)
}

func runCommandsDescribe(ctx context.Context, backend commandCLIBackend, out io.Writer, id string) error {
	description, err := backend.Describe(ctx, id, commandCLILocale)
	if err != nil {
		return err
	}
	return encodeCommandsJSON(out, description)
}

func runCommandsExecute(ctx context.Context, backend commandCLIBackend, out io.Writer, id string, arguments json.RawMessage) error {
	result, err := backend.Execute(ctx, commandcli.Request{
		CommandID: id,
		Arguments: arguments,
	})
	return finishCommandResult(out, result, err)
}

func runCommandsRetry(ctx context.Context, backend commandCLIBackend, out io.Writer, id, requestID string, arguments json.RawMessage) error {
	if err := validateCommandRequestID(requestID); err != nil {
		return err
	}
	result, err := backend.Retry(ctx, commandcli.Request{
		CommandID: id,
		Arguments: arguments,
		RequestID: requestID,
	})
	return finishCommandResult(out, result, err)
}

func runCommandsStatus(ctx context.Context, backend commandCLIBackend, out io.Writer, requestID string) error {
	if err := validateCommandRequestID(requestID); err != nil {
		return err
	}
	result, err := backend.Status(ctx, requestID)
	return finishCommandResult(out, result, err)
}

func parseCommandArguments(value string) (json.RawMessage, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("--arguments é obrigatório e deve conter JSON válido")
	}
	raw := json.RawMessage(value)
	if !json.Valid(raw) {
		return nil, errors.New("--arguments deve conter JSON válido")
	}
	return raw, nil
}

func validateCommandRequestID(value string) error {
	id, err := uuid.Parse(value)
	if err != nil || id.Version() != 7 || id.Variant() != uuid.RFC4122 || id.String() != value {
		return fmt.Errorf("request-id deve ser um UUIDv7 canônico: %q", value)
	}
	return nil
}

func finishCommandResult(out io.Writer, result commandcli.Result, backendErr error) error {
	if backendErr == nil {
		return encodeCommandsJSON(out, result)
	}
	if !hasCommandResult(result) {
		return backendErr
	}
	if err := encodeCommandsJSON(out, result); err != nil {
		return errors.Join(backendErr, err)
	}
	return backendErr
}

func hasCommandResult(result commandcli.Result) bool {
	return result.RequestID != "" || result.Status != "" || result.ResultSummary != nil || result.ErrorCode != nil || result.Output != nil
}

func encodeCommandsJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}
