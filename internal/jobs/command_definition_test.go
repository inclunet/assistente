package jobs

import (
	"assistente/internal/commandjson"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func commandDefinitionJob(t *testing.T) *Job {
	t.Helper()
	return &Job{
		DatabaseID:      uuid.Must(uuid.NewV7()).String(),
		ID:              "daily-report",
		Name:            "Daily report",
		Enabled:         true,
		Pipeline:        "operations",
		PipelineEnabled: true,
		Tool:            "subagent",
		Inputs:          map[string]any{"profile": "specialist", "limit": float64(3)},
		Output:          OutputConfig{Schema: []byte(`{"type":"object"}`), Map: map[string]string{"result": "{{ .output }}"}},
		Events:          EventsConfig{OnSuccess: "report.ready", PayloadTemplate: `{"run":"{{ .run_id }}"}`},
		ErrorPolicy:     ErrorPolicy{Strategy: ErrorRetry, MaxRetries: 2, RetryDelay: "1s", Backoff: BackoffExponential, OnExhausted: OnExhaustedNotify, NotifyChannels: []string{"toast"}},
		MaxRunsPerHour:  12,
		DryRun:          DryRunConfig{Enabled: true, MockOutput: map[string]any{"ok": true}},
	}
}

func TestDefinitionFingerprintChangesExecutableDefinition(t *testing.T) {
	base := commandDefinitionJob(t)
	want, err := DefinitionFingerprint(base)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*Job){
		"inputs and grant target": func(job *Job) { job.Inputs["profile"] = "other-profile" },
		"tool":                    func(job *Job) { job.Tool = "web_fetch" },
		"events":                  func(job *Job) { job.Events.OnFailure = "report.failed" },
		"configuration enabled":   func(job *Job) { job.Enabled = false },
		"pipeline enabled":        func(job *Job) { job.PipelineEnabled = false },
		"output":                  func(job *Job) { job.Output.Map["result"] = "{{ .secret }}" },
		"error policy":            func(job *Job) { job.ErrorPolicy.MaxRetries++ },
		"dry run":                 func(job *Job) { job.DryRun.Enabled = false },
		"name":                    func(job *Job) { job.Name = "Renamed" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			changed := commandDefinitionJob(t)
			changed.DatabaseID = base.DatabaseID
			mutate(changed)
			got, err := DefinitionFingerprint(changed)
			if err != nil {
				t.Fatal(err)
			}
			if got == want {
				t.Fatalf("fingerprint não mudou: %s", got)
			}
		})
	}
}

func TestDefinitionFingerprintIgnoresRuntimeAndMetadata(t *testing.T) {
	base := commandDefinitionJob(t)
	want, err := DefinitionFingerprint(base)
	if err != nil {
		t.Fatal(err)
	}
	base.LastRun = &RunLog{RunID: "run-1", Status: RunStatusFailed}
	base.Status = JobStatusError
	base.Metadata.CreatedAt = "changed"
	base.Metadata.CreatedBy = "ignored"
	base.Description = "changed"
	base.Tags = []string{"new-tag"}
	base.Triggers = []Trigger{{Type: TriggerCron, Expression: "* * * * *"}}
	got, err := DefinitionFingerprint(base)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("estado runtime/metadata alterou fingerprint: got=%s want=%s", got, want)
	}
}

func TestDefinitionFingerprintCanonicalizesMapKeyOrder(t *testing.T) {
	a := commandDefinitionJob(t)
	b := commandDefinitionJob(t)
	b.DatabaseID = a.DatabaseID
	a.Inputs = map[string]any{"z": float64(1), "a": map[string]any{"y": true, "x": "v"}}
	b.Inputs = map[string]any{"a": map[string]any{"x": "v", "y": true}, "z": float64(1)}
	first, err := DefinitionFingerprint(a)
	if err != nil {
		t.Fatal(err)
	}
	second, err := DefinitionFingerprint(b)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("ordenação de chaves alterou fingerprint: %s != %s", first, second)
	}
}

func TestDefinitionFingerprintRejectsInvalidDefinitions(t *testing.T) {
	cases := map[string]func(*Job){
		"nil":                    func(*Job) {},
		"missing database id":    func(job *Job) { job.DatabaseID = "" },
		"non v7 database id":     func(job *Job) { job.DatabaseID = uuid.New().String() },
		"database id whitespace": func(job *Job) { job.DatabaseID = " " + job.DatabaseID },
		"empty slug":             func(job *Job) { job.ID = "" },
		"slug whitespace":        func(job *Job) { job.ID = " daily-report " },
		"invalid raw schema":     func(job *Job) { job.Output.Schema = []byte(`{"type":`) },
		"non json input value":   func(job *Job) { job.Inputs["bad"] = math.NaN() },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			var job *Job
			if name != "nil" {
				job = commandDefinitionJob(t)
			}
			mutate(job)
			if _, err := DefinitionFingerprint(job); !errors.Is(err, ErrInvalidJobDefinition) {
				t.Fatalf("erro=%v, want ErrInvalidJobDefinition", err)
			}
		})
	}
}

func TestDefinitionFingerprintSupportsLargeOutputAndDetectsChanges(t *testing.T) {
	job := commandDefinitionJob(t)
	job.Output.Schema = json.RawMessage(`{"description":"` + strings.Repeat("x", 277289) + `"}`)
	first, err := DefinitionFingerprint(job)
	if err != nil {
		t.Fatalf("schema persistido de 277 KB rejeitado: %v", err)
	}
	job.Output.Schema[len(job.Output.Schema)-3] = 'y'
	second, err := DefinitionFingerprint(job)
	if err != nil || first == second {
		t.Fatalf("alteração no schema grande não invalidou fingerprint: %v", err)
	}
	job.Output.Schema = json.RawMessage(`{"description":"` + strings.Repeat("x", 1024*1024) + `"}`)
	if _, err := DefinitionFingerprint(job); !errors.Is(err, ErrInvalidJobDefinition) {
		t.Fatalf("definição maior que 1 MiB deve ser recusada: %v", err)
	}
}

func TestDefinitionFingerprintKeepsCommandEnvelopeLimit(t *testing.T) {
	job := commandDefinitionJob(t)
	job.Inputs["large"] = strings.Repeat("x", 70000)
	if _, err := DefinitionFingerprint(job); err != nil {
		t.Fatal(err)
	}
	if _, err := commandjson.Marshal(job.Inputs); !errors.Is(err, commandjson.ErrDocumentTooLarge) {
		t.Fatalf("limite do protocolo foi ampliado: %v", err)
	}
}
