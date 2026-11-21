package database

// PluginInstallation identifies one installed package per account and origin.
// A published revision is inert until its components are activated.
type PluginInstallation struct {
	UUIDModel
	UserID  string `gorm:"not null;uniqueIndex:plugin_owner_origin_name,priority:1" json:"-"`
	Origin  string `gorm:"not null;uniqueIndex:plugin_owner_origin_name,priority:2" json:"origin"`
	Name    string `gorm:"not null;uniqueIndex:plugin_owner_origin_name,priority:3" json:"name"`
	Digest  string `gorm:"not null" json:"digest"`
	Version string `json:"version"`
}

type PluginRevision struct {
	InstallationID string `gorm:"primaryKey" json:"-"`
	Digest         string `gorm:"primaryKey" json:"digest"`
	// Captured package files, keyed by package-relative path. Encoding []byte
	// as JSON preserves binary files without deserializing third-party objects.
	Files []byte `gorm:"not null" json:"-"`
}

// PluginSkillActivation belongs to the exact reviewed package revision.
type PluginSkillActivation struct {
	InstallationID string `gorm:"primaryKey"`
	ComponentKey   string `gorm:"primaryKey"`
	Digest         string `gorm:"not null"`
	Slug           string `gorm:"uniqueIndex;not null"`
	Definition     string `gorm:"not null" json:"-"`
}
