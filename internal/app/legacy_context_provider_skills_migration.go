package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"assistente/internal/logging"
)

var legacyContextProviderSkillSlugs = []string{"memory", "workspace"}

const (
	legacyContextProviderCleanupMarker = ".legacy-context-providers-cleaned"

	legacyContextProviderMarkerFormatVersion = 1

	// A ausência do marker continuará suportada por pelo menos duas releases
	// posteriores à release que introduzir seu formato versionado. Remoção após
	// essa janela também depende da política geral da issue #676 e de fixtures
	// cobrindo a versão mínima suportada.
	legacyContextProviderMinimumMarkerlessReleaseWindow = 2
)

type legacyContextProviderMigrationStatus string

const (
	legacyContextProviderMigrationCompleted        legacyContextProviderMigrationStatus = "completed"
	legacyContextProviderMigrationAlreadyCompleted legacyContextProviderMigrationStatus = "already_completed"
	legacyContextProviderMigrationPartialFailure   legacyContextProviderMigrationStatus = "partial_failure"
	legacyContextProviderMigrationMarkerCheckError legacyContextProviderMigrationStatus = "marker_check_error"
	legacyContextProviderMigrationMarkerWriteError legacyContextProviderMigrationStatus = "marker_write_error"
)

type legacyContextProviderMarker struct {
	FormatVersion int       `json:"formatVersion"`
	AppVersion    string    `json:"appVersion"`
	CompletedAt   time.Time `json:"completedAt"`
}

// legacyContextProviderMigrationResult é o diagnóstico local, sem conteúdo de
// skills nem outros dados pessoais, produzido em toda tentativa de migração.
type legacyContextProviderMigrationResult struct {
	Status                  legacyContextProviderMigrationStatus
	MarkerFormat            string
	AppVersion              string
	MarkerAppVersion        string
	BackedUpSlugs           []string
	Failures                []string
	MarkerlessReleaseWindow int
}

type legacyContextProviderSkillsMigrator struct {
	homeDir    string
	appVersion string
	now        func() time.Time
	stat       func(string) (os.FileInfo, error)
	readFile   func(string) ([]byte, error)
	mkdirAll   func(string, os.FileMode) error
	rename     func(string, string) error
	writeFile  func(string, []byte, os.FileMode) error
}

func newLegacyContextProviderSkillsMigrator(homeDir, appVersion string) *legacyContextProviderSkillsMigrator {
	return &legacyContextProviderSkillsMigrator{
		homeDir:    homeDir,
		appVersion: appVersion,
		now:        time.Now,
		stat:       os.Stat,
		readFile:   os.ReadFile,
		mkdirAll:   os.MkdirAll,
		rename:     os.Rename,
		writeFile:  os.WriteFile,
	}
}

func runLegacyContextProviderSkillsMigration(homeDir, appVersion string) legacyContextProviderMigrationResult {
	result := newLegacyContextProviderSkillsMigrator(homeDir, appVersion).Run()
	ctx := logging.WithAttrs(
		context.Background(),
		slog.String("migration", "legacy_context_provider_skills"),
		slog.String("status", string(result.Status)),
		slog.String("marker_format", result.MarkerFormat),
		slog.String("app_version", result.AppVersion),
		slog.String("marker_app_version", result.MarkerAppVersion),
		slog.Int("backed_up_count", len(result.BackedUpSlugs)),
		slog.Int("failure_count", len(result.Failures)),
		slog.Int("markerless_release_window", result.MarkerlessReleaseWindow),
	)
	logger := logging.Logger(ctx, "app.legacy-skill-migration")
	if len(result.Failures) > 0 {
		logger.Error("Migração de skills antigas incompleta")
	} else {
		logger.Info("Migração de skills antigas verificada")
	}
	return result
}

func (m *legacyContextProviderSkillsMigrator) Run() legacyContextProviderMigrationResult {
	result := legacyContextProviderMigrationResult{
		MarkerFormat:            "absent",
		AppVersion:              m.appVersion,
		MarkerlessReleaseWindow: legacyContextProviderMinimumMarkerlessReleaseWindow,
	}
	markerFile := filepath.Join(m.homeDir, legacyContextProviderCleanupMarker)
	markerExists, err := m.inspectMarker(markerFile, &result)
	if err != nil {
		result.Status = legacyContextProviderMigrationMarkerCheckError
		result.Failures = append(result.Failures, err.Error())
		return result
	}
	if markerExists {
		result.Status = legacyContextProviderMigrationAlreadyCompleted
		return result
	}

	completedAt := m.now()
	backupRoot := filepath.Join(
		m.homeDir,
		".legacy-backup",
		"context-providers-"+completedAt.Format("20060102-150405"),
	)
	for _, slug := range legacyContextProviderSkillSlugs {
		targetDir := filepath.Join(m.homeDir, slug)
		if _, err := m.stat(targetDir); err != nil {
			if !os.IsNotExist(err) {
				result.Failures = append(result.Failures, fmt.Sprintf("verificar skill antiga %s: %v", slug, err))
			}
			continue
		}
		backupDir := filepath.Join(backupRoot, slug)
		if err := m.mkdirAll(filepath.Dir(backupDir), 0755); err != nil {
			result.Failures = append(result.Failures, fmt.Sprintf("criar backup da skill antiga %s: %v", slug, err))
			continue
		}
		if err := m.rename(targetDir, backupDir); err != nil {
			result.Failures = append(result.Failures, fmt.Sprintf("mover skill antiga %s para backup: %v", slug, err))
			continue
		}
		result.BackedUpSlugs = append(result.BackedUpSlugs, slug)
	}
	if len(result.Failures) > 0 {
		result.Status = legacyContextProviderMigrationPartialFailure
		return result
	}

	marker := legacyContextProviderMarker{
		FormatVersion: legacyContextProviderMarkerFormatVersion,
		AppVersion:    m.appVersion,
		CompletedAt:   completedAt,
	}
	data, err := json.Marshal(marker)
	if err != nil {
		// Os campos atuais são sempre serializáveis. Este ramo mantém a falha
		// diagnosticável caso o contrato do marker seja ampliado no futuro.
		result.Status = legacyContextProviderMigrationMarkerWriteError
		result.Failures = append(result.Failures, fmt.Sprintf("serializar marker: %v", err))
		return result
	}
	if err := m.writeFile(markerFile, data, 0644); err != nil {
		result.Status = legacyContextProviderMigrationMarkerWriteError
		result.Failures = append(result.Failures, fmt.Sprintf("gravar marker: %v", err))
		return result
	}
	result.Status = legacyContextProviderMigrationCompleted
	result.MarkerFormat = "versioned"
	result.MarkerAppVersion = m.appVersion
	return result
}

// inspectMarker aceita qualquer marker que o caminho anterior tratava como
// concluído. JSON válido adiciona diagnóstico; timestamp/texto antigo e até
// conteúdo ilegível continuam impedindo uma segunda migração.
func (m *legacyContextProviderSkillsMigrator) inspectMarker(
	markerFile string,
	result *legacyContextProviderMigrationResult,
) (bool, error) {
	if _, err := m.stat(markerFile); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("verificar marker: %w", err)
	}

	data, err := m.readFile(markerFile)
	if err != nil {
		result.MarkerFormat = "unreadable"
		return true, nil
	}
	var marker legacyContextProviderMarker
	if err := json.Unmarshal(data, &marker); err == nil && marker.FormatVersion > 0 {
		result.MarkerFormat = "versioned"
		result.MarkerAppVersion = marker.AppVersion
		return true, nil
	}
	result.MarkerFormat = "legacy"
	return true, nil
}
