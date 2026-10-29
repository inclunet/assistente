package main

import (
	"bufio"
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"assistente/internal/app"
	"assistente/internal/database"
)

type mockProviderAuth struct {
	status    app.AuthStatus
	calls     []string
	fail      string
	refreshOK bool
	login     app.LoginRequest
}

func (m *mockProviderAuth) call(name string) error {
	m.calls = append(m.calls, name)
	if m.fail == name {
		return errors.New(name)
	}
	return nil
}
func (m *mockProviderAuth) GetAuthStatus() (app.AuthStatus, error) { return m.status, m.call("status") }
func (m *mockProviderAuth) SetupVault(string) (string, error) {
	if err := m.call("setup"); err != nil {
		return "", err
	}
	m.status.VaultConfigured = true
	m.status.VaultUnlocked = true
	return "recovery", nil
}
func (m *mockProviderAuth) UnlockVault(string, string) error {
	if err := m.call("unlock"); err != nil {
		return err
	}
	m.status.VaultUnlocked = true
	return nil
}
func (m *mockProviderAuth) CreateAdminUser(app.CreateAdminRequest) (*database.User, error) {
	if err := m.call("admin"); err != nil {
		return nil, err
	}
	m.status.HasUsers = true
	return &database.User{}, nil
}
func (m *mockProviderAuth) Login(req app.LoginRequest) (*app.AuthUser, error) {
	m.login = req
	if err := m.call("login"); err != nil {
		return nil, err
	}
	return &app.AuthUser{UserID: "user", SessionID: "session"}, nil
}
func (m *mockProviderAuth) RefreshAuth(app.RefreshRequest) (*app.AuthUser, error) {
	if err := m.call("refresh"); err != nil {
		return nil, err
	}
	if !m.refreshOK {
		return nil, errors.New("expired")
	}
	return &app.AuthUser{UserID: "user", SessionID: "session"}, nil
}

func TestProviderAuthNewInstallationUsesExistingContract(t *testing.T) {
	m := &mockProviderAuth{}
	var out bytes.Buffer
	err := prepareProviderSession(m, &out, bufio.NewReader(strings.NewReader("\nalice\n")), fakePasswordReader("master123", "master123", "account123", "account123"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m.calls, []string{"status", "setup", "status", "admin", "login"}) || m.login.Username != "alice" {
		t.Fatalf("%+v", m)
	}
	if !strings.Contains(out.String(), "recovery") {
		t.Fatal("missing recovery key")
	}
}

func TestProviderAuthUnlockAndRestore(t *testing.T) {
	m := &mockProviderAuth{status: app.AuthStatus{VaultConfigured: true, HasUsers: true}, refreshOK: true}
	err := prepareProviderSession(m, &bytes.Buffer{}, bufio.NewReader(strings.NewReader("")), fakePasswordReader("master123"))
	if err != nil || !reflect.DeepEqual(m.calls, []string{"status", "unlock", "status", "refresh"}) {
		t.Fatalf("%v %+v", err, m)
	}
}

func TestProviderAuthExpiredSessionRequestsLogin(t *testing.T) {
	m := &mockProviderAuth{status: app.AuthStatus{VaultConfigured: true, VaultUnlocked: true, HasUsers: true}}
	err := prepareProviderSession(m, &bytes.Buffer{}, bufio.NewReader(strings.NewReader("alice\n")), fakePasswordReader("account123"))
	if err != nil || m.login.Username != "alice" || m.login.Password != "account123" {
		t.Fatalf("%v %+v", err, m)
	}
}

func TestProviderAuthFailureStopsFollowingSteps(t *testing.T) {
	for _, step := range []string{"status", "unlock", "admin", "login"} {
		t.Run(step, func(t *testing.T) {
			m := &mockProviderAuth{status: app.AuthStatus{VaultConfigured: true, HasUsers: step != "admin"}, fail: step}
			err := prepareProviderSession(m, &bytes.Buffer{}, bufio.NewReader(strings.NewReader("alice\n")), fakePasswordReader("master123", "account123", "account123"))
			if err == nil || m.calls[len(m.calls)-1] != step {
				t.Fatalf("%v %+v", err, m.calls)
			}
		})
	}
}

func TestProviderAuthPipeUsesSingleBufferAndPreservesSecret(t *testing.T) {
	m := &mockProviderAuth{}
	r := bufio.NewReader(strings.NewReader(" master123 \n master123 \n\nalice\n account123 \n account123 \nnext-input\n"))
	var out bytes.Buffer
	if err := prepareProviderSession(m, &out, r, pipePasswordReader(r, &out)); err != nil {
		t.Fatal(err)
	}
	if m.login.Password != " account123 " {
		t.Fatal("password whitespace changed")
	}
	next, err := r.ReadString('\n')
	if err != nil || next != "next-input\n" {
		t.Fatalf("input lost: %q %v", next, err)
	}
	if strings.Contains(out.String(), "account123") {
		t.Fatal("secret echoed")
	}
}
