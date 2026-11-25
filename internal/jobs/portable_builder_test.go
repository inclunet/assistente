package jobs

import (
	"strings"
	"testing"
)

func TestOlderPackageRejectsUnsupportedFields(t *testing.T) {
	for _, field := range []string{`"dry_run":{"enabled":true}`, `"pipeline":"nightly"`, `"error_policy":{"strategy":"retry"}`, `"events":{"on_success":"chain"}`} {
		raw := []byte(`{"name":"Review","tool":"notify","triggers":[{"type":"manual"}],` + field + `}`)
		if _, err := ReadPortableJob("review", raw); err == nil {
			t.Fatal("campo incompatível com o pacote antigo", field)
		}
	}
	if _, err := ReadPortableJob("review", []byte(`{"name":"Review","tool":"notify","triggers":[{"type":"manual","when":"false"}]}`)); err == nil {
		t.Fatal("o contrato antigo aceitou condição")
	}
	raw := []byte(`{"name":"Review","tool":"notify","maxRunsPerHour":1,"triggers":[{"type":"interval","every":"5m"}]}`)
	job, err := ReadPortableJob("review", raw)
	if err != nil || job.MaxRunsPerHour != 1 || job.Triggers[0].Every != "5m" {
		t.Fatal(job, err)
	}
	conflict := strings.Replace(string(raw), "{", `{"requiredInputs":["text"],"inputs":{"text":"supplied"},`, 1)
	if _, err := ReadPortableJob("review", []byte(conflict)); err == nil {
		t.Fatal("pacote preencheu sua própria entrada local")
	}
}

func TestNativePackageRejectsUnknownAndTrailingDefinitions(t *testing.T) {
	const job = "id: revisar\nname: Revisar\ntool: notify\ntriggers: [{type: manual}]\n"
	for _, suffix := range []string{"plugin_installation_id: owner-only\n", "---\n" + job} {
		if _, err := ParsePackageDefinition([]byte(job + suffix)); err == nil {
			t.Errorf("aceitou definição adicional ou campo local: %q", suffix)
		}
	}
}
