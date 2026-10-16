package toolinvocations

import (
	"encoding/json"
	"strings"
	"testing"

	"assistente/internal/tools"
)

func TestRedactJSONAtPathsSupportsNestedArraysAndEscapedSegments(t *testing.T) {
	raw := []byte(`{"nested":{"secret":"input-secret"},"items":[{"token":"array-secret"}],"a/b":"slash-secret"}`)
	redacted, err := RedactJSONAtPaths(raw, []string{"/nested/secret", "/items/0/token", "/a~1b"})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(redacted, &got); err != nil {
		t.Fatal(err)
	}
	if got["nested"].(map[string]any)["secret"] != redactedValue ||
		got["items"].([]any)[0].(map[string]any)["token"] != redactedValue ||
		got["a/b"] != redactedValue {
		t.Fatalf("paths sensíveis não foram redigidos: %s", redacted)
	}
}

func TestOutputForPersistenceWithSensitivePathsDropsLateralPayloads(t *testing.T) {
	svc := &Service{persistMaxResultSize: 4096}
	persisted := svc.outputForPersistence(tools.ToolResult{
		Content:  `{"secret":"output-secret","visible":"ok"}`,
		Metadata: map[string]any{"resource": "output-secret"},
		Annotations: &tools.ResultAnnotations{HTTPResponse: &tools.HTTPResponseAnnotation{
			URL: "https://example.invalid/output-secret",
		}},
		Failure: &tools.ToolFailure{Code: "output-secret", Kind: tools.ErrorKindUnknown},
	}, []string{"/secret"})
	if strings.Contains(string(persisted), "output-secret") {
		t.Fatalf("payload lateral reteve segredo: %s", persisted)
	}
	var payload map[string]any
	if err := json.Unmarshal(persisted, &payload); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"metadata", "annotations", "failure"} {
		if _, exists := payload[key]; exists {
			t.Fatalf("payload %s não foi omitido: %s", key, persisted)
		}
	}
	var content map[string]any
	if err := json.Unmarshal([]byte(payload["content"].(string)), &content); err != nil {
		t.Fatal(err)
	}
	if content["secret"] != redactedValue {
		t.Fatalf("content não redigido: %#v", content)
	}
}

func TestRedactJSONAtPathsNeverReturnsInvalidInput(t *testing.T) {
	redacted, err := RedactJSONAtPaths([]byte(`{"secret"`), []string{"/secret"})
	if err != nil {
		t.Fatal(err)
	}
	if string(redacted) != `{"_redacted":true}` {
		t.Fatalf("fallback de redação = %s", redacted)
	}
}
