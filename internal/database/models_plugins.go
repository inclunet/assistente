package database

// PluginInstallation identifies one installed package per account and origin.
// A published revision is inert until its components are activated.
type PluginInstallation struct {
	DiscoveryRepositoryID string `json:"discoveryRepositoryId,omitempty"`
	UUIDModel
	Disabled bool   `gorm:"not null;default:false" json:"disabled"`
	UserID   string `gorm:"not null;uniqueIndex:plugin_owner_origin_name,priority:1" json:"-"`
	Origin   string `gorm:"not null;uniqueIndex:plugin_owner_origin_name,priority:2" json:"origin"`
	Name     string `gorm:"not null;uniqueIndex:plugin_owner_origin_name,priority:3" json:"name"`
	Digest   string `gorm:"not null" json:"digest"`
	Version  string `json:"version"`
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
	Generation     string `gorm:"not null;default:''" json:"-"`
	PackageFormat  string `json:"-"`
	InstallationID string `gorm:"primaryKey"`
	ComponentKey   string `gorm:"primaryKey"`
	Digest         string `gorm:"not null"`
	Slug           string `gorm:"uniqueIndex;not null"`
	Definition     string `gorm:"not null" json:"-"`
}

type PluginMCPBinding struct {
	InstallationID string `gorm:"primaryKey"`
	ComponentKey   string `gorm:"primaryKey"`
	Digest         string `gorm:"not null"`
	ServerID       string `gorm:"uniqueIndex;not null"`
	Slug           string `gorm:"not null"`
	ConfigDigest   string `gorm:"not null"`
}

type PluginRepository struct {
	CredentialRef string `json:"-"`
	RootID        string `gorm:"not null" json:"-"`
	UUIDModel
	UserID   string `gorm:"not null;uniqueIndex:plugin_repository_owner,priority:1" json:"-"`
	Origin   string `gorm:"not null;uniqueIndex:plugin_repository_owner,priority:2" json:"origin"`
	Indices  []byte `gorm:"not null" json:"-"`
	Revision string `gorm:"not null" json:"revision"`
}
