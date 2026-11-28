package plugins

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strings"

	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"assistente/internal/mcp"
	"assistente/internal/pluginpackage"
	"gorm.io/gorm"
)

type mcpCandidate struct {
	database.MCPServer
	BindingKey          string
	BindingInstallation string
	BindingDigest       string
}

// Limit the number of bind variables per query even for large external manifests.
// A renamed packaged server is still identified by its reviewed component key.
func mcpCandidates(ctx context.Context, db *gorm.DB, owner string, names []string, retiring map[string]bool, installation, digest string) (map[string]map[string]database.MCPServer, map[string]bool, error) {
	matches := make(map[string]map[string]database.MCPServer, len(names))
	installed := map[string]bool{}
	for _, name := range names {
		matches[name] = map[string]database.MCPServer{}
	}
	for len(names) > 0 {
		n := min(200, len(names))
		batch := names[:n]
		names = names[n:]
		keys := make([]string, 0, n)
		for _, name := range batch {
			keys = append(keys, "mcp/"+name)
		}
		var rows []mcpCandidate
		err := db.WithContext(ctx).Table("mcp_servers AS s").Select("s.*, coalesce(b.component_key,'') AS binding_key, coalesce(b.installation_id,'') AS binding_installation, coalesce(b.digest,'') AS binding_digest").
			Joins("LEFT JOIN plugin_mcp_bindings b ON b.server_id=s.id AND b.component_key IN ? AND EXISTS (SELECT 1 FROM plugin_installations p WHERE p.id=b.installation_id AND p.user_id=s.user_id)", keys).
			Where("s.user_id=? AND (s.slug IN ? OR s.name IN ? OR b.component_key IS NOT NULL)", owner, batch, batch).Scan(&rows).Error
		if err != nil {
			return nil, nil, err
		}
		for _, row := range rows {
			if retiring[row.ID] {
				continue
			}
			for _, name := range []string{row.Slug, row.Name, strings.TrimPrefix(row.BindingKey, "mcp/")} {
				if group, ok := matches[name]; ok && len(group) < 2 {
					group[row.ID] = row.MCPServer
				}
			}
			if installation != "" && row.BindingInstallation == installation && row.BindingDigest == digest {
				installed[row.BindingKey] = true
			}
		}
	}
	return matches, installed, nil
}

// Compare the connection the package can actually declare. Runtime toggles,
// the native slug and the account identity remain properties of the registration.
func matchMCP(row database.MCPServer, wanted mcp.ServerConfig) (string, error) {
	cfg, err := mcp.ConfigFromServerRecord(row)
	if err != nil {
		return "", err
	}
	token, err := mcpConfigDigest(&cfg)
	if err != nil {
		return "", err
	}
	wanted.ID, wanted.UserID, wanted.Name, wanted.Description, wanted.Slug = cfg.ID, cfg.UserID, cfg.Name, cfg.Description, cfg.Slug
	wanted.Enabled, wanted.AutoConnect = cfg.Enabled, cfg.AutoConnect
	expected, err := mcpConfigDigest(&wanted)
	if err != nil || expected != token {
		return "", err
	}
	return token, nil
}

func (i *Inventory) DescribeExistingMCP(ctx context.Context, components []Component, snap *pluginpackage.LocalSnapshot, installation string, retiring []string) error {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	_, servers, _ := mcpEntries(snap)
	var names []string
	for _, c := range components {
		if c.Kind == "mcp" && c.Reason == "" && !c.Active {
			names = append(names, c.Name)
		}
	}
	excluded := make(map[string]bool, len(retiring))
	for _, id := range retiring {
		excluded[id] = true
	}
	matches, installed, err := mcpCandidates(ctx, i.db, owner, names, excluded, installation, snap.Digest)
	if err != nil {
		return err
	}
	for n := range components {
		c := &components[n]
		rows := matches[c.Name]
		if c.Kind != "mcp" || c.Reason != "" || c.Active || len(rows) == 0 || installed[c.Key] {
			continue
		}
		c.Reason = "mcpConflict"
		if len(rows) != 1 {
			continue
		}
		cfg, _ := portableMCP(servers[c.Name])
		var candidate database.MCPServer
		for _, row := range rows {
			candidate = row
		}
		revision, err := matchMCP(candidate, cfg)
		if err != nil {
			return err
		}
		if revision != "" {
			c.Reason = "mcpAlreadyAvailable"
			c.MCPRevision = revision
		}
	}
	return nil
}

var ErrComponentEdited = errors.New("PLUGIN_COMPONENT_EDITED")

