package database

import "time"

// DesktopWorkspaceEntry é uma referência local pré-login, não uma autorização
// para o conteúdo das abas. ClientScope identifica o diretório de configuração.
type DesktopWorkspaceEntry struct {
	ClientScope string `gorm:"primaryKey;size:64"`
	WorkspaceID string `gorm:"primaryKey;size:128"`
	Name        string
	Path        string
	LastUsed    time.Time
}

type DesktopWorkspaceState struct {
	ClientScope    string `gorm:"primaryKey;size:64"`
	LastOpened     string
	Revision       uint64
	LegacyImported bool
	LegacyDigest   string
}
