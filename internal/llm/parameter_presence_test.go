package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"assistente/internal/credentials"
)

func TestChatParameterPresenceReachesHTTPPayload(t *testing.T) {
	for _, format := range []APIFormat{APIFormatOpenAI, APIFormatOpenAIResponses} {
		t.Run(string(format), func(t *testing.T) {
			var bodies []map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				bodies = append(bodies, body)
				w.Header().Set("Content-Type", "application/json")
				if format == APIFormatOpenAI {
					_, _ = w.Write([]byte(`{"id":"test","choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
				} else {
					_, _ = w.Write([]byte(`{"id":"test","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}]}`))
				}
			}))
			defer server.Close()
			config := &ProviderConfig{ID: "test", APIFormat: format, BaseURL: server.URL, AuthMode: AuthModeNone, DefaultModel: "model"}
			provider := NewChatProvider(config, credentials.NewManager(nil), nil)
			zero, one := 0.0, 1.0
			for _, params := range []ChatParams{{}, {Temperature: &zero, TopP: &one}} {
				if _, err := provider.SendChat(t.Context(), []Message{{Role: "user", Content: "test"}}, params); err != nil {
					t.Fatal(err)
				}
			}
			if len(bodies) != 2 {
				t.Fatalf("requests=%d", len(bodies))
			}
			if _, present := bodies[0]["temperature"]; present {
				t.Fatal("absent temperature sent")
			}
			if _, present := bodies[0]["top_p"]; present {
				t.Fatal("absent top_p sent")
			}
			if bodies[1]["temperature"] != float64(0) || bodies[1]["top_p"] != float64(1) {
				t.Fatalf("explicit values lost: %v", bodies[1])
			}
		})
	}
}
