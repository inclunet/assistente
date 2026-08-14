package wailsapi

import (
	"context"
	"errors"
	"testing"
)

type stubWelcomeRuntime struct {
	appCtx         context.Context
	loggedIn       bool
	hasMasterKey   bool
	hasMasterErr   error
	userCount      int64
	userCountErr   error
	providerCount  int64
	providerErr    error
	appContextCalls int
}

func (s *stubWelcomeRuntime) AppContext() context.Context {
	s.appContextCalls++
	if s.appCtx != nil {
		return s.appCtx
	}
	return context.Background()
}

func (s *stubWelcomeRuntime) IsLoggedIn() bool { return s.loggedIn }

func (s *stubWelcomeRuntime) HasMasterKey() (bool, error) {
	return s.hasMasterKey, s.hasMasterErr
}

func (s *stubWelcomeRuntime) UserCount() (int64, error) {
	return s.userCount, s.userCountErr
}

func (s *stubWelcomeRuntime) ProviderCount(ctx context.Context) (int64, error) {
	return s.providerCount, s.providerErr
}

func TestWelcomeNotWired(t *testing.T) {
	t.Parallel()
	api := NewWelcome()
	if !api.NeedsWelcomeWizard() {
		t.Fatal("NeedsWelcomeWizard com bind vazio deve ser fail-safe true")
	}
	if _, err := api.RunWelcomeWizard(); !errors.Is(err, ErrWelcomeNotWired) {
		t.Fatalf("RunWelcomeWizard: got %v, want ErrWelcomeNotWired", err)
	}
}

func TestEvaluateNeedsWelcomeWizard(t *testing.T) {
	t.Parallel()
	authErr := errors.New("auth failed")
	masterErr := errors.New("master key check failed")
	userErr := errors.New("user count failed")
	providerErr := errors.New("provider count failed")

	tests := []struct {
		name    string
		session Session
		runtime WelcomeRuntime
		want    bool
	}{
		{
			name:    "runtime nil → true",
			session: nil,
			runtime: nil,
			want:    true,
		},
		{
			name:    "HasMasterKey erro → true",
			session: nil,
			runtime: &stubWelcomeRuntime{hasMasterErr: masterErr},
			want:    true,
		},
		{
			name:    "pré-login sem master key → true",
			session: nil,
			runtime: &stubWelcomeRuntime{loggedIn: false, hasMasterKey: false, userCount: 1},
			want:    true,
		},
		{
			name:    "pré-login com master + userCount 0 → true",
			session: nil,
			runtime: &stubWelcomeRuntime{loggedIn: false, hasMasterKey: true, userCount: 0},
			want:    true,
		},
		{
			name:    "pré-login com master + userCount >0 → false",
			session: nil,
			runtime: &stubWelcomeRuntime{loggedIn: false, hasMasterKey: true, userCount: 2},
			want:    false,
		},
		{
			name:    "UserCount erro → true",
			session: nil,
			runtime: &stubWelcomeRuntime{loggedIn: false, hasMasterKey: true, userCountErr: userErr},
			want:    true,
		},
		{
			name:    "pós-login session nil → true",
			session: nil,
			runtime: &stubWelcomeRuntime{loggedIn: true, hasMasterKey: true},
			want:    true,
		},
		{
			name:    "pós-login AuthenticatedContext erro → true",
			session: stubSession{err: authErr},
			runtime: &stubWelcomeRuntime{loggedIn: true, hasMasterKey: true},
			want:    true,
		},
		{
			name:    "pós-login ctrl nil ProviderCount 0 → true",
			session: stubSession{ctx: context.Background()},
			runtime: &stubWelcomeRuntime{loggedIn: true, hasMasterKey: true, providerCount: 0},
			want:    true,
		},
		{
			name:    "pós-login ctrl nil ProviderCount >0 + hasMaster → false",
			session: stubSession{ctx: context.Background()},
			runtime: &stubWelcomeRuntime{loggedIn: true, hasMasterKey: true, providerCount: 3},
			want:    false,
		},
		{
			name:    "pós-login ctrl nil ProviderCount >0 sem master → true",
			session: stubSession{ctx: context.Background()},
			runtime: &stubWelcomeRuntime{loggedIn: true, hasMasterKey: false, providerCount: 3},
			want:    true,
		},
		{
			name:    "pós-login ctrl nil ProviderCount erro → true",
			session: stubSession{ctx: context.Background()},
			runtime: &stubWelcomeRuntime{loggedIn: true, hasMasterKey: true, providerErr: providerErr},
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := EvaluateNeedsWelcomeWizard(tt.session, nil, tt.runtime)
			if got != tt.want {
				t.Fatalf("EvaluateNeedsWelcomeWizard() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWelcomeNeedsWelcomeWizardUsesAttachedRuntime(t *testing.T) {
	t.Parallel()
	api := NewWelcome()
	rt := &stubWelcomeRuntime{loggedIn: false, hasMasterKey: true, userCount: 2}
	AttachWelcome(api, nil, nil, rt)
	if api.NeedsWelcomeWizard() {
		t.Fatal("NeedsWelcomeWizard após Attach com master+users deve ser false")
	}
}

func TestRunWelcomeWizardNotWiredWithoutCtrl(t *testing.T) {
	t.Parallel()
	api := NewWelcome()
	rt := &stubWelcomeRuntime{appCtx: context.Background()}
	AttachWelcome(api, stubSession{ctx: context.Background()}, nil, rt)
	if _, err := api.RunWelcomeWizard(); !errors.Is(err, ErrWelcomeNotWired) {
		t.Fatalf("RunWelcomeWizard sem ctrl: got %v, want ErrWelcomeNotWired", err)
	}
	if rt.appContextCalls != 0 {
		t.Fatalf("AppContext não deve ser chamado quando ctrl é nil; calls=%d", rt.appContextCalls)
	}
}
