package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"assistente/internal/database"
	"assistente/internal/jobprofilegrant"
	"assistente/internal/tools"
)

type commandProfileTargetSecretStore struct {
	mu     sync.Mutex
	values map[string]string
	errors map[string]error
	calls  []string
}

func (s *commandProfileTargetSecretStore) GetSecret(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, key)
	if err := s.errors[key]; err != nil {
		return "", err
	}
	return s.values[key], nil
}

func (s *commandProfileTargetSecretStore) set(key, value string) {
	s.mu.Lock()
	s.values[key] = value
	s.mu.Unlock()
}

func (s *commandProfileTargetSecretStore) callsFor(key string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, called := range s.calls {
		if called == key {
			count++
		}
	}
	return count
}

func newCommandProfileTargetFixture(t *testing.T, inputs map[string]any) (*Manager, *DBRepository, context.Context, *Job, *commandProfileTargetSecretStore) {
	t.Helper()
	repo, ownerCtx, _ := setupJobsRepositoryTest(t)
	if err := repo.db.Create(&database.ToolCatalog{
		Name: "subagent", DisplayName: "subagent", Origin: "builtin", AvailabilityStatus: "available",
	}).Error; err != nil {
		t.Fatalf("seed subagent tool catalog: %v", err)
	}
	secrets := &commandProfileTargetSecretStore{
		values: make(map[string]string),
		errors: make(map[string]error),
	}
	m := mustNewManager(t, ManagerConfig{
		Repository: repo,
		ContextProvider: func() context.Context {
			return ownerCtx
		},
		SecretStore: secrets,
	})
	t.Cleanup(m.Stop)
	job := &Job{
		ID:       "command-profile-target",
		Name:     "Command profile target",
		Enabled:  true,
		Tool:     jobprofilegrant.ToolSubagent,
		Inputs:   inputs,
		Triggers: []Trigger{{Type: TriggerManual}},
	}
	if err := repo.SaveJob(ownerCtx, job); err != nil {
		t.Fatalf("save job: %v", err)
	}
	job, err := repo.GetJob(ownerCtx, job.ID)
	if err != nil {
		t.Fatalf("reload job: %v", err)
	}
	return m, repo, ownerCtx, job, secrets
}

func TestPrepareCommandProfileTargetOwnerAndInheritance(t *testing.T) {
	t.Run("denies different owner", func(t *testing.T) {
		manager, _, ownerCtx, job, _ := newCommandProfileTargetFixture(t, map[string]any{"profile": "profile-a"})
		otherOwner := database.WithUserID(context.Background(), "user-b")
		if _, err := manager.PrepareCommandProfileTarget(otherOwner, job.DatabaseID); !errors.Is(err, ErrCommandJobDenied) {
			t.Fatalf("error = %v, want ErrCommandJobDenied", err)
		}
		if _, err := manager.PrepareCommandProfileTarget(ownerCtx, job.DatabaseID); err != nil {
			t.Fatalf("owner preparation failed after denied request: %v", err)
		}
	})

	t.Run("missing profile inherits blank target", func(t *testing.T) {
		manager, _, ownerCtx, job, _ := newCommandProfileTargetFixture(t, map[string]any{"prompt": "hello"})
		target, err := manager.PrepareCommandProfileTarget(ownerCtx, job.DatabaseID)
		if err != nil {
			t.Fatal(err)
		}
		if target.Slug != "" || target.Expression != "" || target.DefinitionFingerprint == "" {
			t.Fatalf("target = %+v, want blank inherited profile and fingerprint", target)
		}
	})
}

func TestPrepareCommandProfileTargetLiteralAndTemplateConstant(t *testing.T) {
	cases := []struct {
		name       string
		expression string
		wantSlug   string
	}{
		{name: "literal", expression: " profile-literal ", wantSlug: "profile-literal"},
		{name: "template constant", expression: `{{ "profile-template" }}`, wantSlug: "profile-template"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manager, _, ownerCtx, job, _ := newCommandProfileTargetFixture(t, map[string]any{"profile": tc.expression})
			target, err := manager.PrepareCommandProfileTarget(ownerCtx, job.DatabaseID)
			if err != nil {
				t.Fatal(err)
			}
			if target.Slug != tc.wantSlug || target.Expression != strings.TrimSpace(tc.expression) || target.DefinitionFingerprint == "" {
				t.Fatalf("target = %+v, want slug=%q expression=%q and fingerprint", target, tc.wantSlug, strings.TrimSpace(tc.expression))
			}
		})
	}
}

