package wailsapi

import (
	"slices"
	"testing"
)

func TestUnauthenticatedAllowlistHasLoginPath(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"Login", "GetAuthStatus", "Logout"} {
		if !slices.Contains(UnauthenticatedAppMethods, name) {
			t.Fatalf("%s deve estar na allowlist sem auth", name)
		}
	}
}

func TestTokensMethodsNotOnUnauthAllowlist(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"GetConversationTokenStats",
		"GetTurnTokenStats",
		"GetRecentMessagesTokenCount",
		"CheckContextWindowThreshold",
	} {
		if slices.Contains(UnauthenticatedAppMethods, name) {
			t.Fatalf("%s é autenticado via Tokens/WithUser; não pertence à allowlist", name)
		}
	}
}

func TestAllowlistsMethodsNotOnUnauthAllowlist(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"RespondQuestionnaire",
		"GetAllowlists",
		"GetAllowlist",
		"CreateAllowlist",
		"UpdateAllowlist",
		"DeleteAllowlist",
		"GetAllowlistSearchPaths",
	} {
		if slices.Contains(UnauthenticatedAppMethods, name) {
			t.Fatalf("%s é autenticado via Allowlists/WithUser; não pertence à allowlist", name)
		}
	}
}

func TestSkillsMethodsNotOnUnauthAllowlist(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"GetSkills",
		"GetSkill",
		"CreateSkill",
		"DuplicateSkill",
		"UpdateSkill",
		"DeleteSkill",
		"GetUserInvocableSkillsForProfile",
		"GetSkillSearchPaths",
	} {
		if slices.Contains(UnauthenticatedAppMethods, name) {
			t.Fatalf("%s é autenticado via Skills/WithUser; não pertence à allowlist", name)
		}
	}
}

func TestToolsMethodsNotOnUnauthAllowlist(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"GetAvailableTools",
		"GetRuntimeToolCatalog",
	} {
		if slices.Contains(UnauthenticatedAppMethods, name) {
			t.Fatalf("%s é autenticado via Tools/WithUser; não pertence à allowlist", name)
		}
	}
}

func TestUpdaterMethodsNotOnUnauthAllowlist(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"GetAppVersion",
		"CheckForUpdates",
		"ApplyUpdate",
		"StartUpdate",
	} {
		if slices.Contains(UnauthenticatedAppMethods, name) {
			t.Fatalf("%s é autenticado via Updater/WithUser; não pertence à allowlist", name)
		}
	}
}

func TestProfilesMethodsNotOnUnauthAllowlist(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"GetProfiles",
		"GetProfile",
		"GetActiveProfile",
		"GetActiveProfileSlug",
		"GetActiveProfileAndSlug",
		"SetActiveProfile",
		"CreateProfile",
		"DuplicateProfile",
		"UpdateProfile",
		"DeleteProfile",
		"GetProfileSearchPaths",
		"GetContextProviders",
	} {
		if slices.Contains(UnauthenticatedAppMethods, name) {
			t.Fatalf("%s é autenticado via Profiles/WithUser; não pertence à allowlist", name)
		}
	}
}

func TestHotkeysMethodsNotOnUnauthAllowlist(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"IsGlobalHotkeySupported",
	} {
		if slices.Contains(UnauthenticatedAppMethods, name) {
			t.Fatalf("%s é autenticado via Hotkeys/WithUser; não pertence à allowlist", name)
		}
	}
}

func TestNetTrustMethodsNotOnUnauthAllowlist(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"GetNetworkAllowlist",
		"RemoveNetworkAllowlistEntry",
	} {
		if slices.Contains(UnauthenticatedAppMethods, name) {
			t.Fatalf("%s é autenticado via NetTrust/WithUser; não pertence à allowlist", name)
		}
	}
}
