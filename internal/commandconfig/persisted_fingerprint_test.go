package commandconfig

import (
	"testing"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandautomation"
)

func TestPersistedFingerprintIgnoresOnlyDerivedJobClaims(t *testing.T) {
	base := Snapshot{Scope: Scope{UserID: "owner"}}
	want, err := base.PersistedFingerprint()
	if err != nil || len(want) != 64 {
		t.Fatalf("fingerprint base inválido: %q, %v", want, err)
	}
	job := base
	job.ActivationClaims = []commandactivation.Claim{{ActivationID: "job-claim", SourceType: "job", LayerRef: "layer"}}
	got, err := job.PersistedFingerprint()
	if err != nil || got != want {
		t.Fatalf("claim derivado alterou base persistida: %q, %v", got, err)
	}
	job.ActivationClaims[0].LayerRef = "other-layer"
	got, err = job.PersistedFingerprint()
	if err != nil || got != want {
		t.Fatalf("conteúdo de claim derivado alterou base: %q, %v", got, err)
	}
	// Este fingerprint NÃO autoriza preservar a execução: a resolução e a
	// proveniência do claim devem ser verificadas separadamente pelo host.
	for _, source := range []string{"manual", "event", "context", "", "JOB"} {
		t.Run(source, func(t *testing.T) {
			changed := base
			changed.ActivationClaims = []commandactivation.Claim{{ActivationID: "claim", SourceType: source}}
			got, err := changed.PersistedFingerprint()
			if err != nil || got == want {
				t.Fatalf("claim não-job não distinguiu configuração: %q, %v", got, err)
			}
		})
	}
}

func TestPersistedFingerprintTracksEveryPersistentDependency(t *testing.T) {
	base := Snapshot{Scope: Scope{UserID: "owner"}}
	want, err := base.PersistedFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	workspace := "workspace"
	revoked := time.Unix(100, 0).UTC()
	cases := map[string]func(*Snapshot){
		"owner":             func(s *Snapshot) { s.Scope.UserID = "other" },
		"workspace":         func(s *Snapshot) { s.Scope.WorkspaceID = &workspace },
		"layer":             func(s *Snapshot) { s.Layers = []Layer{{ID: "layer", Enabled: true}} },
		"binding condition": func(s *Snapshot) { s.Bindings = []Binding{{ID: "binding", Condition: `{"focused":true}`}} },
		"generation":        func(s *Snapshot) { s.Generations = []Generation{{Generation: 2}} },
		"activation rule":   func(s *Snapshot) { s.ActivationRules = []commandactivation.Rule{{ID: "rule", Enabled: true}} },
		"grant revocation":  func(s *Snapshot) { s.AutomationGrants = []commandautomation.Grant{{ID: "grant", RevokedAt: &revoked}} },
		"non-job claim": func(s *Snapshot) {
			s.ActivationClaims = []commandactivation.Claim{{ActivationID: "claim", SourceType: "manual"}}
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			changed := base
			change(&changed)
			got, err := changed.PersistedFingerprint()
			if err != nil || got == want {
				t.Fatalf("dependência persistida ignorada: %q, %v", got, err)
			}
			again, err := changed.PersistedFingerprint()
			if err != nil || again != got {
				t.Fatalf("fingerprint instável: %q != %q, %v", again, got, err)
			}
		})
	}
}

func TestPersistedFingerprintKeepsNonJobClaimsInMixedProjection(t *testing.T) {
	manual := commandactivation.Claim{ActivationID: "manual", SourceType: "manual", LayerRef: "base"}
	base := Snapshot{Scope: Scope{UserID: "owner"}, ActivationClaims: []commandactivation.Claim{manual}}
	want, err := base.PersistedFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	mixed := base
	mixed.ActivationClaims = []commandactivation.Claim{
		{ActivationID: "job-before", SourceType: "job"}, manual,
		{ActivationID: "job-after", SourceType: "job"},
	}
	got, err := mixed.PersistedFingerprint()
	if err != nil || got != want {
		t.Fatalf("claims de job interferiram na identidade da base: %q, %v", got, err)
	}
	if len(mixed.ActivationClaims) != 3 || mixed.ActivationClaims[1].ActivationID != "manual" {
		t.Fatal("fingerprint alterou o snapshot recebido")
	}
	mixed.ActivationClaims[1].LayerRef = "other"
	got, err = mixed.PersistedFingerprint()
	if err != nil || got == want {
		t.Fatalf("mudança do claim manual foi ocultada pelos claims de job: %q, %v", got, err)
	}
}
