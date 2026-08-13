package main

import (
	"assistente/internal/logging"
	"context"
	"embed"
	"time"

	"assistente/adapters/wails"
	application "assistente/internal/app"
	"assistente/internal/wailsapi"

	wailslib "github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	a := application.NewApp()

	err := wailslib.Run(&options.App{
		Title:  "assistente",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup: func(ctx context.Context) {
			if err := a.StartupWithAdapters(ctx,
				wails.NewEmitterAdapter(ctx),
				wails.NewWindowAdapter(ctx),
				wails.NewDialogAdapter(ctx),
			); err != nil {
				logging.Fatalf(ctx, "main", "Falha ao inicializar aplicação: %v", err)
			}
			// Restaura foco da janela (resolve bug do Wails no Windows)
			go func() {
				timer := time.NewTimer(400 * time.Millisecond)
				defer timer.Stop()
				select {
				case <-timer.C:
					a.ShowWindow()
				case <-ctx.Done():
					return
				}
			}()
		},
		OnShutdown: func(_ context.Context) {
			a.Shutdown()
		},
		// AEP-0088: multi-bind — App (ciclo de vida + domínios ainda não
		// migrados) e Probe (spike Fase 1). Domínios seguintes entram aqui.
		Bind: []interface{}{
			a,
			wailsapi.NewProbe(),
		},
		Debug: options.Debug{
			OpenInspectorOnStartup: false,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
