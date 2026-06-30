package main

import (
	"bytes"
	"log"
	"log/slog"
	"testing"
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
