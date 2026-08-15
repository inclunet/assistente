package wailsapi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"assistente/controllers"
	"assistente/internal/llm"
	"assistente/internal/profiles"
)

type stubSyncSender struct {
	err  error
	resp string
}

func (s stubSyncSender) SendMessageSync(ctx context.Context, messages []llm.Message, params llm.ChatParams) (string, error) {
	return s.resp, s.err
}

func TestChatNotWired(t *testing.T) {
	t.Parallel()
	api := NewChat()

	if _, err := api.SendMessage("c1", "hi", "", llm.ChatParams{}); !errors.Is(err, ErrChatNotWired) {
		t.Fatalf("SendMessage: got %v", err)
	}
	if _, err := api.RetryMessage("c1", "m1", llm.ChatParams{}); !errors.Is(err, ErrChatNotWired) {
		t.Fatalf("RetryMessage: got %v", err)
	}
	if _, err := api.SendMessageSync(nil, llm.ChatParams{}); !errors.Is(err, ErrChatNotWired) {
		t.Fatalf("SendMessageSync: got %v", err)
	}
}

func TestChatNilControllerIsNotWired(t *testing.T) {
	t.Parallel()
	api := NewChat()
	AttachChat(api, stubSession{}, nil, stubSyncSender{})
	if _, err := api.SendMessage("c1", "hi", "", llm.ChatParams{}); !errors.Is(err, ErrChatNotWired) {
		t.Fatalf("SendMessage com ctrl nil: got %v", err)
	}
}

func TestChatNilSyncCtrlIsNotWired(t *testing.T) {
	t.Parallel()
	api := NewChat()
	AttachChat(api, stubSession{}, controllers.NewChatController(controllers.ChatControllerConfig{}), nil)
	if _, err := api.SendMessageSync(nil, llm.ChatParams{}); !errors.Is(err, ErrChatNotWired) {
		t.Fatalf("SendMessageSync com syncCtrl nil: got %v", err)
	}
	// SendMessage ainda funciona com syncCtrl nil (deps de chat independentes).
	if _, err := api.SendMessage("c1", "hi", "", llm.ChatParams{}); errors.Is(err, ErrChatNotWired) {
		t.Fatalf("SendMessage não deve exigir syncCtrl: got %v", err)
	}
}

func TestChatUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "chat.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("chat.go não deve chamar requireAuthenticatedContext(; use WithUser")
	}
	if !strings.Contains(body, "WithUser(session,") {
		t.Fatal("chat.go deve chamar WithUser(session,")
	}
}

func TestChatAuthRejectsWhenSessionFails(t *testing.T) {
	t.Parallel()
	semAuth := errors.New("sessão não autenticada")
	api := NewChat()
	AttachChat(api, stubSession{err: semAuth}, controllers.NewChatController(controllers.ChatControllerConfig{}), stubSyncSender{})

	casos := []struct {
		nome string
		fn   func() error
	}{
		{"SendMessage", func() error {
			_, err := api.SendMessage("c1", "hi", "", llm.ChatParams{})
			return err
		}},
		{"RetryMessage", func() error {
			_, err := api.RetryMessage("c1", "m1", llm.ChatParams{})
			return err
		}},
		{"SendMessageSync", func() error {
			_, err := api.SendMessageSync(nil, llm.ChatParams{})
			return err
		}},
	}
	for _, c := range casos {
		c := c
		t.Run(c.nome, func(t *testing.T) {
			t.Parallel()
			if err := c.fn(); !errors.Is(err, semAuth) {
				t.Fatalf("erro = %v, quer o da sessão", err)
			}
		})
	}
}

func TestChatSendMessageSync_NoProfileReturnsError(t *testing.T) {
	t.Parallel()
	api := NewChat()
	syncCtrl := controllers.NewSettingsController(controllers.SettingsControllerConfig{
		ProfileMgr: profiles.NewManager(),
	})
	AttachChat(api, stubSession{}, controllers.NewChatController(controllers.ChatControllerConfig{}), syncCtrl)

	_, err := api.SendMessageSync(nil, llm.ChatParams{})
	if err == nil {
		t.Fatal("Expected error when no profile/provider configured")
	}
}
