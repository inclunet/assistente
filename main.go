package main

import (
	"embed"
	"flag"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Parse command line flags
	workdir := flag.String("workdir", "", "Diretório de trabalho inicial para o File Agent")
	flag.Parse()

	// Se --workdir não foi passado, tenta usar o diretório atual do terminal
	initialWorkDir := *workdir
	if initialWorkDir == "" {
		// Usa o diretório de onde o executável foi chamado
		if cwd, err := os.Getwd(); err == nil {
			initialWorkDir = cwd
		}
	} else {
		// Resolve para caminho absoluto
		if abs, err := filepath.Abs(initialWorkDir); err == nil {
			initialWorkDir = abs
		}
	}

	// Create an instance of the app structure
	app := NewApp()
	app.InitialWorkDir = initialWorkDir

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "assistente",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
