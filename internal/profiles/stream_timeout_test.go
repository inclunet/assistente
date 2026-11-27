package profiles

import (
	"encoding/json"
	"testing"
)

func TestPortableLegacyStreamTimeoutRemainsEditable(t *testing.T) {
	original := DefaultProfile()
	original.Active = false
	original.Chat.ResponseTimeout = 90000
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := ReadPortableDefinition(raw)
	if err != nil {
		t.Fatal(err)
	}
	imported.Name = "Perfil renomeado"
	if err := imported.Validate(); err != nil {
		t.Fatal(err)
	}
	if imported.Chat.ResponseTimeout != 90000 {
		t.Fatal("prazo legado foi alterado")
	}
}
