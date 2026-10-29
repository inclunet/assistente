package main

import (
	"bufio"
	"bytes"
	"errors"
	"net/url"
	"strings"
	"testing"

	"assistente/internal/apidto"
	"assistente/internal/llmcatalog"
	"assistente/internal/profiles"
)

type mockProviderSetup struct {
	profile                                     profiles.Profile
	profileUpdates                              int
	profileErr                                  error
	snapshot                                    apidto.LLMCatalogResponse
	refreshed                                   apidto.LLMCatalogResponse
	refreshes, probes, creates                  int
	probe                                       apidto.TestLLMProviderRequest
	created                                     apidto.CreateLLMProviderRequest
	defaultID                                   string
	probeErr, createErr, defaultErr, catalogErr error
	configured                                  bool
}

func (m *mockProviderSetup) GetActiveProfileAndSlug() (*profiles.ActiveProfile, error) {
	return &profiles.ActiveProfile{Slug: "active", Profile: &m.profile}, nil
}
func (m *mockProviderSetup) UpdateProfile(_ string, p profiles.Profile) error {
	m.profileUpdates++
	if m.profileErr != nil {
		return m.profileErr
	}
	m.profile = p
	return nil
}

func (m *mockProviderSetup) GetLLMCatalog() (apidto.LLMCatalogResponse, error) {
	return m.snapshot, m.catalogErr
}
func (m *mockProviderSetup) RefreshLLMCatalog() (apidto.LLMCatalogResponse, error) {
	m.refreshes++
	return m.refreshed, nil
}
func (m *mockProviderSetup) NeedsWelcomeWizard() bool { return !m.configured }
func (m *mockProviderSetup) GetCredentialForURL(raw string) (*apidto.CredentialSummary, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	return &apidto.CredentialSummary{Pattern: u.Hostname()}, nil
}
func (m *mockProviderSetup) ListModelsRaw(req apidto.TestLLMProviderRequest) ([]string, error) {
	m.probes++
	m.probe = req
	return []string{"proxy/alias-1", "model-b"}, m.probeErr
}
func (m *mockProviderSetup) CreateLLMProvider(req apidto.CreateLLMProviderRequest) (map[string]interface{}, error) {
	m.creates++
	m.created = req
	return map[string]interface{}{"id": "returned-id"}, m.createErr
}
func (m *mockProviderSetup) SetDefaultProvider(id string) error {
	m.defaultID = id
	return m.defaultErr
}

func catalogFixture() apidto.LLMCatalogResponse {
	return apidto.LLMCatalogResponse{Document: llmcatalog.Document{Servers: []llmcatalog.Server{{
		ID: "openai", Name: "Nome vindo do catálogo", Status: "active", ProviderFamily: "openai", APIFormat: "openai_responses", BaseURL: "https://api.openai.com/v1", DefaultModelID: "catalog-model",
		Auth: llmcatalog.Auth{Mode: "bearer_api_key", Required: true}, SignupURL: "https://example.com/signup", APIKeyURL: "https://example.com/keys",
		Fields: map[string]llmcatalog.FieldPolicy{"base_url": {Policy: "fixed"}, "api_format": {Policy: "hidden"}, "auth.mode": {Policy: "fixed"}, "default_model_id": {Policy: "editable"}},
	}}}}
}

func TestProviderSetupSharedCatalogFlow(t *testing.T) {
	for _, setup := range []bool{false, true} {
		t.Run(map[bool]string{false: "add", true: "setup"}[setup], func(t *testing.T) {
			m := &mockProviderSetup{snapshot: catalogFixture()}
			var out bytes.Buffer
			r := bufio.NewReader(strings.NewReader("1\n\n\nproxy/alias-1\n"))
			var err error
			if setup {
				err = runSetup(m, &out, r, fakePasswordReader("secret"))
			} else {
				err = configureProvider(m, &out, r, fakePasswordReader("secret"), false)
			}
			if err != nil {
				t.Fatal(err)
			}
			if m.probes != 1 || m.creates != 1 || m.refreshes != 0 {
				t.Fatalf("calls: %+v", m)
			}
			if m.created.ServerProfileID != "openai" || m.created.APIFormat != "openai_responses" || m.created.DefaultModel != "proxy/alias-1" || m.created.Name != "Nome vindo do catálogo" || m.created.ID == "" {
				t.Fatalf("request: %+v", m.created)
			}
			if m.created.Credential == nil || m.created.Credential.Token != "secret" || m.created.Credential.Pattern != "api.openai.com" || m.created.APIKey != "" {
				t.Fatal("credential draft missing")
			}
			if m.probe.Credential != m.created.Credential || m.probe.BaseURL != m.created.BaseURL || m.probe.APIFormat != m.created.APIFormat {
				t.Fatal("probe differs from persisted connection")
			}
			if setup && m.defaultID != "returned-id" || !setup && m.defaultID != "" {
				t.Fatalf("default %q", m.defaultID)
			}
			if !strings.Contains(out.String(), "https://example.com/keys") {
				t.Fatal("missing provider link")
			}
		})
	}
}

