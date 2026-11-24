package plugins

import (
	"assistente/internal/database"
	"assistente/internal/pluginpackage"
	"assistente/internal/skills"
	"assistente/internal/sqlite"
	"context"
	"errors"
	"gorm.io/gorm"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVendorPackagesUseRetainedFormatAndCommandRunner(t *testing.T) {
	for _, format := range []string{"claude", "cursor", "codex"} {
		t.Run(format, func(t *testing.T) {
			dir := t.TempDir()
			write := func(name, content string) {
				t.Helper()
				file := filepath.Join(dir, name)
				if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file, []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			write("."+format+"-plugin/plugin.json", `{"name":"kit","version":"1","skills":"./skills/"}`)
			mcpFile := ".mcp.json"
			if format == "cursor" {
				mcpFile = "mcp.json"
			}
			write(mcpFile, `{"mcpServers":{"docs":{"type":"http","url":"https://example.org/mcp"}}}`)
			body := "Read the notes."
			if format == "claude" {
				body = "Context: !`echo first` and !`echo second`."
			}
			write("skills/note/SKILL.md", "---\ndescription: Read a note\n---\n"+body)
			snap, err := pluginpackage.InspectLocal(dir)
			if err != nil {
				t.Fatal(err)
			}
			if snap.Manifest.Name != "kit" || len(snap.Manifest.Unmapped) != 0 {
				t.Fatal(snap.Manifest)
			}
			db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "vendor.db")), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			conn, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = conn.Close() })
			if err := db.AutoMigrate(&database.PluginInstallation{}, &database.PluginRevision{}, &database.PluginSkillActivation{}, &database.PluginMCPBinding{}, &database.MCPServer{}); err != nil {
				t.Fatal(err)
			}
			owner := database.WithUserID(context.Background(), "reader")
			store := NewInventory(db)
			allow := func(_ *gorm.DB, commit func() error) error { return commit() }
			row, err := store.InstallLocal(owner, dir, snap.Digest, "")
			if err != nil {
				t.Fatal(err)
			}
			components, err := store.InstalledComponents(owner, row.ID, row.Digest)
			if err != nil || len(components) != 2 || components[0].Reason != "" || components[0].Endpoint != "https://example.org/mcp" || components[1].Reason != "" || components[1].RequestsShell != (format == "claude") {
				t.Fatal(components, err)
			}
			if err := store.SetSkillActive(owner, row.ID, row.Digest, "skills/note", true, allow); err != nil {
				t.Fatal(err)
			}
			manager := skills.NewManagerWithInstalledSkills(store.ActiveSkills)
			catalog, err := skills.ListForContext(owner, manager)
			if err != nil {
				t.Fatal(err)
			}
			policy := skills.ResolveSelectionPolicy(catalog, nil, false, false)
			active, err := store.ActiveSkills(owner)
			if err != nil || len(active) != 1 {
				t.Fatal(active, err)
			}
			refused := errors.New("denied by command policy")
			var calls []string
			run := func(reject bool) (string, error) {
				ctx := skills.WithCommandTurn(owner, func(_ context.Context, s *skills.Skill, cmd string) (string, error) {
					calls = append(calls, cmd)
					if s.Slug != active[0].Slug {
						t.Fatal(s.Slug)
					}
					if reject {
						return "", refused
					}
					return "result !`other`", nil
				})
				result, found, err := skills.InvokeContext(ctx, "/"+active[0].Slug, manager, "conversation", policy)
				if !found {
					t.Fatal("skill missing")
				}
				if err != nil {
					return "", err
				}
				return result.Content, nil
			}
			if format == "claude" {
				if _, err = run(true); !errors.Is(err, refused) || len(calls) != 1 {
					t.Fatal(calls, err)
				}
				calls = nil
			}
			result, err := run(false)
			if err != nil {
				t.Fatal(err)
			}
			if format == "claude" && (strings.Join(calls, ",") != "echo first,echo second" || strings.Count(result, "result !`other`") != 2) {
				t.Fatal(calls, result)
			}
			if err := store.SetSkillActive(owner, row.ID, row.Digest, "skills/note", true, allow); err != nil {
				t.Fatal(err)
			}
			if err := active[0].CheckCommandActivation(owner); err != nil {
				t.Fatal(err)
			}
			if err := store.SetSkillActive(owner, row.ID, row.Digest, "skills/note", false, allow); err != nil {
				t.Fatal(err)
			}
			if err := active[0].CheckCommandActivation(owner); !errors.Is(err, ErrComponentUnavailable) {
				t.Fatal("stale skill retained command access", err)
			}
			if err := store.SetSkillActive(owner, row.ID, row.Digest, "skills/note", true, allow); err != nil {
				t.Fatal(err)
			}
			if err := active[0].CheckCommandActivation(owner); !errors.Is(err, ErrComponentUnavailable) {
				t.Fatal("old activation became valid again", err)
			}
			if format != "claude" && len(calls) != 0 {
				t.Fatal(calls)
			}
		})
	}
}

