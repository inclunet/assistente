package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestLegacyContextProviderSkillsMigrationFirstRunBacksUpAndVersionsMarker(t *testing.T) {
	t.Parallel()
	homeDir := t.TempDir()
	for _, slug := range legacyContextProviderSkillSlugs {
		writeLegacySkill(t, homeDir, slug)
	}
	keptDir := filepath.Join(homeDir, "coding")
	if err := os.MkdirAll(keptDir, 0755); err != nil {
		t.Fatalf("mkdir kept skill: %v", err)
	}

	now := time.Date(2026, time.September, 6, 12, 30, 45, 0, time.FixedZone("UTC-3", -3*60*60))
	migrator := newLegacyContextProviderSkillsMigrator(homeDir, "0.6.0")
	migrator.now = func() time.Time { return now }
	result := migrator.Run()

	if result.Status != legacyContextProviderMigrationCompleted {
		t.Fatalf("status = %q, want %q; failures=%v", result.Status, legacyContextProviderMigrationCompleted, result.Failures)
	}
	if result.MarkerFormat != "versioned" || result.MarkerAppVersion != "0.6.0" {
		t.Fatalf("marker diagnostic = format %q app %q", result.MarkerFormat, result.MarkerAppVersion)
	}
	if !slices.Equal(result.BackedUpSlugs, legacyContextProviderSkillSlugs) {
		t.Fatalf("backed up = %#v, want %#v", result.BackedUpSlugs, legacyContextProviderSkillSlugs)
	}
	backupRoot := filepath.Join(homeDir, ".legacy-backup", "context-providers-20260906-123045")
	for _, slug := range legacyContextProviderSkillSlugs {
		if _, err := os.Stat(filepath.Join(homeDir, slug)); !os.IsNotExist(err) {
			t.Fatalf("legacy skill %s remains active, stat err=%v", slug, err)
		}
		if _, err := os.Stat(filepath.Join(backupRoot, slug, "SKILL.md")); err != nil {
			t.Fatalf("backup %s: %v", slug, err)
		}
	}
	if _, err := os.Stat(keptDir); err != nil {
		t.Fatalf("non-legacy skill should remain: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(homeDir, legacyContextProviderCleanupMarker))
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	var marker legacyContextProviderMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		t.Fatalf("parse versioned marker: %v; data=%q", err, data)
	}
	if marker.FormatVersion != legacyContextProviderMarkerFormatVersion {
		t.Fatalf("format version = %d, want %d", marker.FormatVersion, legacyContextProviderMarkerFormatVersion)
	}
	if marker.AppVersion != "0.6.0" || !marker.CompletedAt.Equal(now) {
		t.Fatalf("marker = %#v, want app 0.6.0 at %s", marker, now)
	}
}

func TestLegacyContextProviderSkillsMigrationAcceptsExistingLegacyMarker(t *testing.T) {
	t.Parallel()
	homeDir := t.TempDir()
	markerPath := filepath.Join(homeDir, legacyContextProviderCleanupMarker)
	if err := os.WriteFile(markerPath, []byte("2026-06-17T10:30:00-03:00"), 0644); err != nil {
		t.Fatalf("write legacy marker: %v", err)
	}
	for _, slug := range legacyContextProviderSkillSlugs {
		writeLegacySkill(t, homeDir, slug)
	}

	result := newLegacyContextProviderSkillsMigrator(homeDir, "0.6.0").Run()

	if result.Status != legacyContextProviderMigrationAlreadyCompleted {
		t.Fatalf("status = %q, want %q", result.Status, legacyContextProviderMigrationAlreadyCompleted)
	}
	if result.MarkerFormat != "legacy" || result.MarkerAppVersion != "" {
		t.Fatalf("legacy marker diagnostic = format %q app %q", result.MarkerFormat, result.MarkerAppVersion)
	}
	for _, slug := range legacyContextProviderSkillSlugs {
		if _, err := os.Stat(filepath.Join(homeDir, slug, "SKILL.md")); err != nil {
			t.Fatalf("skill %s should remain after existing marker: %v", slug, err)
		}
	}
}

func TestLegacyContextProviderSkillsMigrationReportsAndRecoversPartialFailure(t *testing.T) {
	t.Parallel()
	homeDir := t.TempDir()
	for _, slug := range legacyContextProviderSkillSlugs {
		writeLegacySkill(t, homeDir, slug)
	}
	now := time.Date(2026, time.September, 6, 15, 0, 0, 0, time.UTC)
	migrator := newLegacyContextProviderSkillsMigrator(homeDir, "0.6.0")
	migrator.now = func() time.Time { return now }
	realRename := migrator.rename
	migrator.rename = func(oldPath, newPath string) error {
		if filepath.Base(oldPath) == "workspace" {
			return &os.LinkError{
				Op:  "rename",
				Old: oldPath,
				New: newPath,
				Err: errors.New("falha injetada"),
			}
		}
		return realRename(oldPath, newPath)
	}

	result := migrator.Run()

	if result.Status != legacyContextProviderMigrationPartialFailure {
		t.Fatalf("status = %q, want %q", result.Status, legacyContextProviderMigrationPartialFailure)
	}
	if !slices.Equal(result.BackedUpSlugs, []string{"memory"}) || len(result.Failures) != 1 {
		t.Fatalf("partial result = backed up %#v failures %#v", result.BackedUpSlugs, result.Failures)
	}
	if strings.Contains(strings.Join(result.Failures, " "), homeDir) {
		t.Fatalf("failure diagnostic exposes home path: %#v", result.Failures)
	}
	if _, err := os.Stat(filepath.Join(homeDir, "memory")); !os.IsNotExist(err) {
		t.Fatalf("memory should be backed up, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(homeDir, "workspace", "SKILL.md")); err != nil {
		t.Fatalf("workspace should remain active after its failure: %v", err)
	}
	markerPath := filepath.Join(homeDir, legacyContextProviderCleanupMarker)
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("marker should not exist after partial failure, stat err=%v", err)
	}

	retry := newLegacyContextProviderSkillsMigrator(homeDir, "0.6.1")
	retry.now = func() time.Time { return now }
	retryResult := retry.Run()
	if retryResult.Status != legacyContextProviderMigrationCompleted {
		t.Fatalf("retry status = %q, failures=%v", retryResult.Status, retryResult.Failures)
	}
	if !slices.Equal(retryResult.BackedUpSlugs, []string{"workspace"}) {
		t.Fatalf("retry backed up = %#v, want workspace", retryResult.BackedUpSlugs)
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("marker should exist after successful retry: %v", err)
	}
}

func TestLegacyContextProviderSkillsMigrationMarkerlessSupportPolicy(t *testing.T) {
	t.Parallel()
	result := newLegacyContextProviderSkillsMigrator(t.TempDir(), "dev").Run()

	if result.MarkerlessReleaseWindow != 2 {
		t.Fatalf("markerless release window = %d, want 2", result.MarkerlessReleaseWindow)
	}
	if result.Status != legacyContextProviderMigrationCompleted {
		t.Fatalf("empty install status = %q, failures=%v", result.Status, result.Failures)
	}
}

func writeLegacySkill(t *testing.T, homeDir, slug string) {
	t.Helper()
	skillDir := filepath.Join(homeDir, slug)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", slug, err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# legacy "+slug), 0644); err != nil {
		t.Fatalf("write %s: %v", slug, err)
	}
}
