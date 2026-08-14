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
		"UpdateProfileMediaSupport",
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

func TestMemoryMethodsNotOnUnauthAllowlist(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"ListMemoryRecords",
		"SearchMemoryRecords",
		"GetMemoryRecord",
		"CreateMemoryRecord",
		"UpdateMemoryRecord",
		"ArchiveMemoryRecord",
		"UnarchiveMemoryRecord",
		"DeleteMemoryRecord",
		"GetMemoryPolicySummary",
	} {
		if slices.Contains(UnauthenticatedAppMethods, name) {
			t.Fatalf("%s é autenticado via Memory/WithUser; não pertence à allowlist", name)
		}
	}
}

func TestDatabaseMethodsNotOnUnauthAllowlist(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"ResetDatabase",
		"ClearMessages",
		"GetMaintenanceSettings",
		"SaveMaintenanceSettings",
		"GetDatabaseStats",
		"RunDatabaseMaintenance",
	} {
		if slices.Contains(UnauthenticatedAppMethods, name) {
			t.Fatalf("%s é autenticado via Database/WithUser|WithAdmin; não pertence à allowlist", name)
		}
	}
}

func TestWelcomeMethodsNotOnUnauthAllowlist(t *testing.T) {
	t.Parallel()
	// welcome vive em wailsapi.Welcome (não *App). NeedsWelcomeWizard é dual-mode
	// sem WithUser, mas UnauthenticatedAppMethods só lista métodos do *App.
	for _, name := range []string{
		"NeedsWelcomeWizard",
		"RunWelcomeWizard",
	} {
		if slices.Contains(UnauthenticatedAppMethods, name) {
			t.Fatalf("%s está em wailsapi.Welcome; não pertence à allowlist de *App", name)
		}
	}
}

func TestLegacyCleanupMethodsNotOnUnauthAllowlist(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"CleanupLegacyChannelJSON",
	} {
		if slices.Contains(UnauthenticatedAppMethods, name) {
			t.Fatalf("%s é autenticado via LegacyCleanup/WithUser; não pertence à allowlist", name)
		}
	}
}

func TestSubagentMethodsNotOnUnauthAllowlist(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"ListSubAgentRuns",
		"CancelSubAgentRun",
	} {
		if slices.Contains(UnauthenticatedAppMethods, name) {
			t.Fatalf("%s é autenticado via Subagent/WithUser; não pertence à allowlist", name)
		}
	}
}

func TestACPProvidersMethodsNotOnUnauthAllowlist(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"TestACPAgent",
		"DetectACPAgent",
	} {
		if slices.Contains(UnauthenticatedAppMethods, name) {
			t.Fatalf("%s é autenticado via ACPProviders/WithUser; não pertence à allowlist", name)
		}
	}
}

func TestACPCommandsMethodsNotOnUnauthAllowlist(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"GetAgentSessionCommands",
	} {
		if slices.Contains(UnauthenticatedAppMethods, name) {
			t.Fatalf("%s é autenticado via ACPCommands/WithUser; não pertence à allowlist", name)
		}
	}
}

func TestACPTrustMethodsNotOnUnauthAllowlist(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"GetAgentPermissions",
		"RevokeAgentPermission",
	} {
		if slices.Contains(UnauthenticatedAppMethods, name) {
			t.Fatalf("%s é autenticado via ACPTrust/WithUser; não pertence à allowlist", name)
		}
	}
}

func TestLLMProvidersMethodsNotOnUnauthAllowlist(t *testing.T) {
	t.Parallel()
	// CreateDefaultLLMProvider é bootstrap pré-login sem WithUser, mas vive em
	// wailsapi.LLMProviders — UnauthenticatedAppMethods só lista métodos do *App.
	for _, name := range []string{
		"GetLLMProviders",
		"GetLLMProvider",
		"GetActiveProviderInfo",
		"GetLLMProvidersWithStatus",
		"TestLLMProvider",
		"ListModelsRaw",
		"CreateLLMProvider",
		"UpdateLLMProvider",
		"SetDefaultProvider",
		"DeleteLLMProvider",
		"ReloadLLMClient",
		"CreateDefaultLLMProvider",
	} {
		if slices.Contains(UnauthenticatedAppMethods, name) {
			t.Fatalf("%s está em wailsapi.LLMProviders; não pertence à allowlist de *App", name)
		}
	}
}

func TestJobsMethodsNotOnUnauthAllowlist(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"GetJobs",
		"GetJob",
		"ToggleJob",
		"RunJob",
		"DryRunJob",
		"GetJobRuns",
		"ReplayRun",
		"GetJobEvents",
		"GetJobEventsPage",
		"GetJobPipelines",
		"GetToolCatalog",
		"RegenerateJobCatalog",
		"SaveJob",
		"TestToolDryRun",
		"InferEventSchema",
		"ListKnownEvents",
		"DeleteJob",
	} {
		if slices.Contains(UnauthenticatedAppMethods, name) {
			t.Fatalf("%s é autenticado via Jobs/WithUser; não pertence à allowlist", name)
		}
	}
}
