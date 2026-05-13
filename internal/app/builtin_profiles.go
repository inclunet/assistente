package app

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

//go:embed all:builtin/profiles
var builtinProfilesFS embed.FS

// installBuiltinProfiles copies embedded profiles to ~/.assistente/profiles/.
// Installs new profiles and updates those with an older _builtin_version.
//
// Version logic:
//   - File doesn't exist → install
//   - File exists with _builtin_version → update if embedded is newer
//   - File exists without _builtin_version → legacy builtin (pre-versioning), treat as "0.0.0" and update
//
// CRÍTICO: campos de runtime NUNCA são sobrescritos do embedded.
// Embedded carrega "factory defaults" (chat config, voice, input, etc).
// Runtime do usuário (Active, MediaSupport) é preservado do disco —
// caso contrário, cada upgrade builtin ressuscitaria o flag de active
// embarcado, podendo deixar dois perfis com active=true ao mesmo tempo
// (foi exatamente esse bug que travou o picker em "Padrão" mesmo com
// outro perfil escolhido pelo usuário).
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

	entries, err := fs.ReadDir(builtinProfilesFS, "builtin/profiles")
	if err != nil {
		log.Printf("[Profiles] Error reading embedded profiles: %v", err)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		embeddedData, err := fs.ReadFile(builtinProfilesFS, "builtin/profiles/"+entry.Name())
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

		// Defesa em profundidade: jamais aceitar `active: true` num arquivo
		// embarcado. Active é estado de runtime do user, não dado de fábrica.
		// Se algum dev esquecer e marcar active=true no embedded, esse zero
		// aqui evita que o flag escape para o disco do usuário.
		embeddedProfile.Active = false

		targetFile := filepath.Join(homeDir, entry.Name())

		var existingProfile *profiles.Profile
		if existingData, err := os.ReadFile(targetFile); err == nil {
			var existing profiles.Profile
			if err := json.Unmarshal(existingData, &existing); err == nil {
				existingProfile = &existing
				installedVersion := existing.BuiltinVersion
				if installedVersion == "" {
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

		toWrite := mergeBuiltinPreservingRuntime(embeddedProfile, existingProfile)

		prettyData, err := json.MarshalIndent(toWrite, "", "  ")
		if err != nil {
			log.Printf("[Profiles] Error marshaling profile %s: %v", entry.Name(), err)
			continue
		}

		if err := os.WriteFile(targetFile, prettyData, 0644); err != nil {
			log.Printf("[Profiles] Error writing %s: %v", targetFile, err)
		}
	}

	a.ensureActiveProfile()
}

// mergeBuiltinPreservingRuntime aplica o conteúdo embedded (factory defaults)
// preservando campos de runtime escritos pelo usuário/sistema.
//
// Por que separar runtime de factory: o embedded reflete a opinião do build
// (defaults de chat/voice/input/skills). O runtime reflete decisões do user
// no app (perfil ativo, suporte a mídia detectado). Sobrescrever runtime a
// cada upgrade builtin desfaz silenciosamente escolhas do user — foi a
// origem do bug de "active: true" ressuscitando em perfis builtin.
func mergeBuiltinPreservingRuntime(embedded profiles.Profile, existing *profiles.Profile) profiles.Profile {
	merged := embedded
	if existing == nil {
		return merged
	}
	merged.Active = existing.Active
	if existing.MediaSupport != nil {
		merged.MediaSupport = existing.MediaSupport
	}
	return merged
}

// ensureActiveProfile verifies that at least one profile is marked Active.
// If none is, it marks "padrao" as active so the system has a deterministic default.
func (a *App) ensureActiveProfile() {
	if a.profileManager == nil {
		return
	}

	list, err := a.profileManager.List()
	if err != nil || len(list) == 0 {
		return
	}

	for _, info := range list {
		p, err := a.profileManager.Get(info.Slug)
		if err == nil && p.Active {
			return // already has an active profile
		}
	}

	if err := a.profileManager.SetActive("padrao"); err != nil {
		log.Printf("[Profiles] Could not set 'padrao' as active: %v", err)
		// Fallback: activate the first available profile
		if len(list) > 0 {
			_ = a.profileManager.SetActive(list[0].Slug)
		}
	}
}
