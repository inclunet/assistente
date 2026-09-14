package toolinvocations

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInvocationProjectionCalculaHashesSemExporValores(t *testing.T) {
	invocation := &Invocation{
		Input:  json.RawMessage(`{"token":"segredo","query":"termo"}`),
		Output: json.RawMessage(`{"content":"resultado-secreto","is_error":false}`),
	}
	populateInputProjection(invocation)
	populateOutputProjection(invocation)

	if invocation.InputBytes == 0 || invocation.OutputBytes == 0 ||
		invocation.InputHash == "" || invocation.OutputHash == "" ||
		invocation.ResultAvailability != "available" {
		t.Fatalf("projeção incompleta: %+v", invocation)
	}
	if strings.Contains(invocation.InputPreview, "segredo") ||
		strings.Contains(invocation.OutputPreview, "resultado-secreto") {
		t.Fatalf("preview expôs valor: input=%q output=%q", invocation.InputPreview, invocation.OutputPreview)
	}
}
