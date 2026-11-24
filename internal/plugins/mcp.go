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
	"gorm.io/gorm"
)

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
	ID, Name string
	Edited   bool
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
		database.MCPServer `gorm:"embedded"`
		ConfigDigest       string
	}
	err := db.WithContext(ctx).Table("plugin_mcp_bindings AS b").Select("s.*, b.config_digest").
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
		servers = append(servers, BoundMCPServer{ID: cfg.ID, Name: cfg.Name, Edited: digest != row.ConfigDigest})
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
	owner, err := database.RequireUserID(ctx)
	if err != nil {
		return err
	}
	snap, err := i.installedSnapshot(ctx, id, digest)
	if err != nil {
		return err
	}
	var file struct {
		Servers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(snap.Files["mcp.json"], &file); err != nil {
		return ErrComponentUnavailable
	}
	var cfg mcp.ServerConfig
	for _, component := range DiscoverComponents(snap) {
		if component.Key == key && component.Kind == "mcp" && component.Reason == "" {
			var reason string
			cfg, reason = portableMCP(file.Servers[component.Name])
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
		var binding database.PluginMCPBinding
		err := tx.Where("installation_id=? AND component_key=?", id, key).Take(&binding).Error
		if err == nil {
			if binding.Digest != digest {
				return ErrInstallationChanged
			}
			_, lookup := mcp.NewDBRepository(tx).GetServerByID(ctx, binding.ServerID)
			if !errors.Is(lookup, gorm.ErrRecordNotFound) {
				return lookup
			}
			if err := tx.Delete(&binding).Error; err != nil {
				return err
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
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
		if err == nil && !preserve {
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
