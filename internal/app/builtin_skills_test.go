package app

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveLegacyContextProviderSkills(t *testing.T) {
	homeDir := t.TempDir()
	for _, slug := range legacyContextProviderSkillSlugs {
		skillDir := filepath.Join(homeDir, slug)
		if err := os.MkdirAll(skillDir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", slug, err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# legacy"), 0644); err != nil {
			t.Fatalf("write %s: %v", slug, err)
		}
	}
	keptDir := filepath.Join(homeDir, "coding")
	if err := os.MkdirAll(keptDir, 0755); err != nil {
		t.Fatalf("mkdir kept skill: %v", err)
	}

	removeLegacyContextProviderSkills(homeDir)

	for _, slug := range legacyContextProviderSkillSlugs {
		if _, err := os.Stat(filepath.Join(homeDir, slug)); !os.IsNotExist(err) {
			t.Fatalf("legacy skill %s should be moved out of active skills, stat err=%v", slug, err)
		}
		matches, err := filepath.Glob(filepath.Join(homeDir, ".legacy-backup", "context-providers-*", slug, "SKILL.md"))
		if err != nil {
			t.Fatalf("glob backup for %s: %v", slug, err)
		}
		if len(matches) != 1 {
			t.Fatalf("legacy skill %s should have one backup, got %d", slug, len(matches))
		}
	}
	if _, err := os.Stat(keptDir); err != nil {
		t.Fatalf("non-legacy skill should remain: %v", err)
	}
	if _, err := os.Stat(filepath.Join(homeDir, legacyContextProviderCleanupMarker)); err != nil {
		t.Fatalf("cleanup marker should be written: %v", err)
	}
}

func TestRemoveLegacyContextProviderSkillsDoesNotRunAfterMarker(t *testing.T) {
	homeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(homeDir, legacyContextProviderCleanupMarker), []byte("done"), 0644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	for _, slug := range legacyContextProviderSkillSlugs {
		skillDir := filepath.Join(homeDir, slug)
		if err := os.MkdirAll(skillDir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", slug, err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# custom "+slug), 0644); err != nil {
			t.Fatalf("write %s: %v", slug, err)
		}
	}

	removeLegacyContextProviderSkills(homeDir)

	for _, slug := range legacyContextProviderSkillSlugs {
		if _, err := os.Stat(filepath.Join(homeDir, slug, "SKILL.md")); err != nil {
			t.Fatalf("custom skill %s should remain after marker: %v", slug, err)
		}
	}
}

func TestRemoveLegacyContextProviderSkillsDoesNotWriteMarkerAfterBackupFailure(t *testing.T) {
	homeDir := t.TempDir()
	skillDir := filepath.Join(homeDir, legacyContextProviderSkillSlugs[0])
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir legacy skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# legacy"), 0644); err != nil {
		t.Fatalf("write legacy skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, ".legacy-backup"), []byte("not a dir"), 0644); err != nil {
		t.Fatalf("write backup blocker: %v", err)
	}

	removeLegacyContextProviderSkills(homeDir)

	if _, err := os.Stat(filepath.Join(homeDir, legacyContextProviderCleanupMarker)); !os.IsNotExist(err) {
		t.Fatalf("cleanup marker should not be written after backup failure, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
		t.Fatalf("legacy skill should remain after backup failure: %v", err)
	}
}

func TestBuiltinProfilesDoNotEnableLegacyContextProviderSkills(t *testing.T) {
	entries, err := fs.ReadDir(builtinProfilesFS, "builtin/profiles")
	if err != nil {
		t.Fatalf("read builtin profiles: %v", err)
	}
	legacy := map[string]bool{}
	for _, slug := range legacyContextProviderSkillSlugs {
		legacy[slug] = true
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := fs.ReadFile(builtinProfilesFS, "builtin/profiles/"+entry.Name())
		if err != nil {
			t.Fatalf("read profile %s: %v", entry.Name(), err)
		}
		var profile struct {
			Chat struct {
				EnabledSkills []string `json:"enabled_skills"`
			} `json:"chat"`
		}
		if err := json.Unmarshal(data, &profile); err != nil {
			t.Fatalf("parse profile %s: %v", entry.Name(), err)
		}
		for _, skill := range profile.Chat.EnabledSkills {
			if legacy[skill] {
				t.Fatalf("builtin profile %s enables legacy context provider skill %q", entry.Name(), skill)
			}
		}
	}
}
