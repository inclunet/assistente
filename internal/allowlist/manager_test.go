package allowlist

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"assistente/internal/testutil/authorizationfixture"
	"context"
	"os"
)

// newTestManager cria um Manager isolado em um diretorio temporario do teste.
func newTestManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	repo, _ := authorizationfixture.Open(t, dir)
	return NewManager(Options{Repository: repo, HomeDir: func() string { return dir }, Paths: func() []string { return []string{dir} }, MayImport: func(context.Context) bool { return true }})
}

func writeAllowlistFile(t *testing.T, m *Manager, slug string, al *Allowlist) {
	t.Helper()
	data, err := json.MarshalIndent(al, "", "  ")
	if err != nil {
		t.Fatalf("marshal allowlist: %v", err)
	}

	if err := os.WriteFile(filepath.Join(m.paths()[0], slug+".json"), data, 0600); err != nil {
		t.Fatalf("create %s.json: %v", slug, err)
	}
}

func TestEnsureDefaults_CreatesPadraoWhenEmpty(t *testing.T) {
	m := newTestManager(t)

	if err := m.EnsureDefaults(authorizationfixture.Context()); err != nil {
		t.Fatalf("EnsureDefaults: %v", err)
	}

	got, err := m.Get(authorizationfixture.Context(), defaultSlug)
	if err != nil {
		t.Fatalf("Get(padrao): %v", err)
	}
	if len(got.CommandRules) == 0 {
		t.Fatal("padrao deveria ter CommandRules apos criacao")
	}
}

func TestMigracaoPreservaNegativasComMaiusculas(t *testing.T) {
	m := newTestManager(t)
	writeAllowlistFile(t, m, "restrita", &Allowlist{Name: "Restrita", DefaultAction: " DENY ", CommandRules: []CommandRule{{Program: "git", Decision: " DENY "}}})
	policy, err := m.Get(authorizationfixture.Context(), "restrita")
	if err != nil {
		t.Fatal(err)
	}
	if policy.DefaultAction != "deny" || len(policy.CommandRules) != 1 || strings.ToLower(strings.TrimSpace(policy.CommandRules[0].Decision)) != "deny" {
		t.Fatalf("negativa alterada na importação: %+v", policy)
	}
}

func TestEnsureDefaults_MigratesPadraoWithoutKubectlRules(t *testing.T) {
	m := newTestManager(t)

	// Simula um padrao.json pre-AEP-0060: sem command_rules para kubectl.
	preExisting := &Allowlist{
		Name:          "Padrao",
		AutoApprove:   []string{"ls", "kubectl get *"},
		AlwaysDeny:    []string{"rm -rf /"},
		DefaultAction: "confirm",
	}
	writeAllowlistFile(t, m, defaultSlug, preExisting)

	if err := m.EnsureDefaults(authorizationfixture.Context()); err != nil {
		t.Fatalf("EnsureDefaults: %v", err)
	}

	got, err := m.Get(authorizationfixture.Context(), defaultSlug)
	if err != nil {
		t.Fatalf("Get(padrao): %v", err)
	}

	// O usuario nao tinha NENHUMA regra estruturada para kubectl, entao todas
	// as regras default de kubectl devem ter sido mescladas.
	hasGet := false
	hasDelete := false
	for _, rule := range got.CommandRules {
		if rule.Program != "kubectl" {
			continue
		}
		if len(rule.Subcommands) == 1 && rule.Subcommands[0] == "get" {
			hasGet = true
		}
		if len(rule.Subcommands) == 1 && rule.Subcommands[0] == "delete" {
			hasDelete = true
		}
	}
	if !hasGet || !hasDelete {
		t.Fatalf("migracao nao adicionou regras default de kubectl: %+v", got.CommandRules)
	}

	// A allowlist legada deve ter sido preservada.
	if len(got.AutoApprove) != len(preExisting.AutoApprove) {
		t.Fatalf("AutoApprove perdido na migracao: got=%v want=%v", got.AutoApprove, preExisting.AutoApprove)
	}
}