// portableMCP maps only transports and connection fields that the native
// client can honor for a package. It never contacts the endpoint.
func portableMCP(raw json.RawMessage) (mcp.ServerConfig, string) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return mcp.ServerConfig{}, "invalid"
	}
	var name, address string
	if json.Unmarshal(fields["type"], &name) != nil {
		return mcp.ServerConfig{}, "invalid"
	}
	var transport mcp.TransportType
	switch name {
	case "streamable-http":
		transport = mcp.TransportStreamable
	case "sse":
		transport = mcp.TransportSSE
	default:
		return mcp.ServerConfig{}, "transport"
	}
	if json.Unmarshal(fields["url"], &address) != nil {
		return mcp.ServerConfig{}, "invalid"
	}
	if strings.Contains(address, "${") {
		return mcp.ServerConfig{}, "configuration"
	}
	u, err := url.Parse(address)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return mcp.ServerConfig{}, "invalid"
	}
	if u.Scheme != "https" && (u.Scheme != "http" || (!strings.EqualFold(u.Hostname(), "localhost") && !net.ParseIP(u.Hostname()).IsLoopback())) {
		return mcp.ServerConfig{}, "invalid"
	}
	// Query parameters and headers may carry credentials. A pending declaration
	// exposes only its validated endpoint for inspection.
	if u.RawQuery != "" || u.ForceQuery || len(fields) > 2 {
		u.RawQuery, u.ForceQuery = "", false
		return mcp.ServerConfig{URL: u.String()}, "configuration"
	}
	return mcp.ServerConfig{URL: address, Transport: transport, AuthType: mcp.AuthNone, Enabled: true, AutoConnect: false}, ""
}

// BoundMCPServer identifies the native registration affected by an update.
type BoundMCPServer struct {
	ID, Name, Slug, ComponentKey string
	Preserved                    bool
	Edited                       bool
}

func (i *Inventory) BoundMCPServers(ctx context.Context, id, digest string) ([]BoundMCPServer, error) {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	return boundMCPServers(ctx, i.db, owner, id, digest)
}

func boundMCPServers(ctx context.Context, db *gorm.DB, owner, id, digest string) ([]BoundMCPServer, error) {
	var rows []struct {
		database.MCPServer         `gorm:"embedded"`
		ConfigDigest, ComponentKey string
		Preserved                  bool
	}
	err := db.WithContext(ctx).Table("plugin_mcp_bindings AS b").Select("s.*, b.config_digest, b.component_key, (b.consumer_only OR EXISTS (SELECT 1 FROM plugin_mcp_bindings other WHERE other.server_id=b.server_id AND (other.installation_id<>b.installation_id OR other.component_key<>b.component_key))) AS preserved").
		Joins("JOIN plugin_installations p ON p.id = b.installation_id AND p.digest = b.digest").
		Joins("JOIN mcp_servers s ON s.id = b.server_id AND s.user_id = p.user_id").
		Where("p.id = ? AND p.user_id = ? AND p.digest = ?", id, owner, digest).Order("s.name, s.id").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	servers := make([]BoundMCPServer, 0, len(rows))
	for _, row := range rows {
		cfg, err := mcp.ConfigFromServerRecord(row.MCPServer)
		if err != nil {
			return nil, err
		}
		digest, err := mcpConfigDigest(&cfg)
		if err != nil {
			return nil, err
		}
		servers = append(servers, BoundMCPServer{ID: cfg.ID, Name: cfg.Name, Slug: cfg.Slug, ComponentKey: row.ComponentKey, Preserved: row.Preserved, Edited: !row.Preserved && digest != row.ConfigDigest})
	}
	return servers, nil
}

func mcpConfigDigest(cfg *mcp.ServerConfig) (string, error) {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:]), nil
}

func (i *Inventory) RegisterMCP(ctx context.Context, id, digest, key string, admit commandsecurity.Admission) error {
	return i.registerMCP(ctx, id, digest, key, "", admit)
}

func (i *Inventory) ReuseMCP(ctx context.Context, id, digest, key, revision string, admit commandsecurity.Admission) error {
	if revision == "" {
		return ErrInstallationChanged
	}
	return i.registerMCP(ctx, id, digest, key, revision, admit)
}