func TestPrepareCommandProfileTargetRejectsExplicitInvalidEmptyAndMissingEvent(t *testing.T) {
	cases := []struct {
		name    string
		profile any
	}{
		{name: "invalid non-string", profile: 42},
		{name: "explicit empty template", profile: `{{ "" }}`},
		{name: "missing event does not inherit", profile: `{{ .event.profile }}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manager, _, ownerCtx, job, _ := newCommandProfileTargetFixture(t, map[string]any{"profile": tc.profile})
			if target, err := manager.PrepareCommandProfileTarget(ownerCtx, job.DatabaseID); !errors.Is(err, ErrCommandJobDenied) || target != (CommandProfileTarget{}) {
				t.Fatalf("target=%+v err=%v, want empty target and ErrCommandJobDenied", target, err)
			}
		})
	}
}

func TestPrepareCommandProfileTargetUsesOnlyProfileSecretAndRedactsFailure(t *testing.T) {
	t.Run("reuses secret store for profile", func(t *testing.T) {
		manager, _, ownerCtx, job, secrets := newCommandProfileTargetFixture(t, map[string]any{
			"profile": `{{ secret "profile_slug" }}`,
			"prompt":  `{{ secret "unrelated" }}`,
		})
		secrets.set("profile_slug", "profile-from-secret")
		secrets.set("unrelated", "must-not-be-read")

		target, err := manager.PrepareCommandProfileTarget(ownerCtx, job.DatabaseID)
		if err != nil {
			t.Fatal(err)
		}
		if target.Slug != "profile-from-secret" {
			t.Fatalf("target slug = %q, want profile-from-secret", target.Slug)
		}
		if secrets.callsFor("profile_slug") != 1 {
			t.Fatalf("profile secret calls = %d, want 1", secrets.callsFor("profile_slug"))
		}
		if secrets.callsFor("unrelated") != 0 {
			t.Fatalf("unrelated secret calls = %d, want 0", secrets.callsFor("unrelated"))
		}
	})

	t.Run("secret failure is redacted", func(t *testing.T) {
		manager, _, ownerCtx, job, secrets := newCommandProfileTargetFixture(t, map[string]any{
			"profile": `{{ secret "broken_profile" }}`,
		})
		secret := "do-not-leak-profile-secret"
		secrets.errors["broken_profile"] = fmt.Errorf("vault payload contains %s", secret)

		target, err := manager.PrepareCommandProfileTarget(ownerCtx, job.DatabaseID)
		if !errors.Is(err, ErrCommandJobDenied) {
			t.Fatalf("error = %v, want ErrCommandJobDenied", err)
		}
		if target != (CommandProfileTarget{}) {
			t.Fatalf("target = %+v, want zero target", target)
		}
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("secret leaked in preparation error: %v", err)
		}
	})
}

type commandProfileTargetProbeTool struct {
	calls          atomic.Int32
	permanentFirst bool
	transientFirst bool
	onCall         func(int)
}

func (t *commandProfileTargetProbeTool) Name() string { return jobprofilegrant.ToolSubagent }

func (t *commandProfileTargetProbeTool) Description() string { return "profile target test tool" }

func (t *commandProfileTargetProbeTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"profile":{"type":"string"}}}`)
}

func (t *commandProfileTargetProbeTool) Execute(_ context.Context, _ json.RawMessage) (tools.ToolResult, error) {
	call := int(t.calls.Add(1))
	if t.onCall != nil {
		t.onCall(call)
	}
	if t.permanentFirst && call == 1 {
		return tools.ToolResult{IsError: true, Failure: &tools.ToolFailure{
			Code: "profile_target_permanent", Kind: tools.ErrorKindConfiguration, Retryable: false,
		}}, nil
	}
	if t.transientFirst && call == 1 {
		return tools.ToolResult{IsError: true, Failure: &tools.ToolFailure{
			Code: "profile_target_retry", Kind: tools.ErrorKindUnavailable, Retryable: true,
		}}, nil
	}
	return tools.ToolResult{Content: `{"ok":true}`}, nil
}

func TestCommandExecutorRejectsChangedSecretProfileBeforeFirstTool(t *testing.T) {
	secrets := &commandProfileTargetSecretStore{values: map[string]string{"profile_slug": "profile-new"}, errors: make(map[string]error)}
	probe := &commandProfileTargetProbeTool{}
	registry := tools.NewRegistry()
	registry.MustRegister(probe)
	executor := mustNewJobExecutor(t, ExecutorConfig{ToolRegistry: registry, SecretStore: secrets})
	ctx := context.WithValue(context.Background(), commandJobDispatchKey{}, commandJobDispatch{
		jobID:         "job-db-id",
		profileTarget: "profile-old",
		revalidate:    func(context.Context) error { return nil },
	})

	_, err := executor.executeSingle(ctx, &Job{
		DatabaseID: "job-db-id",
		ID:         "profile-target-job",
		Tool:       jobprofilegrant.ToolSubagent,
		Inputs:     map[string]any{"profile": `{{ secret "profile_slug" }}`},
	}, &TriggerContext{EventPayload: map[string]any{}}, nil)
	failure := requireAttemptFailure(t, err)
	if failure.code != "command_job_profile_changed" {
		t.Fatalf("failure code = %q, want command_job_profile_changed", failure.code)
	}
	if got := probe.calls.Load(); got != 0 {
		t.Fatalf("tool calls = %d, want 0", got)
	}
}

func TestCommandExecutorAllowsUnchangedSecretProfileToReachTool(t *testing.T) {
	secrets := &commandProfileTargetSecretStore{values: map[string]string{"profile_slug": "profile-same"}, errors: make(map[string]error)}
	probe := &commandProfileTargetProbeTool{permanentFirst: true}
	registry := tools.NewRegistry()
	registry.MustRegister(probe)
	executor := mustNewJobExecutor(t, ExecutorConfig{ToolRegistry: registry, SecretStore: secrets})
	ctx := context.WithValue(context.Background(), commandJobDispatchKey{}, commandJobDispatch{
		jobID:         "job-db-id",
		profileTarget: "profile-same",
		revalidate:    func(context.Context) error { return nil },
	})

	_, err := executor.executeSingle(ctx, &Job{
		DatabaseID: "job-db-id",
		ID:         "profile-target-job",
		Tool:       jobprofilegrant.ToolSubagent,
		Inputs:     map[string]any{"profile": `{{ secret "profile_slug" }}`},
	}, &TriggerContext{EventPayload: map[string]any{}}, nil)
	failure := requireAttemptFailure(t, err)
	if failure.code != "profile_target_permanent" {
		t.Fatalf("failure code = %q, want profile_target_permanent", failure.code)
	}
	if failure.retryable {
		t.Fatal("permanent tool failure must not be retryable")
	}
	if got := probe.calls.Load(); got != 1 {
		t.Fatalf("tool calls = %d, want exactly one", got)
	}
}

func TestCommandExecutorRejectsChangedSecretProfileBeforeRetryTool(t *testing.T) {
	secrets := &commandProfileTargetSecretStore{values: map[string]string{"profile_slug": "profile-old"}, errors: make(map[string]error)}
	probe := &commandProfileTargetProbeTool{
		transientFirst: true,
		onCall: func(call int) {
			if call == 1 {
				secrets.set("profile_slug", "profile-new")
			}
		},
	}
	registry := tools.NewRegistry()
	registry.MustRegister(probe)
	executor := mustNewJobExecutor(t, ExecutorConfig{ToolRegistry: registry, SecretStore: secrets, CircuitBreaker: NewCircuitBreaker()})
	ctx := context.WithValue(context.Background(), commandJobDispatchKey{}, commandJobDispatch{
		jobID:         "job-db-id",
		profileTarget: "profile-old",
		revalidate: func(context.Context) error {
			return nil
		},
	})

	job := &Job{
		DatabaseID:  "job-db-id",
		ID:          "profile-target-job",
		Tool:        jobprofilegrant.ToolSubagent,
		Inputs:      map[string]any{"profile": `{{ secret "profile_slug" }}`},
		ErrorPolicy: ErrorPolicy{Strategy: ErrorRetry, MaxRetries: 1, RetryDelay: "1ms"},
	}
	run := executor.Execute(ctx, job, &TriggerContext{Type: TriggerManual, EventPayload: map[string]any{}})
	if run == nil || run.Status != RunStatusFailed {
		t.Fatalf("run = %+v, want failed run", run)
	}
	if got := probe.calls.Load(); got != 1 {
		t.Fatalf("tool calls = %d, want exactly one first-attempt call", got)
	}
}