func TestEnsureDefaults_DoesNotOverwriteUserKubectlRules(t *testing.T) {
	m := newTestManager(t)

	// Usuario customizou parcialmente kubectl: tem regra para "get" e quer
	// que "delete" seja approve (perigoso, mas e a vontade do usuario).
	customGet := CommandRule{Program: "kubectl", Subcommands: []string{"get"}, Args: []string{"*"}, Decision: "approve"}
	customDelete := CommandRule{Program: "kubectl", Subcommands: []string{"delete"}, Args: []string{"*"}, Decision: "approve"}
	preExisting := &Allowlist{
		Name:          "Padrao",
		DefaultAction: "confirm",
		CommandRules:  []CommandRule{customGet, customDelete},
	}
	writeAllowlistFile(t, m, defaultSlug, preExisting)

	if err := m.EnsureDefaults(authorizationfixture.Context()); err != nil {
		t.Fatalf("EnsureDefaults: %v", err)
	}

	got, err := m.Get(authorizationfixture.Context(), defaultSlug)
	if err != nil {
		t.Fatalf("Get(padrao): %v", err)
	}

	// Esperamos exatamente as duas regras originais — nada migrado para kubectl.
	if len(got.CommandRules) != 2 {
		t.Fatalf("regras estruturadas alteradas (esperado 2): %+v", got.CommandRules)
	}
	for _, rule := range got.CommandRules {
		if rule.Program != "kubectl" {
			t.Fatalf("regra de programa diferente apareceu: %+v", rule)
		}
		if rule.Decision != "approve" {
			t.Fatalf("regra do usuario foi alterada: %+v", rule)
		}
	}
}

func TestEnsureDefaults_MigrationIsIdempotent(t *testing.T) {
	m := newTestManager(t)

	preExisting := &Allowlist{
		Name:          "Padrao",
		DefaultAction: "confirm",
	}
	writeAllowlistFile(t, m, defaultSlug, preExisting)

	if err := m.EnsureDefaults(authorizationfixture.Context()); err != nil {
		t.Fatalf("EnsureDefaults#1: %v", err)
	}
	first, err := m.Get(authorizationfixture.Context(), defaultSlug)
	if err != nil {
		t.Fatalf("Get(padrao)#1: %v", err)
	}

	if err := m.EnsureDefaults(authorizationfixture.Context()); err != nil {
		t.Fatalf("EnsureDefaults#2: %v", err)
	}
	second, err := m.Get(authorizationfixture.Context(), defaultSlug)
	if err != nil {
		t.Fatalf("Get(padrao)#2: %v", err)
	}

	if len(first.CommandRules) != len(second.CommandRules) {
		t.Fatalf("migracao nao e idempotente: 1=%d 2=%d", len(first.CommandRules), len(second.CommandRules))
	}
}

func TestEnsureDefaults_DoesNotMigrateOtherProfiles(t *testing.T) {
	m := newTestManager(t)

	// Allowlist com slug diferente; nao deveria ser tocada pela migracao.
	other := &Allowlist{
		Name:          "Personalizado",
		DefaultAction: "confirm",
	}
	writeAllowlistFile(t, m, "personalizado", other)

	if err := m.EnsureDefaults(authorizationfixture.Context()); err != nil {
		t.Fatalf("EnsureDefaults: %v", err)
	}

	got, err := m.Get(authorizationfixture.Context(), "personalizado")
	if err != nil {
		t.Fatalf("Get(personalizado): %v", err)
	}
	if len(got.CommandRules) != 0 {
		t.Fatalf("migracao tocou em allowlist nao-padrao: %+v", got.CommandRules)
	}
}

func TestSave_RejectsInvalidAllowlist(t *testing.T) {
	m := newTestManager(t)

	bad := &Allowlist{
		Name:          "x",
		DefaultAction: "confirm",
		CommandRules: []CommandRule{
			{Program: "kubectl", Decision: "approv"},
		},
	}

	_, err := m.Create(authorizationfixture.Context(), bad)
	if err == nil {
		t.Fatal("save aceitou allowlist invalida")
	}

	if _, err := m.Get(authorizationfixture.Context(), "x"); err == nil {
		t.Fatal("persistiu política inválida no banco")
	}
}
