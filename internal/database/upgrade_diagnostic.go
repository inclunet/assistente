package database

import (
	"fmt"

	"gorm.io/gorm"
)

// UpgradeDiagnostic é um retrato local e sem PII do estado das migrações.
// Ele contém somente números e identificadores estáveis definidos no código;
// não inclui caminhos, IDs de usuário nem conteúdo persistido.
type UpgradeDiagnostic struct {
	SchemaVersion   int      `json:"schemaVersion"`
	LatestVersion   int      `json:"latestVersion"`
	AppliedCount    int      `json:"appliedCount"`
	PendingVersions []int    `json:"pendingVersions"`
	PendingNames    []string `json:"pendingNames"`
}

func GetUpgradeDiagnostic() (UpgradeDiagnostic, error) {
	return buildUpgradeDiagnostic(db)
}

func buildUpgradeDiagnostic(database *gorm.DB) (UpgradeDiagnostic, error) {
	diagnostic := UpgradeDiagnostic{
		PendingVersions: []int{},
		PendingNames:    []string{},
	}
	if len(schemaMigrations) > 0 {
		diagnostic.LatestVersion = schemaMigrations[len(schemaMigrations)-1].Version
	}
	if database == nil {
		return diagnostic, fmt.Errorf("banco de dados não inicializado")
	}
	if !database.Migrator().HasTable("schema_migrations") {
		for _, migration := range schemaMigrations {
			diagnostic.PendingVersions = append(diagnostic.PendingVersions, migration.Version)
			diagnostic.PendingNames = append(diagnostic.PendingNames, migration.Name)
		}
		return diagnostic, nil
	}

	applied, err := appliedMigrationVersions(database)
	if err != nil {
		return diagnostic, err
	}
	diagnostic.AppliedCount = len(applied)
	var userVersion int
	if err := database.Raw("PRAGMA user_version").Scan(&userVersion).Error; err != nil {
		return diagnostic, fmt.Errorf("ler PRAGMA user_version: %w", err)
	}
	diagnostic.SchemaVersion = userVersion
	for _, migration := range schemaMigrations {
		if applied[migration.Version] {
			continue
		}
		diagnostic.PendingVersions = append(diagnostic.PendingVersions, migration.Version)
		diagnostic.PendingNames = append(diagnostic.PendingNames, migration.Name)
	}
	return diagnostic, nil
}