func (i *Inventory) registerMCP(ctx context.Context, id, digest, key, revision string, admit commandsecurity.Admission) error {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	snap, err := i.installedSnapshot(ctx, id, digest)
	if err != nil {
		return err
	}
	_, servers, _ := mcpEntries(snap)
	var cfg mcp.ServerConfig
	for _, component := range DiscoverComponents(snap) {
		if component.Key == key && component.Kind == "mcp" && component.Reason == "" {
			var reason string
			cfg, reason = portableMCP(servers[component.Name])
			if reason != "" {
				return ErrComponentUnavailable
			}
			cfg.Name = component.Name
			break
		}
	}
	if cfg.URL == "" {
		return ErrComponentUnavailable
	}
	cfg.Slug = "plugin-" + strings.ToLower(rand.Text())
	err = database.WithSQLiteAdmittedCommit(ctx, i.db, "plugins.mcp.register", func(tx *gorm.DB) error {
		var install database.PluginInstallation
		if err := tx.Where("user_id=? AND id=? AND digest=?", owner, id, digest).Take(&install).Error; err != nil {
			return err
		}
		if install.Disabled {
			return ErrInstallationDisabled
		}
		var binding database.PluginMCPBinding
		err := tx.Where("installation_id=? AND component_key=?", id, key).Take(&binding).Error
		if err == nil {
			if binding.Digest != digest {
				return ErrInstallationChanged
			}
			existing, lookup := mcp.NewDBRepository(tx).GetServerByID(ctx, binding.ServerID)
			if lookup == nil && revision != "" {
				current, err := mcpConfigDigest(existing)
				if err != nil {
					return err
				}
				if !binding.ConsumerOnly || current != revision || binding.ConfigDigest != revision {
					return ErrInstallationChanged
				}
			}
			if !errors.Is(lookup, gorm.ErrRecordNotFound) {
				return lookup
			}
			if err := tx.Delete(&binding).Error; err != nil {
				return err
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		byName, _, err := mcpCandidates(ctx, tx, owner, []string{cfg.Name}, nil, "", "")
		candidates := byName[cfg.Name]
		if err != nil {
			return err
		}
		if revision != "" {
			if len(candidates) != 1 {
				return ErrInstallationChanged
			}
			var candidate database.MCPServer
			for _, row := range candidates {
				candidate = row
			}
			current, err := matchMCP(candidate, cfg)
			if err != nil {
				return err
			}
			if current != revision {
				return ErrInstallationChanged
			}

			return tx.Create(&database.PluginMCPBinding{InstallationID: id, ComponentKey: key, Digest: digest, ServerID: candidate.ID, Slug: candidate.Slug, ConfigDigest: current, ConsumerOnly: true}).Error
		}
		if len(candidates) > 0 {
			return ErrInstallationChanged
		}
		serverID, err := mcp.CreatePackagedServer(ctx, tx, cfg)
		if err != nil {
			return err
		}
		saved, err := mcp.NewDBRepository(tx).GetServerByID(ctx, serverID)
		if err != nil {
			return err
		}
		fingerprint, err := mcpConfigDigest(saved)
		if err != nil {
			return err
		}
		return tx.Create(&database.PluginMCPBinding{InstallationID: id, ComponentKey: key, Digest: digest, ServerID: serverID, Slug: cfg.Slug, ConfigDigest: fingerprint}).Error
	}, admit)
	if err != nil {
		return err
	}
	if revision != "" {
		return nil
	}
	return i.SyncMCP(ctx, admit)
}

// removeMCPBindings is called inside the same writer transaction that
// retires the package. A modified native config needs an explicit choice.
func removeMCPBindings(ctx context.Context, tx *gorm.DB, id, key string, preserve bool) error {
	var rows []database.PluginMCPBinding
	query := tx.Where("installation_id=?", id)
	if key != "" {
		query = query.Where("component_key=?", key)
	}
	if err := query.Find(&rows).Error; err != nil {
		return err
	}
	repo := mcp.NewDBRepository(tx)
	for _, row := range rows {
		cfg, err := repo.GetServerByID(ctx, row.ServerID)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var others int64
		if err := tx.Model(&database.PluginMCPBinding{}).Where("server_id=? AND (installation_id<>? OR component_key<>?)", row.ServerID, row.InstallationID, row.ComponentKey).Count(&others).Error; err != nil {
			return err
		}
		if err == nil && !preserve && !row.ConsumerOnly && others == 0 {
			current, err := mcpConfigDigest(cfg)
			if err != nil {
				return err
			}
			if current != row.ConfigDigest {
				return ErrComponentEdited
			}
			if err := mcp.DeletePackagedServer(ctx, tx, row.ServerID, row.Slug); err != nil {
				return err
			}
		}
		if err := tx.Delete(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func (i *Inventory) RemoveMCP(ctx context.Context, id, digest, key string, preserve bool, admit commandsecurity.Admission) error {
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	if key == "" {
		return ErrComponentUnavailable
	}
	err = database.WithSQLiteAdmittedCommit(ctx, i.db, "plugins.mcp.remove", func(tx *gorm.DB) error {
		var install database.PluginInstallation
		if err := tx.Where("user_id=? AND id=? AND digest=?", owner, id, digest).Take(&install).Error; err != nil {
			return err
		}
		return removeMCPBindings(ctx, tx, id, key, preserve)
	}, admit)
	if err != nil {
		return err
	}
	return i.SyncMCP(ctx, admit)
}
