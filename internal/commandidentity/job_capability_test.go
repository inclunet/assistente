package commandidentity

import "testing"

func TestJobServiceCapabilitiesHaveDistinctIdentity(t *testing.T) {
	// O runtime vincula a capacidade ao contexto privado do run. Uma emissão
	// independente não pode ser indistinguível da capacidade já vinculada.
	seen := make(map[JobServiceCapability]bool)
	for i := 0; i < 128; i++ {
		capability := JobServiceCapabilityForRuntime()
		if capability == nil || seen[capability] {
			t.Fatal("capacidade ausente ou reutilizada entre emissões")
		}
		seen[capability] = true
	}
}
