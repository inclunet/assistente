package database

// PluginProject guarda a composição autoral, independente de instalações e exportações.
type PluginProject struct {
	ID        string `gorm:"primaryKey"`
	UserID    string `gorm:"not null;index"`
	Revision  int    `gorm:"not null"`
	Selection []byte `gorm:"not null"`
}
