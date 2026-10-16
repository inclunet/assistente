package app

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// O mesmo JSON é consumido por commandFrontendContract.test.ts através do
// adaptador Wails e do controlador real, não por uma cópia do validador.
func TestCommandFrontendKeyboardWireContract(t *testing.T) {
	a := commandJobPublicationApp(t)
	keyboard, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	keyboard.Generation, keyboard.OwnerID, keyboard.SessionID, keyboard.WorkspaceID = "contract", "contract-user", "contract-session", "contract-workspace"
	keyboard.ValidUntil = 0
	raw, err := json.Marshal(keyboard)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../../frontend/src/lib/__fixtures__/commandKeyboardMap.json")
	if err != nil {
		t.Fatal(err)
	}
	var gotJSON, wantJSON any
	if err := json.Unmarshal(raw, &gotJSON); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(want, &wantJSON); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotJSON, wantJSON) {
		t.Fatal("DTO padrão divergiu da fixture compartilhada com o frontend; atualize e valide os dois lados do contrato")
	}
}
