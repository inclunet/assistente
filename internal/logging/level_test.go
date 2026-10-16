package logging

import (
	"bytes"
	"context"
	"errors"
	"log"
	"log/slog"
	"strings"
	"testing"
)

func TestParseToolLedgerLogLevelArgs(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantLevel string
		wantArgs  []string
		wantError error
	}{
		{name: "default", args: []string{"--debug"}, wantLevel: "info", wantArgs: []string{"--debug"}},
		{name: "trace separado", args: []string{"--tool-ledger-log-level", "trace", "--debug"}, wantLevel: "trace", wantArgs: []string{"--debug"}},
		{name: "debug igual", args: []string{"--tool-ledger-log-level=DEBUG"}, wantLevel: "debug", wantArgs: []string{}},
		{name: "inválido", args: []string{"--tool-ledger-log-level=noisy"}, wantError: ErrLogLevelRequired},
		{name: "repetido", args: []string{"--tool-ledger-log-level=info", "--tool-ledger-log-level=trace"}, wantError: ErrLogLevelRepeated},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			level, args, err := ParseToolLedgerLogLevelArgs(test.args)
			if test.wantError != nil {
				if !errors.Is(err, test.wantError) {
					t.Fatalf("erro = %v, esperado %v", err, test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseToolLedgerLogLevelArgs: %v", err)
			}
			if level != test.wantLevel || strings.Join(args, "\x00") != strings.Join(test.wantArgs, "\x00") {
				t.Fatalf("resultado = (%q, %#v), esperado (%q, %#v)", level, args, test.wantLevel, test.wantArgs)
			}
		})
	}
}

func TestConfigureToolLedgerLevelHabilitaTraceSomenteNoLedger(t *testing.T) {
	previousLogger := slog.Default()
	previousWriter := log.Writer()
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
		log.SetOutput(previousWriter)
	})

	var output bytes.Buffer
	log.SetOutput(&output)
	if err := ConfigureToolLedgerLevel("trace"); err != nil {
		t.Fatalf("ConfigureToolLedgerLevel: %v", err)
	}
	Tracef(context.Background(), "toolinvocations.backfill", "batch rows=%d bytes=%d", 2, 128)
	Debugf(context.Background(), "unrelated.component", "segredo=%s", "nao-pode-vazar")

	got := output.String()
	if !strings.Contains(got, "batch rows=2 bytes=128") || !strings.Contains(got, "component=toolinvocations.backfill") {
		t.Fatalf("trace ausente: %q", got)
	}
	if strings.Contains(got, "nao-pode-vazar") {
		t.Fatalf("debug de componente não relacionado vazou: %q", got)
	}
}
