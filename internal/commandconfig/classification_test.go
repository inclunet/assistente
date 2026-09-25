package commandconfig

import "testing"

func TestCommandConfigActionClassificationIsClosed(t *testing.T) {
	for _, a := range []string{"layer_list", "layer_get", "binding_list", "binding_check_conflict", "config_export"} {
		if c, e := ClassifyAction(a); e != nil || c != ReadAction {
			t.Fatalf("%s: %s %v", a, c, e)
		}
	}
	for _, a := range []string{"layer_create", "layer_update", "layer_delete", "layer_enable", "layer_disable", "layer_restore", "binding_create", "binding_update", "binding_delete", "binding_enable", "binding_disable", "binding_restore", "config_restore", "config_import"} {
		if c, e := ClassifyAction(a); e != nil || c != CapabilityMutation {
			t.Fatalf("%s: %s %v", a, c, e)
		}
	}
	for _, a := range []string{"", "binding_write", "config_export_sensitive", "future_action"} {
		if _, e := ClassifyAction(a); e == nil {
			t.Fatalf("ação desconhecida admitida: %s", a)
		}
	}
}
