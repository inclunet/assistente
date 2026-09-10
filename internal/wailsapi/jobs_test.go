package wailsapi

import (
	"assistente/internal/jobprofilegrant"
	"assistente/internal/profileaccess"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestJobsNotWired(t *testing.T) {
	t.Parallel()
	api := NewJobs()
	if _, err := api.GetJobs(); !errors.Is(err, ErrJobsNotWired) {
		t.Fatalf("got %v", err)
	}
}

func TestJobsUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	dir := filepath.Dir(thisFile)
	for _, name := range []string{"jobs.go", "jobs_dryrun.go"} {
		src, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		body := string(src)
		if strings.Contains(body, "requireAuthenticatedContext(") {
			t.Fatalf("%s não deve chamar requireAuthenticatedContext(; use WithUser", name)
		}
	}
	src, err := os.ReadFile(filepath.Join(dir, "jobs.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "WithUser(") {
		t.Fatal("jobs.go deve chamar WithUser(")
	}
}

func TestJobGrantCoversLiteralAndDynamicProfiles(t *testing.T) {
	state := profileaccess.JobGrantState{
		Grants: []jobprofilegrant.Grant{{TargetProfileSlug: "especialista"}},
	}
	if !jobGrantCovers(state, map[string]any{"profile": "especialista"}) {
		t.Fatal("grant literal exato deveria permitir habilitação")
	}
	if jobGrantCovers(state, map[string]any{"profile": "outro"}) {
		t.Fatal("grant de outro target não deveria permitir habilitação")
	}
	if !jobGrantCovers(state, map[string]any{"profile": "{{ event.profile }}"}) {
		t.Fatal("template com ao menos um target autorizado deveria permitir habilitação")
	}
	if jobGrantCovers(profileaccess.JobGrantState{}, map[string]any{"profile": "{{ event.profile }}"}) {
		t.Fatal("template sem targets autorizados não deveria permitir habilitação")
	}
}
