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
