package main

import (
	"context"
	"fmt"
	"io"
	"log"
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
	verbose     bool
	rootApp     *app.App
	cliEmitter  *cliadapter.EmitterAdapter
)

var rootCmd = &cobra.Command{
	Use:   "assistente",
	Short: "Assistente pessoal via terminal",
	Long:  "Interface CLI para o assistente pessoal — chat com LLMs, gerenciamento de perfis e configurações.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Silencia logs de startup quando não está em modo verbose
		if !verbose {
			log.SetOutput(io.Discard)
			database.SetLogLevel(gormlogger.Silent)
		}

		// Inicializa o app com adapters CLI
		ctx, cancel := context.WithCancel(context.Background())

		// Cancela o contexto em SIGINT/SIGTERM
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			cancel()
		}()

		rootApp = app.NewApp()

		cliEmitter = cliadapter.NewEmitterAdapter(
			cliadapter.WithVerbose(verbose),
		)

		rootApp.StartupWithAdapters(ctx,
			cliEmitter,
			cliadapter.WindowAdapter{},
			cliadapter.DialogAdapter{},
		)

		return nil
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		if rootApp != nil {
			rootApp.Shutdown()
		}
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Exibe eventos detalhados no stderr")

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(chatCmd)
	rootCmd.AddCommand(profilesCmd)
	rootCmd.AddCommand(configCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Exibe a versão do assistente",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("assistente %s\n", AppVersion)
	},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
