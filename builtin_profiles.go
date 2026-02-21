package main

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"assistente/internal/configdir"
	"assistente/internal/profiles"
)

//go:embed all:.assistente/profiles
var builtinProfilesFS embed.FS

// installBuiltinProfiles copies embedded profiles to ~/.assistente/profiles/.
// Installs new profiles and updates those with an older _builtin_version.
//
// Version logic:
//   - File doesn't exist → install
//   - File exists with _builtin_version → update if embedded is newer
//   - File exists without _builtin_version → legacy builtin (pre-versioning), treat as "0.0.0" and update
//
// To prevent updates, users can set _builtin_version to "999.0.0" in their profile JSON.
func (a *App) installBuiltinProfiles() {
	resolver := configdir.NewResolver("profiles")
	homeDir := resolver.GetHomeDir()
	if homeDir == "" {
		log.Printf("[Profiles] Home dir not available, skipping builtin profile install")
		return
	}

	if err := os.MkdirAll(homeDir, 0755); err != nil {
		log.Printf("[Profiles] Error creating profiles home dir: %v", err)
		return
	}

	entries, err := fs.ReadDir(builtinProfilesFS, ".assistente/profiles")
	if err != nil {
		log.Printf("[Profiles] Error reading embedded profiles: %v", err)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		embeddedData, err := fs.ReadFile(builtinProfilesFS, ".assistente/profiles/"+entry.Name())
		if err != nil {
			log.Printf("[Profiles] Error reading embedded profile %s: %v", entry.Name(), err)
			continue
		}

		var embeddedProfile profiles.Profile
		if err := json.Unmarshal(embeddedData, &embeddedProfile); err != nil {
			log.Printf("[Profiles] Error parsing embedded profile %s: %v", entry.Name(), err)
			continue
		}

		if embeddedProfile.BuiltinVersion == "" {
			log.Printf("[Profiles] Embedded profile %s has no _builtin_version, skipping", entry.Name())
			continue
		}

		targetFile := filepath.Join(homeDir, entry.Name())

		if existingData, err := os.ReadFile(targetFile); err == nil {
			var existingProfile profiles.Profile
			if err := json.Unmarshal(existingData, &existingProfile); err == nil {
				installedVersion := existingProfile.BuiltinVersion
				if installedVersion == "" {
					// Legacy profile (pre-versioning, created by old EnsureDefaults) — treat as 0.0.0
					installedVersion = "0.0.0"
					log.Printf("[Profiles] Migrating legacy profile %s (no _builtin_version → v0.0.0)", entry.Name())
				}
				if !isVersionNewer(embeddedProfile.BuiltinVersion, installedVersion) {
					log.Printf("[Profiles] Builtin %s v%s up to date (installed: v%s)", entry.Name(), embeddedProfile.BuiltinVersion, installedVersion)
					continue
				}
				log.Printf("[Profiles] Updating builtin profile %s: v%s → v%s", entry.Name(), installedVersion, embeddedProfile.BuiltinVersion)
			}
		} else {
			log.Printf("[Profiles] Installing builtin profile %s v%s", entry.Name(), embeddedProfile.BuiltinVersion)
		}

		prettyData, err := json.MarshalIndent(embeddedProfile, "", "  ")
		if err != nil {
			log.Printf("[Profiles] Error marshaling profile %s: %v", entry.Name(), err)
			continue
		}

		if err := os.WriteFile(targetFile, prettyData, 0644); err != nil {
			log.Printf("[Profiles] Error writing %s: %v", targetFile, err)
		}
	}
}
