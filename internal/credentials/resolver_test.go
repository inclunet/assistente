package credentials

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestResolveExternalRef_LiteralValue(t *testing.T) {
	val, err := resolveTestSource("static", "ghp_abc123", nil)
	if err != nil {
		t.Fatal(err)
	}
	if val != "ghp_abc123" {
		t.Errorf("esperado ghp_abc123, obteve %s", val)
	}
}

func TestResolveExternalRef_EmptyString(t *testing.T) {
	val, err := resolveTestSource("static", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if val != "" {
		t.Errorf("esperado string vazia, obteve %q", val)
	}
}

func TestResolveEnvRef_Success(t *testing.T) {
	t.Setenv("TEST_CRED_TOKEN", "my-secret-token")

	val, err := resolveTestSource("env", "", &SourceConfig{Env: "TEST_CRED_TOKEN"})
	if err != nil {
		t.Fatal(err)
	}
	if val != "my-secret-token" {
		t.Errorf("esperado my-secret-token, obteve %s", val)
	}
}

func TestResolveEnvRef_Missing(t *testing.T) {
	_ = os.Unsetenv("NONEXISTENT_VAR_XYZ")
	_, err := resolveTestSource("env", "", &SourceConfig{Env: "NONEXISTENT_VAR_XYZ"})
	if err == nil {
		t.Error("esperava erro para var não definida")
	}
}

func TestResolveEnvRef_EmptyName(t *testing.T) {
	_, err := resolveTestSource("env", "", &SourceConfig{})
	if err == nil {
		t.Error("esperava erro para nome vazio")
	}
}

func TestResolveKeyringRef_Empty(t *testing.T) {
	_, err := resolveTestSource("keyring", "", &SourceConfig{})
	if err == nil {
		t.Error("esperava erro para ref vazia")
	}
}

func TestResolveKeyringRef_NoSlash(t *testing.T) {
	// Sem "/" tenta lookup direto (wincred no Windows, erro em outras plataformas)
	// De qualquer forma deve retornar erro pois o target não existe
	_, err := resolveTestSource("keyring", "", &SourceConfig{KeyringTarget: "nonexistent-target-xyz"})
	if err == nil {
		t.Error("esperava erro para target inexistente")
	}
}

// Com service/user, a falha relatada é a do go-keyring — não a mensagem do
// lookup direto sugerindo "use o formato keyring://service/user", que o usuário
// já está usando.
func TestResolveKeyringRef_ServiceUserErrorMentionsKeyring(t *testing.T) {
	_, err := resolveTestSource("keyring", "", &SourceConfig{KeyringService: "servico-inexistente-xyz", KeyringUser: "usuario-xyz"})
	if err == nil {
		t.Fatal("esperava erro para service/user inexistente")
	}
	msg := err.Error()
	if !strings.Contains(msg, "keyring") {
		t.Errorf("erro deveria citar a ref completa: %s", msg)
	}
	if strings.HasPrefix(msg, "erro ao buscar keyring://servico-inexistente-xyz/usuario-xyz: lookup direto") {
		t.Errorf("erro do lookup direto não deveria mascarar o do go-keyring: %s", msg)
	}
}

// Prefixes in static material are literal, never an external-source contract.
func TestStaticDoesNotInterpretPrefixes(t *testing.T) {
	for _, value := range []string{"env://GITHUB_TOKEN", "keyring://service/user", "literal", ""} {
		got, err := resolveTestSource("static", value, nil)
		if err != nil || got != value {
			t.Fatalf("static material changed: %v", err)
		}
	}
	if _, err := resolveTestSource("", "legacy", nil); err == nil {
		t.Fatal("legacy source must require manual reconfiguration")
	}
}

func resolveTestSource(source, token string, config *SourceConfig) (string, error) {
	auth, err := ResolveSource(context.Background(), &AuthConfig{Source: source, Type: "bearer", Token: token, SourceConfig: config})
	if err != nil {
		return "", err
	}
	return auth.Token, nil
}
