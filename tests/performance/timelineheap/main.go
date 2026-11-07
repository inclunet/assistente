//go:build windows && timeline_heap

// Hospeda a fixture de medição no mesmo runtime Wails/WebView2 do aplicativo.
package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

func main() {
	if len(os.Args) != 3 || !filepath.IsAbs(os.Args[1]) || !filepath.IsAbs(os.Args[2]) {
		log.Fatal("uso: timeline-heap <assets absolutos> <perfil WebView absoluto>")
	}
	if err := wails.Run(&options.App{
		Title: "Timeline heap", Width: 1000, Height: 800, StartHidden: true,
		AssetServer: &assetserver.Options{Assets: os.DirFS(os.Args[1])},
		Windows:     &windows.Options{WebviewUserDataPath: os.Args[2]},
	}); err != nil {
		log.Fatal(err)
	}
}
