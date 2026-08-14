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

func TestCredentialsCRUDMethodsNotOnUnauthAllowlist(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"ListCredentials",
		"UpsertCredential",
		"DeleteCredential",
		"ListExternalSources",
	} {
		if slices.Contains(UnauthenticatedAppMethods, name) {
			t.Fatalf("%s é autenticado via Credentials/WithUser; não pertence à allowlist", name)
		}
	}
}

func TestCredentialsVaultMethodsOnUnauthAllowlist(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"HasMasterKey",
		"SetupMasterPassword",
		"GetVaultIntegrityStatus",
		"CanPersistCredentials",
	} {
		if !slices.Contains(UnauthenticatedAppMethods, name) {
			t.Fatalf("%s é pré-sessão (vault/onboarding); deve estar na allowlist", name)
		}
	}
}

func TestSettingsMethodsNotOnUnauthAllowlist(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"GetNativeTTSProviders",
		"TestConnection",
		"TestConnectionWithModels",
		"ResetConfig",
		"ClearAllCredentials",
		"ClearAllProfiles",
		"ClearAllSkills",
		"ClearAllChannels",
	} {
		if slices.Contains(UnauthenticatedAppMethods, name) {
			t.Fatalf("%s é autenticado via Settings/WithUser|WithAdmin; não pertence à allowlist", name)
		}
	}
}

func TestTerminalMethodsNotOnUnauthAllowlist(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"ListTerminalSessions",
		"CreateTerminalSession",
		"CloseTerminalSession",
		"GetTerminalHistory",
		"RunTerminalCommand",
		"SendTerminalInput",
		"InterruptTerminalCommand",
		"GetTerminalStats",
	} {
		if slices.Contains(UnauthenticatedAppMethods, name) {
			t.Fatalf("%s é autenticado via Terminal/WithUser; não pertence à allowlist", name)
		}
	}
}

func TestMCPMethodsNotOnUnauthAllowlist(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"ListMCPServers",
		"ConnectMCPServer",
		"DisconnectMCPServer",
		"ReconnectMCPServer",
		"SaveMCPServer",
		"DuplicateMCPServer",
		"DeleteMCPServer",
		"GetMCPServerTools",
		"GetMCPServerConfig",
		"ReadMCPResource",
		"GetMCPPrompt",
		"SetMCPWorkspaceRoots",
		"GetMCPWorkspaceRoots",
		"SubscribeToMCPResource",
		"UnsubscribeFromMCPResource",
		"SaveMCPServerAuth",
		"DeleteMCPServerAuth",
		"GetMCPServerAuthInfo",
		"DiscoverMCPServerAuth",
		"GetMCPServerLogs",
	} {
		if slices.Contains(UnauthenticatedAppMethods, name) {
			t.Fatalf("%s é autenticado via MCP/WithUser; não pertence à allowlist", name)
		}
	}
}

func TestSignalMethodsNotOnUnauthAllowlist(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"SignalRegister",
		"SignalVerify",
		"SignalLink",
		"SignalLinkRaw",
		"SignalUnregister",
		"SignalCheckAPI",
		"SignalListAccounts",
	} {
		if slices.Contains(UnauthenticatedAppMethods, name) {
			t.Fatalf("%s é autenticado via Signal/WithUser; não pertence à allowlist", name)
		}
	}
}
