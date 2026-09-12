package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"assistente/internal/logging"
)

type startupMessages struct {
	appTitle        string
	logPathRequired string
	logFlagRepeated string
	logOpen         string
	logConfigure    string
	logClose        string
	appStartup      string
	appRun          string
}

var startupCatalog = map[string]startupMessages{
	"pt": {
		appTitle:        "Assistente IA",
		logPathRequired: "A opção --log-file requer um caminho.",
		logFlagRepeated: "A opção --log-file foi informada mais de uma vez.",
		logOpen:         "Não foi possível abrir o arquivo de log %q: %v",
		logConfigure:    "Erro ao configurar logs: %v",
		logClose:        "Erro ao fechar arquivo de log %q: %v",
		appStartup:      "Falha ao inicializar aplicação: %v",
		appRun:          "Erro: %v",
	},
	"en": {
		appTitle:        "AI Assistant",
		logPathRequired: "The --log-file option requires a path.",
		logFlagRepeated: "The --log-file option was provided more than once.",
		logOpen:         "Could not open log file %q: %v",
		logConfigure:    "Error configuring logs: %v",
		logClose:        "Error closing log file %q: %v",
		appStartup:      "Failed to start application: %v",
		appRun:          "Error: %v",
	},
	"es": {
		appTitle:        "Asistente IA",
		logPathRequired: "La opción --log-file requiere una ruta.",
		logFlagRepeated: "La opción --log-file se proporcionó más de una vez.",
		logOpen:         "No se pudo abrir el archivo de registro %q: %v",
		logConfigure:    "Error al configurar los registros: %v",
		logClose:        "Error al cerrar el archivo de registro %q: %v",
		appStartup:      "No se pudo iniciar la aplicación: %v",
		appRun:          "Error: %v",
	},
}

var startupLocaleProvider = detectStartupLocale

func startupLogConfigurationError(err error) string {
	messages := currentStartupMessages()
	switch {
	case errors.Is(err, logging.ErrLogFilePathRequired):
		return messages.logPathRequired
	case errors.Is(err, logging.ErrLogFileRepeated):
		return messages.logFlagRepeated
	}
	var openError *logging.FileOpenError
	if errors.As(err, &openError) {
		return fmt.Sprintf(messages.logOpen, openError.Path, openError.Err)
	}
	return fmt.Sprintf(messages.logConfigure, err)
}

func startupLogCloseError(path string, err error) string {
	return fmt.Sprintf(currentStartupMessages().logClose, path, err)
}

func startupApplicationError(err error) string {
	return fmt.Sprintf(currentStartupMessages().appStartup, err)
}

func startupRunError(err error) string {
	return fmt.Sprintf(currentStartupMessages().appRun, err)
}

func startupDialogTitle() string {
	return currentStartupMessages().appTitle
}

func currentStartupMessages() startupMessages {
	locale := strings.ToLower(startupLocaleProvider())
	switch {
	case strings.HasPrefix(locale, "pt"):
		return startupCatalog["pt"]
	case strings.HasPrefix(locale, "es"):
		return startupCatalog["es"]
	default:
		return startupCatalog["en"]
	}
}

func detectStartupLocale() string {
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return platformStartupLocale()
}
