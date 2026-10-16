package httpapi

import (
	"testing"

	"assistente/internal/commandexecution"
)

type fixedExternalCommandProvider struct {
	service *commandexecution.ExternalService
}

func (p fixedExternalCommandProvider) ExternalCommandService(string) *commandexecution.ExternalService {
	return p.service
}

func TestNewRejectsMixedExternalCommandProviderAndStaticMap(t *testing.T) {
	fixture := newHTTPCommandFixture(t, false)
	provider := fixedExternalCommandProvider{service: fixture.services["ui"]}
	server := New(Config{
		Mode:                    "external",
		ExternalCommandProvider: provider,
		ExternalCommands:        fixture.services,
	})

	if server.externalCommandProvider != nil || len(server.externalCommands) != 0 {
		t.Fatal("mixed provider/map configuration must fail closed by disabling both service sources")
	}
	for _, source := range []string{"palette", "ui", "chat"} {
		if service := server.externalCommandServiceFor(source); service != nil {
			t.Fatalf("mixed configuration exposed %q service: %p", source, service)
		}
	}
}
