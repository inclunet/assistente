package commandexecution

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandledger"
	"assistente/internal/commandsecurity"
)

// Dependências sem I/O: estes testes exercitam somente validação de bootstrap.
func configOnlyFixture(t *testing.T) Config {
	t.Helper()
	locales := map[string]commandcatalog.LocalizedMetadata{}
	for _, locale := range []string{"pt-BR", "en", "es"} {
		locales[locale] = commandcatalog.LocalizedMetadata{Name: "Fixture", Description: "Fixture", Category: "Fixture"}
	}
	registry, err := commandcatalog.New([]commandcatalog.Registration{{Definition: commandcatalog.Definition{ID: "fixture.read", Effect: commandcatalog.Read, Decision: commandcatalog.NoDecision, Context: commandcatalog.ContextPolicy{None: true}, AllowedSources: []commandcatalog.Source{commandcatalog.Palette}, Presentation: &commandcatalog.Presentation{Version: "1", Locales: locales}}, Handler: commandcatalog.HandlerContract{Effect: commandcatalog.Read}}})
	if err != nil {
		t.Fatal(err)
	}
	epochs, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	return Config{Sessions: &auth.SessionService{}, Epochs: epochs, Store: &commandledger.Store{}, Registry: registry, RegistryVersion: "1", Source: commandcatalog.Palette,
		Snapshot:  func(context.Context, auth.LocalSessionPrincipal) (Versions, error) { return Versions{}, nil },
		Authorize: func(context.Context, auth.LocalSessionPrincipal, string, commandcatalog.Source) error { return nil },
		Keys:      func(context.Context, string) ([]byte, error) { return nil, nil }, KeyVersion: "v1", Now: time.Now, Retention: time.Hour, ExecutionTimeout: time.Minute, FinalizationTimeout: time.Second,
		Handlers: map[string]Handler{"fixture.read": {Contract: commandcatalog.HandlerContract{Effect: commandcatalog.Read}, Start: func(context.Context, Invocation) (ExecutionHandle, error) { return ExecutionHandle{}, nil }}},
	}
}

func TestNewRejectsIncompleteOrUnsafeConfiguration(t *testing.T) {
	for name, change := range map[string]func(*Config){
		"sessions": func(c *Config) { c.Sessions = nil }, "epochs": func(c *Config) { c.Epochs = nil }, "store": func(c *Config) { c.Store = nil },
		"registry": func(c *Config) { c.Registry = nil }, "snapshot": func(c *Config) { c.Snapshot = nil }, "policy": func(c *Config) { c.Authorize = nil },
		"keys": func(c *Config) { c.Keys = nil }, "clock": func(c *Config) { c.Now = nil }, "registry version": func(c *Config) { c.RegistryVersion = " " },
		"key version": func(c *Config) { c.KeyVersion = "internal-auth:v1" }, "retention": func(c *Config) { c.Retention = 0 },
		"timeout": func(c *Config) { c.ExecutionTimeout = 0 }, "finalization": func(c *Config) { c.FinalizationTimeout = 0 },
		"source": func(c *Config) { c.Source = commandcatalog.KeyboardGlobal }, "empty handlers": func(c *Config) { c.Handlers = nil },
		"unknown route": func(c *Config) { c.Handlers["missing.command"] = c.Handlers["fixture.read"] },
		"write contract": func(c *Config) {
			h := c.Handlers["fixture.read"]
			h.Contract.Effect = commandcatalog.Write
			c.Handlers["fixture.read"] = h
		},
		"mutates capability": func(c *Config) {
			h := c.Handlers["fixture.read"]
			h.Contract.MutatesEffectiveCapability = true
			c.Handlers["fixture.read"] = h
		},
		"nil start": func(c *Config) { h := c.Handlers["fixture.read"]; h.Start = nil; c.Handlers["fixture.read"] = h },
	} {
		t.Run(name, func(t *testing.T) {
			config := configOnlyFixture(t)
			change(&config)
			if service, err := New(config); service != nil || !errors.Is(err, ErrInvalidConfiguration) {
				t.Fatalf("%v, %v", service, err)
			}
		})
	}
}

func TestNewDetachesHandlerMap(t *testing.T) {
	config := configOnlyFixture(t)
	service, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	delete(config.Handlers, "fixture.read")
	config.RegistryVersion = "changed"
	if len(service.config.Handlers) != 1 || service.config.Handlers["fixture.read"].Start == nil || service.config.RegistryVersion != "1" {
		t.Fatal("configuração externa alterou snapshot")
	}
}

func TestNilExecutionServiceFailsClosed(t *testing.T) {
	var service *Service
	if record, err := service.Execute(context.Background(), "", Request{}); record != (commandledger.Record{}) || !errors.Is(err, ErrInvalidRequest) {
		t.Fatal(record, err)
	}
	if record, err := service.GetInvocation(context.Background(), "", Request{}); record != (commandledger.Record{}) || !errors.Is(err, ErrInvalidRequest) {
		t.Fatal(record, err)
	}
}
