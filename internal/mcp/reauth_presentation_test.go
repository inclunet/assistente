package mcp

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"assistente/internal/oauthflow"
)

func TestManagedTransportPublishesReauthorizationOnceAndClearsOnRecovery(t *testing.T) {
	m, _, ctx := managedFixture(t)
	emitter := &capturingEmitter{}
	m.emitEvent = emitter.emit
	if err := m.SaveConfig("reauth", managedConfig("https://resource.example")); err != nil {
		t.Fatal(err)
	}
	cfg, store, record := loadManaged(t, m, ctx, "reauth")
	client := m.buildAuthHTTPClient(ctx, cfg.Slug, cfg)
	for range 2 {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.Do(req)
		if !errors.Is(err, oauthflow.ErrReauthorize) {
			t.Fatalf("expected reauthorization: %v", err)
		}
	}
	if !m.List()[0].NeedsReauth || emitter.countByName("mcp:server_needs_reauth") != 1 {
		t.Fatal("missing or repeated notice")
	}
	record.State = "connected"
	record.Tokens = oauthflow.Tokens{Access: "VALID", Type: "Bearer", ExpiresAt: time.Now().Add(time.Hour)}
	record.Revision++
	if err := store.CompareAndSwap(ctx, record, record.Revision-1); err != nil {
		t.Fatal(err)
	}
	if _, err := m.resolveManagedOAuth(ctx, cfg, ""); err != nil {
		t.Fatal(err)
	}
	if m.List()[0].NeedsReauth || emitter.countByName("mcp:server_reauthorized") != 1 {
		t.Fatal("recovery did not clear notice")
	}
}

func TestManagedReauthNoticeIgnoresCanceledAndReplacedConsumers(t *testing.T) {
	m, _, ctx := managedFixture(t)
	emitter := &capturingEmitter{}
	m.emitEvent = emitter.emit
	if err := m.SaveConfig("reauth", managedConfig("https://resource.example")); err != nil {
		t.Fatal(err)
	}
	cfg, _, _ := loadManaged(t, m, ctx, "reauth")
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, _ = m.resolveManagedOAuth(canceled, cfg, "")
	if emitter.countByName("mcp:server_needs_reauth") != 0 {
		t.Fatal("canceled request published notice")
	}
	replaced := cfg
	replaced.OAuthAuthorizationID = "obsolete"
	m.signalNeedsReauth(replaced, "oauth_reauthorization_required")
	if m.List()[0].NeedsReauth {
		t.Fatal("obsolete authorization changed current status")
	}
}

func TestReauthorizationRecoveryDoesNotClearReplacementNotice(t *testing.T) {
	m, _, ctx := managedFixture(t)
	if err := m.SaveConfig("reauth", managedConfig("https://resource.example")); err != nil {
		t.Fatal(err)
	}
	cfg, _, _ := loadManaged(t, m, ctx, "reauth")
	m.signalNeedsReauth(cfg, "oauth_reauthorization_required")
	obsolete := cfg
	obsolete.OAuthAuthorizationID = "obsolete"
	m.clearNeedsReauth(obsolete)
	if !m.List()[0].NeedsReauth {
		t.Fatal("obsolete recovery cleared current notice")
	}
}
