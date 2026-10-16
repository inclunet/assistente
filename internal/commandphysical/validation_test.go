package commandphysical

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandforeground"
	"assistente/internal/ossession"
)

type foregroundStub struct {
	snapshot commandforeground.Snapshot
	err      error
}

func (f foregroundStub) Capture(context.Context) (commandforeground.Snapshot, error) {
	return f.snapshot, f.err
}

func TestValidateReadyWhenAllPhysicalEvidenceExists(t *testing.T) {
	report := Validate(context.Background(), Config{
		Foreground:        foregroundStub{snapshot: validForegroundSnapshot()},
		CaptureForeground: true,
		HotkeySupported:   func() bool { return true },
		OSSessionProbe: func(context.Context) (ossession.State, error) {
			return ossession.State{Known: true, Locked: false}, nil
		},
		StreamDeckModel:  "Stream Deck",
		StreamDeckSerial: "AL28K2C54852",
	})
	if !report.Ready || len(report.Reasons) != 0 {
		t.Fatalf("report deveria estar pronto: %+v", report)
	}
	if !report.Env.ForegroundNative || !report.Env.HotkeyGlobal || !report.Env.OSSessionObservable || !report.Env.StreamDeckPhysical {
		t.Fatalf("env incompleto: %+v", report.Env)
	}
}

func TestValidateReportsEveryMissingPhysicalEvidence(t *testing.T) {
	report := Validate(context.Background(), Config{
		Foreground:        foregroundStub{err: errors.New("foreground unavailable")},
		CaptureForeground: true,
		HotkeySupported:   func() bool { return false },
		OSSessionProbe: func(context.Context) (ossession.State, error) {
			return ossession.State{Known: false, Locked: true}, nil
		},
	})
	want := map[string]bool{
		"keyboard-global-unverified":     true,
		"foreground-native-unverified":   true,
		"os-session-unverified":          true,
		"streamdeck-physical-unverified": true,
	}
	if report.Ready {
		t.Fatalf("report não deveria estar pronto")
	}
	for _, reason := range report.Reasons {
		delete(want, reason)
	}
	if len(want) != 0 {
		t.Fatalf("faltaram motivos: %+v em %+v", want, report.Reasons)
	}
}

func validForegroundSnapshot() commandforeground.Snapshot {
	return commandforeground.Snapshot{
		Version:    "foreground-version",
		CapturedAt: time.Now(),
		Summary: commandforeground.Summary{
			Executable:      "powershell.exe",
			WindowClass:     "ConsoleWindowClass",
			ProviderVersion: commandforeground.ProviderVersion,
		},
	}
}