func TestVendorConventionsInCapturedFiles(t *testing.T) {
	snapshot := &pluginpackage.LocalSnapshot{
		Manifest: pluginpackage.Manifest{Format: pluginpackage.ClaudeCode},
		Files:    map[string][]byte{".mcp.json": []byte(`{"docs":{"url":"https://example.org/mcp"}}`)},
	}
	components := DiscoverComponents(snapshot)
	if len(components) != 1 || components[0].Name != "docs" || components[0].Reason != "" {
		t.Fatal(components)
	}

	snapshot.Manifest.Format = pluginpackage.Cursor
	snapshot.Files = map[string][]byte{"rules/project.mdc": []byte("project instructions"), "skills/note/SKILL.md": []byte("---\ndescription: Read a note\n---\nRead")}
	components = DiscoverComponents(snapshot)
	if len(components) != 1 || components[0].Reason != "manifestConfiguration" {
		t.Fatal(components)
	}

	files := map[string][]byte{"plugin.json": []byte(`{"$schema":"` + pluginpackage.ManifestSchema + `","name":"legacy"}`), ".claude-plugin/plugin.json": []byte(`{"name":"overlay"}`)}
	_, _, manifest, _, err := pluginpackage.LocateManifest(func(name string) ([]byte, error) {
		raw, ok := files[name]
		if !ok {
			return nil, os.ErrNotExist
		}
		return raw, nil
	})
	if err != nil || manifest.Format != pluginpackage.AgentPlugins || manifest.Name != "legacy" {
		t.Fatal(manifest, err)
	}
}

func TestVendorMCPRequirements(t *testing.T) {
	for _, tc := range []struct{ path, data, kind, reason string }{
		{".mcp.json", `{"mcpServers":{"docs":{"url":"https://${TENANT}.example.org/mcp"}}}`, "mcp", "configuration"},
		{".mcp.json/example", "{}", "invalidMcp", ""},
		{".mcp.json", `{"mcpServers":{"docs":{"url":"https://example.org/${TENANT}/mcp"}}}`, "mcp", "configuration"},
	} {
		snap := &pluginpackage.LocalSnapshot{Manifest: pluginpackage.Manifest{Format: pluginpackage.ClaudeCode}, Files: map[string][]byte{tc.path: []byte(tc.data)}}
		got := DiscoverComponents(snap)
		if len(got) != 1 || got[0].Kind != tc.kind || got[0].Reason != tc.reason {
			t.Fatalf("%s: %+v", tc.path, got)
		}
	}
}

func TestVendorRequirementsStayWithTheirContribution(t *testing.T) {
	files := map[string][]byte{".cursor-plugin/plugin.json": []byte(`{"name":"kit","mcpServers":{"server":{}}}`), "skills/note/SKILL.md": []byte("---\nname: note\ndescription: A note\n---\nread")}
	_, manifest, _, err := pluginpackage.CapturedManifest(files)
	if err != nil {
		t.Fatal(err)
	}
	components := DiscoverComponents(&pluginpackage.LocalSnapshot{Manifest: manifest, Files: files})
	if len(components) != 2 || components[0].Kind != "mcp" || components[0].Reason != "manifestConfiguration" || components[1].Reason != "" {
		t.Fatal(components)
	}
	delete(files, "skills/note/SKILL.md")
	files["SKILL.md"] = []byte("note")
	components = DiscoverComponents(&pluginpackage.LocalSnapshot{Manifest: manifest, Files: files})
	if len(components) != 2 || components[0].Entry != "SKILL.md" || components[0].Reason != "rootSkill" {
		t.Fatal(components)
	}
}

func TestAdditionalClaudeContributionsKeepDefaultsVisible(t *testing.T) {
	files := map[string][]byte{".claude-plugin/plugin.json": []byte(`{"name":"kit","skills":["./extra"],"mcpServers":"./servers.json"}`), "skills/note/SKILL.md": []byte("---\ndescription: A note\n---\nnotes"), ".mcp.json": []byte(`{"docs":{"url":"https://example.org/mcp"}}`)}
	_, manifest, _, err := pluginpackage.CapturedManifest(files)
	if err != nil {
		t.Fatal(err)
	}
	snap := &pluginpackage.LocalSnapshot{Manifest: manifest, Files: files}
	components := DiscoverComponents(snap)
	if len(components) != 4 || components[2].Key != "mcp/docs" || components[2].Reason != "manifestConfiguration" || components[3].Key != "skills/note" || components[3].Reason != "" {
		t.Fatal(components)
	}
	files["bin/package-tool"] = []byte("utility")
	files["skills/note/SKILL.md"] = []byte("---\ndescription: A note\n---\nRun !`package-tool`.")
	components = DiscoverComponents(snap)
	if components[3].Reason != "supportingFiles" {
		t.Fatal(components)
	}
}