func TestProviderSetupManualHTTPConsentBeforeProbe(t *testing.T) {
	for _, accept := range []bool{false, true} {
		m := &mockProviderSetup{}
		answer := "n"
		if accept {
			answer = "s"
		}
		input := "n\n1\nProxy\nhttp://remote.example:4000/v1\n1\n1\n" + answer + "\n\nproxy/alias-1\n"
		err := configureProvider(m, &bytes.Buffer{}, bufio.NewReader(strings.NewReader(input)), fakePasswordReader("secret"), false)
		if !accept {
			if err == nil || m.probes != 0 || m.creates != 0 {
				t.Fatal("denial must prevent network and persistence")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if m.probe.InsecureHTTPAuthorizedURL != "http://remote.example:4000/v1" || m.created.InsecureHTTPAuthorizedURL != m.probe.InsecureHTTPAuthorizedURL || m.created.ServerProfileID != "" {
			t.Fatalf("consent: %+v", m.created)
		}
	}
}

func TestProviderSetupNoAuthOffline(t *testing.T) {
	m := &mockProviderSetup{}
	input := "n\n1\nLocal\nhttp://localhost:8080/v1\n1\n3\n\nlocal-alias\n"
	err := configureProvider(m, &bytes.Buffer{}, bufio.NewReader(strings.NewReader(input)), fakePasswordReader(), false)
	if err != nil {
		t.Fatal(err)
	}
	if m.created.Credential != nil || m.created.AuthMode != "none" || m.created.DefaultModel != "local-alias" {
		t.Fatal("manual no-auth connection changed")
	}
}

func TestProviderSetupFailures(t *testing.T) {
	for _, tc := range []struct {
		name, input, key                            string
		probeErr, createErr, defaultErr, catalogErr error
		probes, creates                             int
		setup                                       bool
	}{
		{name: "invalid choice", input: "99\n"},
		{name: "EOF", input: "1\n"},
		{name: "empty key", input: "1\n\n"},
		{name: "probe declined", input: "1\n\n\nn\n", key: "key", probeErr: errors.New("offline"), probes: 1},
		{name: "create failure", input: "1\n\n\nmodel\n", key: "key", createErr: errors.New("db"), probes: 1, creates: 1},
		{name: "default failure", input: "1\n\n\nmodel\n", key: "key", defaultErr: errors.New("db"), probes: 1, creates: 1, setup: true},
		{name: "catalog auth failure", catalogErr: errors.New("unauthenticated")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &mockProviderSetup{snapshot: catalogFixture(), probeErr: tc.probeErr, createErr: tc.createErr, defaultErr: tc.defaultErr, catalogErr: tc.catalogErr}
			err := configureProvider(m, &bytes.Buffer{}, bufio.NewReader(strings.NewReader(tc.input)), fakePasswordReader(tc.key), tc.setup)
			if err == nil || m.probes != tc.probes || m.creates != tc.creates {
				t.Fatalf("err=%v probes=%d creates=%d", err, m.probes, m.creates)
			}
		})
	}
}

func TestProviderSetupProbeFailureCanKeepExactManualModel(t *testing.T) {
	m := &mockProviderSetup{snapshot: catalogFixture(), probeErr: errors.New("offline")}
	err := configureProvider(m, &bytes.Buffer{}, bufio.NewReader(strings.NewReader("1\n\n\ns\nmy/proxy-alias\n")), fakePasswordReader("key"), false)
	if err != nil || m.created.DefaultModel != "my/proxy-alias" {
		t.Fatalf("%v %+v", err, m.created)
	}
}

func TestProviderSetupRefreshAndFieldPolicies(t *testing.T) {
	snapshot := catalogFixture()
	snapshot.Document.Servers[0].Fields["default_model_id"] = llmcatalog.FieldPolicy{Policy: "choice", Choices: []any{"model-a", "model-b"}, Default: "model-a"}
	m := &mockProviderSetup{refreshed: snapshot}
	err := configureProvider(m, &bytes.Buffer{}, bufio.NewReader(strings.NewReader("s\n1\n\n\n2\n")), fakePasswordReader("key"), false)
	if err != nil || m.refreshes != 1 || m.created.DefaultModel != "model-b" {
		t.Fatalf("%v %+v", err, m)
	}
	for _, policy := range []string{"fixed", "hidden"} {
		snapshot.Document.Servers[0].Fields["default_model_id"] = llmcatalog.FieldPolicy{Policy: policy, Default: "model-fixed"}
		m := &mockProviderSetup{snapshot: snapshot}
		err := configureProvider(m, &bytes.Buffer{}, bufio.NewReader(strings.NewReader("1\n\n\n")), fakePasswordReader("key"), false)
		if err != nil || m.created.DefaultModel != "model-fixed" {
			t.Fatalf("%s: %v %+v", policy, err, m.created)
		}
	}
}

func TestProviderSetupUnsupportedProfilesAreNotOffered(t *testing.T) {
	snapshot := catalogFixture()
	snapshot.Document.Servers[0].Status = "deprecated"
	m := &mockProviderSetup{snapshot: snapshot}
	var out bytes.Buffer
	_ = configureProvider(m, &out, bufio.NewReader(strings.NewReader("99\n")), fakePasswordReader(), false)
	if strings.Contains(out.String(), "Nome vindo") || !strings.Contains(out.String(), "Configuração manual") {
		t.Fatal(out.String())
	}
}

func TestRunSetupConfiguredDeclineDoesNotReadCatalog(t *testing.T) {
	m := &mockProviderSetup{configured: true, catalogErr: errors.New("must not read")}
	if err := runSetup(m, &bytes.Buffer{}, bufio.NewReader(strings.NewReader("n\n")), fakePasswordReader()); err != nil {
		t.Fatal(err)
	}
}

func TestProviderSetupAnthropicUsesHeaderCredential(t *testing.T) {
	snapshot := catalogFixture()
	s := &snapshot.Document.Servers[0]
	s.ID, s.ProviderFamily, s.APIFormat, s.BaseURL = "anthropic", "anthropic", "anthropic", "https://api.anthropic.com/v1"
	s.Auth.Mode = "api_key_header"
	m := &mockProviderSetup{snapshot: snapshot}
	err := configureProvider(m, &bytes.Buffer{}, bufio.NewReader(strings.NewReader("1\n\n\nclaude-alias\n")), fakePasswordReader("secret"), false)
	if err != nil {
		t.Fatal(err)
	}
	c := m.created.Credential
	if c == nil || c.Type != "custom" || c.HeaderName != "x-api-key" || c.HeaderValue != "secret" || c.Token != "" || m.probe.Credential != c {
		t.Fatalf("wrong authentication draft: %+v", c)
	}
}

func TestProviderSetupReconfigurationReplacesActiveProfileOverrides(t *testing.T) {
	for _, setup := range []bool{false, true} {
		m := &mockProviderSetup{snapshot: catalogFixture()}
		m.profile.Chat.Model = "old-model"
		m.profile.Chat.LLMProvider = "old-provider"
		err := configureProvider(m, &bytes.Buffer{}, bufio.NewReader(strings.NewReader("1\n\n\nnew-model\n")), fakePasswordReader("secret"), setup)
		if err != nil {
			t.Fatal(err)
		}
		if setup {
			if m.profile.Chat.Model != profiles.DefaultProviderSentinel || m.profile.Chat.LLMProvider != profiles.DefaultProviderSentinel || m.profileUpdates != 1 {
				t.Fatal("setup left active profile pinned to old connection")
			}
		} else if m.profileUpdates != 0 || m.profile.Chat.Model != "old-model" || m.profile.Chat.LLMProvider != "old-provider" {
			t.Fatal("add changed active profile")
		}
	}
}

func TestProviderSetupProfileFailureReportsSavedConnection(t *testing.T) {
	m := &mockProviderSetup{snapshot: catalogFixture(), profileErr: errors.New("profile db")}
	err := configureProvider(m, &bytes.Buffer{}, bufio.NewReader(strings.NewReader("1\n\n\nnew-model\n")), fakePasswordReader("secret"), true)
	if err == nil || !strings.Contains(err.Error(), "returned-id criado") || m.creates != 1 {
		t.Fatalf("%v", err)
	}
}
