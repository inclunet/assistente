package commandphysical

import (
	"context"
	"os"
	"testing"
	"time"

	"assistente/internal/commandforeground"
	"assistente/internal/hotkey"
	"assistente/internal/ossession"
)

func TestManualPhysicalEnvironment(t *testing.T) {
	if os.Getenv("ASSISTENTE_PHYSICAL_MANUAL") != "1" {
		t.Skip("defina ASSISTENTE_PHYSICAL_MANUAL=1 para validar foreground/hotkey/ambiente físico")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	foreground := commandforeground.NewNative()
	if snapshot, err := foreground.Capture(ctx); err != nil {
		t.Logf("foreground nativo falhou: %v", err)
	} else {
		t.Logf("foreground nativo: executable=%s class=%s version=%s", snapshot.Summary.Executable, snapshot.Summary.WindowClass, snapshot.Version)
	}
	report := Validate(ctx, Config{
		Foreground:        foreground,
		CaptureForeground: true,
		HotkeySupported:   hotkey.IsSupported,
		OSSessionProbe:    ossession.Probe,
		StreamDeckModel:   os.Getenv("ASSISTENTE_STREAMDECK_MODEL"),
		StreamDeckSerial:  os.Getenv("ASSISTENTE_STREAMDECK_SERIAL"),
	})
	if !report.Ready {
		t.Fatalf("ambiente físico incompleto: %+v", report)
	}
	t.Logf("ambiente físico validado: %+v", report.Env)
}
