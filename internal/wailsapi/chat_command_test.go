package wailsapi

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"assistente/controllers"
	"assistente/internal/llm"
)

func TestChatCommandMetadataTransportAndNoLegacyFallback(t *testing.T) {
	api := NewChat()
	called := 0
	next := func(context.Context) (string, error) { called++; return "accepted", nil }
	ctx := context.Background()
	if _, err := api.submit(ctx, &llm.ChatCommandMetadata{}, "c", "", next); err == nil || called != 0 {
		t.Fatal("unwired metadata fell back")
	}
	rejected := errors.New("rejected")
	AttachChatCommandHook(api, func(context.Context, *llm.ChatCommandMetadata, string, string, func(context.Context) (string, error)) (string, error) {
		return "", rejected
	})
	if _, err := api.submit(ctx, &llm.ChatCommandMetadata{}, "c", "", next); !errors.Is(err, rejected) || called != 0 {
		t.Fatal("invalid metadata fell back")
	}
	if got, err := api.submit(ctx, nil, "c", "", next); err != nil || got != "accepted" || called != 1 {
		t.Fatal("legacy caller broken")
	}
	var params llm.ChatParams
	if err := json.Unmarshal([]byte(`{"command":{"ticket":"t","handoffId":"h"},"model":"m"}`), &params); err != nil {
		t.Fatal(err)
	}
	if params.Command == nil || params.Command.Ticket != "t" || params.Command.HandoffID != "h" {
		t.Fatal("metadata contract")
	}
	metadata, stripped := stripChatCommand(params)
	if metadata != params.Command || stripped.Command != nil || stripped.Model != params.Model {
		t.Fatal("metadata reached controller params or model was changed")
	}
	AttachChat(api, stubSession{}, controllers.NewChatController(controllers.ChatControllerConfig{}))
	for _, retry := range []bool{false, true} {
		AttachChatCommandHook(api, func(got context.Context, metadata *llm.ChatCommandMetadata, cid, mid string, next func(context.Context) (string, error)) (string, error) {
			if metadata.Ticket != "t" || metadata.HandoffID != "h" || cid != "c" || (mid != "") != retry {
				t.Fatal("hook correlation")
			}
			return "accepted", nil // Do not enter a deliberately unwired controller.
		})
		var err error
		if retry {
			_, err = api.RetryMessage("c", "m", params)
		} else {
			_, err = api.SendMessage("c", "private", "media", params)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}
