package app

import (
	"strings"
	"testing"

	"assistente/internal/commandautomation"
	"assistente/internal/commandconfig"
)

func TestCommandSettingsDiffExplainsValuesWithoutDeviceOrOwnerIdentifiers(t *testing.T) {
	command := "workspace.tab.next"
	before := commandconfig.Binding{ID: "private-binding", LayerRef: "private-layer", CommandID: &command, TriggerType: "streamdeck.key", TriggerSpec: `{"version":1,"device":"private-serial","key":2}`, Arguments: `{}`, Condition: `{"version":1,"clauses":[]}`, Enabled: true, Effect: "execute", ReviewStatus: "active"}
	after := before
	after.ResolutionPriority = 42
	diff := commandconfig.MutationDiff{Scope: commandconfig.Scope{UserID: "private-owner"}, BeforeLayers: []commandconfig.Layer{{ID: "private-layer", Name: "Trabalho"}}, AfterLayers: []commandconfig.Layer{{ID: "private-layer", Name: "Trabalho"}}, BeforeBindings: []commandconfig.Binding{before}, AfterBindings: []commandconfig.Binding{after}}
	for _, locale := range []string{"pt-BR", "en", "es"} {
		text, err := renderCommandSettingsDiff(locale, diff)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"private-owner", "private-serial", "private-binding", "private-layer"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s leaked %s: %s", locale, forbidden, text)
			}
		}
		for _, expected := range []string{"Trabalho", command, "42", "Stream Deck"} {
			if !strings.Contains(text, expected) {
				t.Fatalf("%s omitted changed value %s: %s", locale, expected, text)
			}
		}
		words := commandSettingsDiffWords(locale)
		if !strings.Contains(text, words["before"]) || !strings.Contains(text, words["after"]) {
			t.Fatalf("missing before/after: %s", text)
		}
	}
}

func TestCommandSettingsDiffExplainsGrantWithoutAuthorityMaterial(t *testing.T) {
	diff := commandconfig.MutationDiff{Scope: commandconfig.Scope{UserID: "private-owner"}, AfterAutomationGrants: []commandautomation.Grant{{ID: "private-grant", LayerRef: commandautomation.RuleRef{Ref: "private-layer"}, EventName: commandautomation.JobRunStateEvent, AutomationGrantGeneration: 2, AutomationGrantFingerprint: "private-fingerprint", AuthorizationDecisionID: "private-receipt"}}, BeforeLayers: []commandconfig.Layer{{ID: "private-layer", Name: "Durante o job"}}, AfterLayers: []commandconfig.Layer{{ID: "private-layer", Name: "Durante o job"}}}
	for _, locale := range []string{"pt-BR", "en", "es"} {
		text, err := renderCommandSettingsDiff(locale, diff)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(text, "Durante o job") || !strings.Contains(text, commandautomation.JobRunStateEvent) || !strings.Contains(text, commandSettingsDiffWords(locale)["authorization"]) {
			t.Fatalf("authorization missing: %s", text)
		}
		if strings.Contains(text, "private-") {
			t.Fatalf("authority leaked: %s", text)
		}
	}
}

func TestCommandSettingsDiffRejectsEmptyChange(t *testing.T) {
	if _, err := renderCommandSettingsDiff("pt-BR", commandconfig.MutationDiff{}); err == nil {
		t.Fatal("missing owner accepted")
	}
	if _, err := renderCommandSettingsDiff("pt-BR", commandconfig.MutationDiff{Scope: commandconfig.Scope{UserID: "owner"}}); err == nil {
		t.Fatal("empty diff accepted")
	}
}
