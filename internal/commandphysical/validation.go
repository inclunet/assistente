// Package commandphysical consolida evidências de ambiente físico para a AEP-0103.
//
// Ele não registra hotkeys, não abre dispositivos e não executa comandos de
// produto. O objetivo é transformar validações dispersas de SO/hardware em um
// relatório pequeno, auditável e fail-closed.
package commandphysical

import (
	"context"
	"errors"
	"runtime"
	"strings"

	"assistente/internal/commandforeground"
	"assistente/internal/ossession"
)

var (
	ErrInvalidValidation = errors.New("validação física inválida")
)

type Environment struct {
	OS                  string
	HotkeyGlobal        bool
	ForegroundNative    bool
	OSSessionObservable bool
	StreamDeckPhysical  bool
	StreamDeckModel     string
	StreamDeckSerial    string
}

type Report struct {
	Ready   bool
	Reasons []string
	Env     Environment
}

type Config struct {
	Foreground        commandforeground.Reader
	CaptureForeground bool
	HotkeySupported   func() bool
	OSSessionProbe    func(context.Context) (ossession.State, error)
	StreamDeckModel   string
	StreamDeckSerial  string
}

func Validate(ctx context.Context, config Config) Report {
	env := Environment{OS: runtime.GOOS}
	report := Report{Env: env}
	if ctx == nil {
		report.Reasons = append(report.Reasons, "context-missing")
		return finalize(report)
	}
	if config.HotkeySupported != nil && config.HotkeySupported() {
		report.Env.HotkeyGlobal = true
	} else {
		report.Reasons = append(report.Reasons, "keyboard-global-unverified")
	}
	if config.CaptureForeground {
		if config.Foreground == nil {
			report.Reasons = append(report.Reasons, "foreground-reader-missing")
		} else if snapshot, err := config.Foreground.Capture(ctx); err != nil || strings.TrimSpace(snapshot.Version) == "" || snapshot.Summary.Executable == "" || snapshot.Summary.WindowClass == "" {
			report.Reasons = append(report.Reasons, "foreground-native-unverified")
		} else {
			report.Env.ForegroundNative = true
		}
	} else {
		report.Reasons = append(report.Reasons, "foreground-native-unverified")
	}
	if config.OSSessionProbe != nil {
		if state, err := config.OSSessionProbe(ctx); err == nil && state.Known && !state.Locked {
			report.Env.OSSessionObservable = true
		} else {
			report.Reasons = append(report.Reasons, "os-session-unverified")
		}
	} else {
		// Sem prova autoritativa, I13.6 só pode considerar fail-closed conhecido
		// por testes de ossession, não uma validação física de ambiente.
		report.Reasons = append(report.Reasons, "os-session-unverified")
	}
	if strings.TrimSpace(config.StreamDeckModel) != "" && strings.TrimSpace(config.StreamDeckSerial) != "" {
		report.Env.StreamDeckPhysical = true
		report.Env.StreamDeckModel = config.StreamDeckModel
		report.Env.StreamDeckSerial = config.StreamDeckSerial
	} else {
		report.Reasons = append(report.Reasons, "streamdeck-physical-unverified")
	}
	return finalize(report)
}

func finalize(report Report) Report {
	report.Reasons = unique(report.Reasons)
	report.Ready = len(report.Reasons) == 0
	return report
}

func unique(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
