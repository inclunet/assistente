package main

import (
	"bytes"
	"context"
	"errors"
	"log"
	"log/slog"
	"testing"

	"github.com/spf13/cobra"
)

func TestSilenceDefaultLogsAlsoSilencesSlog(t *testing.T) {
	var stdLogBuf, slogBuf bytes.Buffer
	prevLogOutput := log.Writer()
	prevSlog := slog.Default()
	t.Cleanup(func() {
		log.SetOutput(prevLogOutput)
		slog.SetDefault(prevSlog)
	})

	log.SetOutput(&stdLogBuf)
	slog.SetDefault(slog.New(slog.NewTextHandler(&slogBuf, nil)))

	silenceDefaultLogs()

	log.Print("standard log should be discarded")
	slog.Info("structured log should be discarded")

	if stdLogBuf.Len() > 0 {
		t.Fatalf("standard log was not silenced: %q", stdLogBuf.String())
	}
	if slogBuf.Len() > 0 {
		t.Fatalf("slog was not silenced: %q", slogBuf.String())
	}
}

func TestExecuteCLICleansUpAfterCommandFailure(t *testing.T) {
	previous := rootCancel
	t.Cleanup(func() { rootCancel = previous })
	ctx, cancel := context.WithCancel(context.Background())
	rootCancel = cancel
	want := errors.New("command denied")
	cmd := &cobra.Command{Use: "test", SilenceErrors: true, SilenceUsage: true, RunE: func(*cobra.Command, []string) error { return want }}
	cmd.SetArgs([]string{})
	if err := executeCLI(cmd); !errors.Is(err, want) {
		t.Fatalf("error = %v", err)
	}
	if ctx.Err() == nil || rootCancel != nil {
		t.Fatal("failed command left CLI context alive")
	}
	cleanupRootApp() // O PostRun e o defer podem chamar a mesma limpeza.
}

func TestEnableVerboseLogsEnablesSlogDebug(t *testing.T) {
	var slogBuf bytes.Buffer
	prevSlog := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(prevSlog)
	})

	enableVerboseLogs(&slogBuf)

	slog.Debug("structured debug log")

	if slogBuf.Len() == 0 {
		t.Fatal("slog debug log was not emitted")
	}
}
