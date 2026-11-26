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
			if err := db.AutoMigrate(&database.PluginInstallation{}, &database.PluginRevision{}, &database.PluginSkillActivation{}, &database.PluginAgentActivation{}, &database.PluginProfileActivation{}, &database.Job{}, &database.PluginProviderBinding{}, &database.LLMProvider{}, &database.PluginMCPBinding{}, &database.MCPServer{}); err != nil {
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
	if components[3].Reason != "supportingFiles" || !components[3].RequestsShell {
		t.Fatal(components)
	}
}

func TestSupportingFilesPreserveClaudeShellNotice(t *testing.T) {
	snap := &pluginpackage.LocalSnapshot{Manifest: pluginpackage.Manifest{Format: pluginpackage.ClaudeCode}, Files: map[string][]byte{
		"skills/note/SKILL.md":     []byte("---\ndescription: Note\n---\nRead !`echo note`."),
		"skills/note/reference.md": []byte("material"),
	}}
	got := DiscoverComponents(snap)
	if len(got) != 1 || got[0].Reason != "supportingFiles" || !got[0].RequestsShell {
		t.Fatal(got)
	}
}

func TestClaudeAgentFilesAppearInPreviewWithoutActivation(t *testing.T) {
	snap := &pluginpackage.LocalSnapshot{
		Manifest: pluginpackage.Manifest{Format: pluginpackage.ClaudeCode},
		Files: map[string][]byte{
			"agents/reviewer.md":         []byte("---\nname: reviewer\n---\nReview"),
			"agents/backend/reviewer.md": []byte("Review the backend"),
			"agents/schema.json":         []byte("{}"),
			"examples/reviewer.md":       []byte("example"),
			"skills/notes/SKILL.md":      []byte("---\nname: notes\ndescription: Read notes\n---\nRead."),
		},
	}
	got := DiscoverComponents(snap)
	if len(got) != 3 {
		t.Fatal(got)
	}
	for i, entry := range []string{"agents/backend/reviewer.md", "agents/reviewer.md"} {
		c := got[i]
		if c.Entry != entry || c.Key != entry || c.Kind != "agent" || c.Reason != "agentConfiguration" || c.Active {
			t.Fatal(c)
		}
	}
	if got[2].Kind != "skill" || got[2].Reason != "" {
		t.Fatal(got[2])
	}
}

func TestClaudeAgentGrantIsLocalAndRevocable(t *testing.T) {
	store := NewInventory(managedJobTestDB(t))
	user := database.WithUserID(t.Context(), "owner")
	other := database.WithUserID(t.Context(), "other")
	dir := t.TempDir()
	for name, content := range map[string]string{
		".claude-plugin/plugin.json": `{"name":"equipe"}`,
		"agents/reviewer.md":         "---\nname: reviewer\ndescription: Review\ntools: Read, Grep\nmodel: inherit\n---\nReview the change.",
		"agents/unsupported.md":      "---\nname: writer\ndescription: Edit\ntools: [UnknownTool]\n---\nEdit.",
	} {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	snap, err := pluginpackage.InspectLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	admit := func(_ *gorm.DB, commit func() error) error { return commit() }
	row, err := store.InstallSource(user, snap.Origin, snap.Digest, "", nil, admit, func() (*pluginpackage.LocalSnapshot, error) { return snap, nil })
	if err != nil {
		t.Fatal(err)
	}
	toggle := func(key string, active bool) error {
		return store.SetAgentActive(user, row.ID, row.Digest, key, active, admit)
	}
	if err := toggle("agents/unsupported.md", true); err == nil {
		t.Fatal("ativou uma restrição desconhecida")
	}
	key := "agents/reviewer.md"
	if err := toggle(key, true); err != nil {
		t.Fatal(err)
	}
	items, err := store.InstalledComponents(user, row.ID, row.Digest)
	if err != nil {
		t.Fatal(err)
	}
	var slug string
	for _, item := range items {
		if item.Key == key && item.Active {
			slug = item.Slug
		}
	}
	if slug == "" {
		t.Fatal("ativação ausente")
	}
	generation, err := store.AgentGeneration(user, slug)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AgentGeneration(other, slug); err == nil {
		t.Fatal("outra conta resolveu o slug")
	}
	run, err := store.ResolveAgent(user, generation)
	if err != nil || run.AgentInstructions != "Review the change." || len(run.Tools) != 2 || run.Tools[0] != "read_file" || run.Tools[1] != "grep_search" {
		t.Fatal(run, err)
	}
	if err := toggle(key, false); err != nil {
		t.Fatal(err)
	}
	if err := toggle(key, true); err != nil {
		t.Fatal(err)
	}
	if err := store.CheckAgent(user, generation); err == nil {
		t.Fatal("uma reativação restaurou a concessão anterior")
	}
}

func TestClaudeAgentDefinitionDiagnostics(t *testing.T) {
	const header = "---\nname: reviewer\ndescription: Review\n"
	const body = "---\nInstructions."
	for _, tc := range []struct{ option, field string }{
		{"tools: [Write]\n", "tools"},
		{"hooks: {}\n", "hooks"},
		{"model: opus\n", "model"},
		{"size: small\n", "size"},
	} {
		_, field, err := readClaudeAgent([]byte(header + tc.option + body))
		if err == nil || field != tc.field {
			t.Errorf("%q: field=%q error=%v", tc.option, field, err)
		}
	}
	exact := header + body + strings.Repeat("x", maxClaudeAgentBytes-len(header+body))
	if _, _, err := readClaudeAgent([]byte(exact)); err != nil {
		t.Fatal(err)
	}
	if _, field, err := readClaudeAgent([]byte(exact + "x")); err != errAgentTooLarge || field != "" {
		t.Fatal(field, err)
	}
}
