package database

// PluginInstallation identifies one installed package per account and origin.
// A published revision is inert until its components are activated.
type PluginInstallation struct {
	// HookGeneration invalida um aceite apresentado antes de desativar ou substituir a revisão.
	HookGeneration        uint64 `gorm:"not null;default:0" json:"-"`
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
	// Índice derivado do snapshot imutável. NULL identifica revisões anteriores à indexação.
	ProviderRequirements []byte `json:"-"`
	InstallationID       string `gorm:"primaryKey" json:"-"`
	Digest               string `gorm:"primaryKey" json:"digest"`
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

// PluginAgentActivation grants delegation to one reviewed component and revision.
// A new activation has a new generation even if the files are unchanged.
type PluginAgentActivation struct {
	Slug           string `gorm:"uniqueIndex;not null"`
	InstallationID string `gorm:"primaryKey"`
	ComponentKey   string `gorm:"primaryKey"`
	Digest         string `gorm:"not null"`
	Generation     string `gorm:"uniqueIndex;not null"`
}

type PluginProfileActivation struct {
	InstallationID string `gorm:"primaryKey"`
	ComponentKey   string `gorm:"primaryKey"`
	Digest         string `gorm:"not null"`
	Slug           string `gorm:"uniqueIndex;not null"`
	Definition     []byte `gorm:"not null" json:"-"`
}

// PluginListReceipt records an accepted request independently of the created list.
type PluginListReceipt struct {
	InstallationID string             `gorm:"primaryKey"`
	Digest         string             `gorm:"primaryKey"`
	ComponentKey   string             `gorm:"primaryKey"`
	RequestID      string             `gorm:"primaryKey"`
	ListID         string             `gorm:"not null"`
	Installation   PluginInstallation `gorm:"foreignKey:InstallationID;constraint:OnDelete:CASCADE"`
}

// PluginJobCopyReceipt survives deletion of the copy and removal of its plugin.
type PluginJobCopyReceipt struct {
	UserID         string `gorm:"primaryKey"`
	InstallationID string `gorm:"primaryKey"`
	SourceSlug     string `gorm:"primaryKey"`
	RequestID      string `gorm:"primaryKey"`
	CopySlug       string `gorm:"not null"`
}

type PluginProviderBinding struct {
	InstallationID string `gorm:"primaryKey"`
	ComponentKey   string `gorm:"primaryKey"`
	Digest         string `gorm:"not null"`
	ProviderID     string `gorm:"index:idx_plugin_provider_users;uniqueIndex:ux_plugin_provider_source,where:consumer_only=0;not null"`
	ConsumerOnly   bool   `gorm:"not null;default:false"`
}

type PluginMCPBinding struct {
	InstallationID string `gorm:"primaryKey"`
	ComponentKey   string `gorm:"primaryKey"`
	Digest         string `gorm:"not null"`
	ServerID       string `gorm:"index:idx_plugin_mcp_users;uniqueIndex:ux_plugin_mcp_source,where:consumer_only=0;not null"`
	ConsumerOnly   bool   `gorm:"not null;default:false"`
	Slug           string `gorm:"not null"`
	ConfigDigest   string `gorm:"not null"`
}

type PluginRepository struct {
	ArchiveETag         string `gorm:"column:archive_etag" json:"-"`
	ArchiveLastModified string `json:"-"`
	ExplicitHTTPSPort   bool   `json:"-"`
	CredentialRef       string `json:"-"`
	RootID              string `gorm:"not null" json:"-"`
	UUIDModel
	UserID   string `gorm:"not null;uniqueIndex:plugin_repository_owner,priority:1" json:"-"`
	Origin   string `gorm:"not null;uniqueIndex:plugin_repository_owner,priority:2" json:"origin"`
	Indices  []byte `gorm:"not null" json:"-"`
	Revision string `gorm:"not null" json:"revision"`
}
