package jobs

import "testing"

func TestNativePackageRejectsUnknownAndTrailingDefinitions(t *testing.T) {
	const job = "id: revisar\nname: Revisar\ntool: notify\ntriggers: [{type: manual}]\n"
	for _, suffix := range []string{"plugin_installation_id: owner-only\n", "---\n" + job} {
		if _, err := ParsePackageDefinition([]byte(job + suffix)); err == nil {
			t.Errorf("aceitou definição adicional ou campo local: %q", suffix)
		}
	}
}
