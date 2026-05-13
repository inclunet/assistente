package allowlist

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"assistente/internal/configdir"
)

// newTestManager cria um Manager isolado em um diretorio temporario do teste.
func newTestManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	return &Manager{resolver: configdir.NewResolverWithBase(dir)}
}

func writeAllowlistFile(t *testing.T, m *Manager, slug string, al *Allowlist) {
	t.Helper()
	data, err := json.MarshalIndent(al, "", "  ")
	if err != nil {
		t.Fatalf("marshal allowlist: %v", err)
	}
	if err := m.resolver.EnsureHomeDir(); err != nil {
		t.Fatalf("ensure home dir: %v", err)
	}
	if err := m.resolver.Create(slug+".json", data); err != nil {
		t.Fatalf("create %s.json: %v", slug, err)
	}
}

func TestEnsureDefaults_CreatesPadraoWhenEmpty(t *testing.T) {
	m := newTestManager(t)

	if err := m.EnsureDefaults(); err != nil {
		t.Fatalf("EnsureDefaults: %v", err)
	}

	got, err := m.Get(defaultSlug)
	if err != nil {
		t.Fatalf("Get(padrao): %v", err)
	}
	if len(got.CommandRules) == 0 {
		t.Fatal("padrao deveria ter CommandRules apos criacao")
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

	if err := m.EnsureDefaults(); err != nil {
		t.Fatalf("EnsureDefaults: %v", err)
	}

	got, err := m.Get(defaultSlug)
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

	if err := m.EnsureDefaults(); err != nil {
		t.Fatalf("EnsureDefaults: %v", err)
	}

	got, err := m.Get(defaultSlug)
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

	if err := m.EnsureDefaults(); err != nil {
		t.Fatalf("EnsureDefaults#1: %v", err)
	}
	first, err := m.Get(defaultSlug)
	if err != nil {
		t.Fatalf("Get(padrao)#1: %v", err)
	}

	if err := m.EnsureDefaults(); err != nil {
		t.Fatalf("EnsureDefaults#2: %v", err)
	}
	second, err := m.Get(defaultSlug)
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

	if err := m.EnsureDefaults(); err != nil {
		t.Fatalf("EnsureDefaults: %v", err)
	}

	got, err := m.Get("personalizado")
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

	err := m.save("x", bad)
	if err == nil {
		t.Fatal("save aceitou allowlist invalida")
	}

	// O arquivo nao deve ter sido criado.
	if m.resolver.Exists(filepath.Base("x.json")) {
		t.Fatal("save persistiu allowlist invalida no disco")
	}
}
