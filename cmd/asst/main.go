package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	cliadapter "assistente/adapters/cli"
	"assistente/internal/app"
	"assistente/internal/database"

	"github.com/spf13/cobra"
	gormlogger "gorm.io/gorm/logger"
)

// AppVersion é injetado via ldflags no build.
var AppVersion = "dev"

var (
	verbose    bool
	rootApp    *app.App
	cliEmitter *cliadapter.EmitterAdapter
	rootCancel context.CancelFunc
	rootSigCh  chan os.Signal
)

var rootCmd = &cobra.Command{
	Use:   "asst",
	Short: "Assistente pessoal via terminal",
	Long:  "Interface CLI para o assistente pessoal — chat com LLMs, gerenciamento de perfis e configurações.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Comandos anotados com skipInit não precisam do app (version, completion)
		if cmd.Annotations["skipInit"] == "true" {
			return nil
		}

		// Inicializa o app com adapters CLI
		ctx, cancel := context.WithCancel(context.Background())
		rootCancel = cancel

		// Cancela o contexto apenas em SIGTERM.
		// SIGINT é tratado localmente pelo chat (barge-in) para não encerrar o REPL.
		rootSigCh = make(chan os.Signal, 1)
		signal.Notify(rootSigCh, syscall.SIGTERM)
		go func() {
			select {
			case <-rootSigCh:
				cancel()
			case <-ctx.Done():
			}
		}()

		if verbose {
			enableVerboseLogs(os.Stderr)
		}

		rootApp = app.NewApp()

		cliEmitter = cliadapter.NewEmitterAdapter(
			cliadapter.WithVerbose(verbose),
		)

		// O nível de log do GORM precisa ser configurado antes do startup,
		// pois o database.Init() ocorre durante a inicialização do app.
		if !verbose {
			database.SetLogLevel(gormlogger.Silent)
		}

		if err := rootApp.StartupWithAdapters(ctx,
			cliEmitter,
			cliadapter.WindowAdapter{},
			cliadapter.DialogAdapter{},
		); err != nil {
			signal.Stop(rootSigCh)
			cancel()
			rootSigCh = nil
			rootCancel = nil
			rootApp = nil
			cliEmitter = nil
			return fmt.Errorf("falha ao inicializar aplicação: %w", err)
		}

		// Silencia logs padrão após startup bem-sucedido
		// para manter visíveis eventuais erros de inicialização.
		if !verbose {
			silenceDefaultLogs()
		}

		return nil
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		if rootApp != nil {
			rootApp.Shutdown()
		}
		if rootSigCh != nil {
			signal.Stop(rootSigCh)
		}
		if rootCancel != nil {
			rootCancel()
		}
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Exibe eventos detalhados no stderr")

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(chatCmd)
	rootCmd.AddCommand(profilesCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(completionCmd)
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(providersCmd)
	rootCmd.AddCommand(credentialsCmd)
	rootCmd.AddCommand(mcpCmd)
	rootCmd.AddCommand(historyCmd)
	rootCmd.AddCommand(dataCmd)
	rootCmd.AddCommand(toolsCmd)
}

func silenceDefaultLogs() {
	log.SetOutput(io.Discard)
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func enableVerboseLogs(w io.Writer) {
	if w == nil {
		w = os.Stderr
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug})))
}

var versionCmd = &cobra.Command{
	Use:         "version",
	Short:       "Exibe a versão do assistente",
	Annotations: map[string]string{"skipInit": "true"},
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("asst %s\n", AppVersion)
	},
}

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Gera script de auto-complete para o shell",
	Long: `Gera script de auto-complete para o shell especificado.

Exemplos:
  # Bash (adicione ao ~/.bashrc):
  asst completion bash > /etc/bash_completion.d/asst

  # Zsh (adicione ao ~/.zshrc):
  asst completion zsh > "${fpath[1]}/_asst"

  # Fish:
  asst completion fish > ~/.config/fish/completions/asst.fish

  # PowerShell (adicione ao $PROFILE):
  asst completion powershell | Out-String | Invoke-Expression`,
	Annotations:           map[string]string{"skipInit": "true"},
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.GenFishCompletion(os.Stdout, true)
		case "powershell":
			return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
		default:
			return fmt.Errorf("shell não suportado: %s", args[0])
		}
	},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
