package credentials

import (
	"os"
	"testing"
)

func TestResolveExternalRef_LiteralValue(t *testing.T) {
	val, err := ResolveExternalRef("ghp_abc123")
	if err != nil {
		t.Fatal(err)
	}
	if val != "ghp_abc123" {
		t.Errorf("esperado ghp_abc123, obteve %s", val)
	}
}

func TestResolveExternalRef_EmptyString(t *testing.T) {
	val, err := ResolveExternalRef("")
	if err != nil {
		t.Fatal(err)
	}
	if val != "" {
		t.Errorf("esperado string vazia, obteve %q", val)
	}
}

func TestResolveEnvRef_Success(t *testing.T) {
	t.Setenv("TEST_CRED_TOKEN", "my-secret-token")

	val, err := ResolveExternalRef("env://TEST_CRED_TOKEN")
	if err != nil {
		t.Fatal(err)
	}
	if val != "my-secret-token" {
		t.Errorf("esperado my-secret-token, obteve %s", val)
	}
}

func TestResolveEnvRef_Missing(t *testing.T) {
	os.Unsetenv("NONEXISTENT_VAR_XYZ")
	_, err := ResolveExternalRef("env://NONEXISTENT_VAR_XYZ")
	if err == nil {
		t.Error("esperava erro para var não definida")
	}
}

func TestResolveEnvRef_EmptyName(t *testing.T) {
	_, err := ResolveExternalRef("env://")
	if err == nil {
		t.Error("esperava erro para nome vazio")
	}
}

func TestResolveKeyringRef_Empty(t *testing.T) {
	_, err := resolveKeyringRef("keyring://")
	if err == nil {
		t.Error("esperava erro para ref vazia")
	}
}

func TestResolveKeyringRef_NoSlash(t *testing.T) {
	// Sem "/" tenta lookup direto (wincred no Windows, erro em outras plataformas)
	// De qualquer forma deve retornar erro pois o target não existe
	_, err := resolveKeyringRef("keyring://nonexistent-target-xyz")
	if err == nil {
		t.Error("esperava erro para target inexistente")
	}
}

func TestIsExternalRef(t *testing.T) {
	if !IsExternalRef("keyring://gh:github.com/leo") {
		t.Error("deveria ser external ref")
	}
	if !IsExternalRef("env://GITHUB_TOKEN") {
		t.Error("deveria ser external ref")
	}
	if IsExternalRef("ghp_abc123") {
		t.Error("não deveria ser external ref")
	}
	if IsExternalRef("") {
		t.Error("string vazia não deveria ser external ref")
	}
}
